package main

import (
	"context"
	"log"
	"my-bot-go/internal/config"
	"my-bot-go/internal/database"
	"my-bot-go/internal/telegram"
	"os"
	"os/signal"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. 初始化数据库 (Cloudflare D1)
	db := database.NewD1Client(cfg)
	// 初始化时同步一次历史记录 (可选，看你需求，保留着比较稳妥)
	if err := db.LoadHistory(); err != nil {
		log.Printf("⚠️ Warning: Failed to load history from D1: %v", err)
	}

	// 3. 初始化 Telegram Bot
	botHandler, err := telegram.NewBot(cfg, db)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// 4. 启动 Bot (使用 Context 控制退出)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Println("🚀 Forward Bot is starting...")
	botHandler.Start(ctx)
}
