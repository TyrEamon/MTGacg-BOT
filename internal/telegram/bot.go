package telegram

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"my-bot-go/internal/config"
	"my-bot-go/internal/database"
	"net/http"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/nfnt/resize"
)

type BotHandler struct {
	API *bot.Bot
	Cfg *config.Config
	DB  *database.D1Client
	// 转发会话状态
	Forwarding      bool
	ForwardTitle    string
	ForwardPreview  *models.Message
	ForwardOriginal *models.Message
}

func NewBot(cfg *config.Config, db *database.D1Client) (*BotHandler, error) {
	h := &BotHandler{Cfg: cfg, DB: db}

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.Message == nil {
				return
			}
			// 只有在 forward 模式下才收集
			if h.Forwarding {
				msg := update.Message
				log.Printf("📥 收到消息: ID=%d | Photo=%v | Doc=%v", msg.ID, len(msg.Photo) > 0, msg.Document != nil)

				// 1. 如果还没有预览图，这一条就是预览图！
				// (不管是 Photo 还是 Document，谁先来谁就是预览)
				if h.ForwardPreview == nil {
					// 只有带图或带文件的才算
					if len(msg.Photo) > 0 || msg.Document != nil {
						h.ForwardPreview = msg
						log.Printf("✅ 设定为预览图: %d", msg.ID)
						return
					}
				}

				// 2. 如果预览图已经有了，且这一条是 Document，那就是原图文件！
				if h.ForwardOriginal == nil && msg.Document != nil {
					// 确保不是刚才那条预览消息自己
					if h.ForwardPreview != nil && h.ForwardPreview.ID != msg.ID {
						h.ForwardOriginal = msg
						log.Printf("✅ 设定为原图文件: %d", msg.ID)
					}
				}
			}
		}),
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		return nil, err
	}

	h.API = b

	// 注册命令
	b.RegisterHandler(bot.HandlerTypeMessageText, "/save", bot.MatchTypeExact, h.handleSave)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/forward_start", bot.MatchTypePrefix, h.handleForwardStart)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/forward_end", bot.MatchTypeExact, h.handleForwardEnd)

	// 保留老的手动转发逻辑 (非 forward 模式下生效)
	b.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}
		if h.Forwarding {
			return
		}
		if len(update.Message.Photo) > 0 {
			h.handleManual(ctx, b, update)
		}
	})

	return h, nil
}

func (h *BotHandler) Start(ctx context.Context) {
	h.API.Start(ctx)
}

// 下载文件辅助函数
func (h *BotHandler) downloadFile(ctx context.Context, fileID string) ([]byte, error) {
	file, err := h.API.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.Cfg.BotToken, file.FilePath)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// 压缩图片辅助函数
func compressImage(data []byte, targetSize int64) ([]byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width > 9500 || height > 9500 {
		log.Printf("📏 Resizing image from %dx%d", width, height)
		if width > height {
			img = resize.Resize(9500, 0, img, resize.Lanczos3)
		} else {
			img = resize.Resize(0, 9500, img, resize.Lanczos3)
		}
	}

	log.Printf("📉 Compressing %s image...", format)
	quality := 99
	for {
		buf := new(bytes.Buffer)
		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
		if err != nil {
			return nil, fmt.Errorf("encode error: %v", err)
		}
		compressedData := buf.Bytes()
		size := int64(len(compressedData))
		if size <= targetSize || quality <= 40 {
			return compressedData, nil
		}
		quality -= 5
	}
}

// handleSave 手动保存历史记录
func (h *BotHandler) handleSave(ctx context.Context, b *bot.Bot, update *models.Update) {
	userID := update.Message.From.ID
	if userID != 8040798522 && userID != 6874581126 {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Database synced (Realtime mode).",
	})
}

// handleManual 老的手动模式
func (h *BotHandler) handleManual(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || len(update.Message.Photo) == 0 {
		return
	}
	photo := update.Message.Photo[len(update.Message.Photo)-1]
	postID := fmt.Sprintf("manual_%d", update.Message.ID)
	caption := update.Message.Caption
	if caption == "" {
		caption = "MtcACG:TG"
	}
	msg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:  h.Cfg.ChannelID,
		Photo:   &models.InputFileString{Data: photo.FileID},
		Caption: caption,
	})
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: "❌ Fail: " + err.Error()})
		return
	}
	finalFileID := msg.Photo[len(msg.Photo)-1].FileID
	h.DB.SaveImage(postID, finalFileID, "", caption, "TG-forward", "TG-C", photo.Width, photo.Height)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		Text:            "✅ Saved (Legacy Mode)",
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
}

// handleForwardStart
func (h *BotHandler) handleForwardStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil {
		return
	}
	userID := msg.From.ID
	if userID != 8040798522 && userID != 6874581126 {
		return
	}

	text := msg.Text
	title := ""
	if len(text) > len("/forward_start") {
		title = strings.TrimSpace(text[len("/forward_start"):])
	}

	h.Forwarding = true
	h.ForwardTitle = title
	h.ForwardPreview = nil
	h.ForwardOriginal = nil

	log.Printf("🚀 转发模式已开启 (User: %d)", userID)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            "✅ 转发模式开启。\n支持两种模式：\n1. 单文件模式：只发原图文件 (Bot自动生成预览)\n2. 双文件模式：先发预览图，再发原图文件\n完成后发送 /forward_end",
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	})
}

// handleForwardEnd
func (h *BotHandler) handleForwardEnd(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil {
		return
	}
	if !h.Forwarding {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "ℹ️ 请先 /forward_start"})
		return
	}
	if h.ForwardPreview == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "❌ 未收到任何文件或图片。"})
		h.Forwarding = false
		return
	}

	postID := fmt.Sprintf("manual_%d", h.ForwardPreview.ID)
	caption := h.ForwardTitle
	if caption == "" {
		caption = h.ForwardPreview.Caption
	}
	if caption == "" {
		caption = "MtcACG:TG"
	}

	var previewFileID, originFileID string
	var width, height int

	// 1. 如果预览是 Photo (常规图片)
	if len(h.ForwardPreview.Photo) > 0 {
		srcPhoto := h.ForwardPreview.Photo[len(h.ForwardPreview.Photo)-1]
		fwdMsg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:  h.Cfg.ChannelID,
			Photo:   &models.InputFileString{Data: srcPhoto.FileID},
			Caption: caption,
		})
		if err == nil && len(fwdMsg.Photo) > 0 {
			previewFileID = fwdMsg.Photo[len(fwdMsg.Photo)-1].FileID
			width = srcPhoto.Width
			height = srcPhoto.Height
		}
		// 检查有无额外原图
		if h.ForwardOriginal != nil && h.ForwardOriginal.Document != nil {
			originFileID = h.ForwardOriginal.Document.FileID
		}
	} else if h.ForwardPreview.Document != nil {
		// 2. 如果预览是 Document (文件) -> 自动下载并生成预览
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "⏳ 正在处理文件..."})
		originFileID = h.ForwardPreview.Document.FileID // 默认原图就是它

		fileData, err := h.downloadFile(ctx, originFileID)
		if err == nil {
			// 尝试作为 Photo 发送
			fwdMsg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID:  h.Cfg.ChannelID,
				Photo:   &models.InputFileUpload{Filename: "preview.jpg", Data: bytes.NewReader(fileData)},
				Caption: caption,
			})
			if err == nil && len(fwdMsg.Photo) > 0 {
				previewFileID = fwdMsg.Photo[len(fwdMsg.Photo)-1].FileID
				width = fwdMsg.Photo[len(fwdMsg.Photo)-1].Width
				height = fwdMsg.Photo[len(fwdMsg.Photo)-1].Height
			} else {
				// 转换失败（可能不是图片），那预览图也只能是文件
				previewFileID = originFileID
			}
		} else {
			previewFileID = originFileID
		}
	}

	// 存库
	err := h.DB.SaveImage(postID, previewFileID, originFileID, caption, "TG-forward", "TG-C", width, height)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "❌ Save Error: " + err.Error()})
	} else {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			Text:            "✅ 发布成功！",
			ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
		})
	}

	// 重置
	h.Forwarding = false
	h.ForwardTitle = ""
	h.ForwardPreview = nil
	h.ForwardOriginal = nil
}
