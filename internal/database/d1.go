package database

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"my-bot-go/internal/config"
	"net/http"
	"time"
)

type D1Client struct {
	Cfg     *config.Config
	History map[string]bool // 本地内存缓存，防止短时间重复
}

func NewD1Client(cfg *config.Config) *D1Client {
	return &D1Client{
		Cfg:     cfg,
		History: make(map[string]bool),
	}
}

// D1Query 请求结构
type D1Query struct {
	SQL    string        `json:"sql"`
	Params []interface{} `json:"params"`
}

// LoadHistory 从 D1 加载最近的历史记录到内存 (简单起见，只加载最近的 ID)
func (d *D1Client) LoadHistory() error {
	// 简单实现：这里可以写一个 SELECT post_id FROM images ORDER BY created_at DESC LIMIT 100
	// 暂时留空，防止启动太慢，或者你可以根据需求实现
	return nil
}

// SaveImage 保存图片信息到 D1
func (d *D1Client) SaveImage(postID, fileID, originFileID, caption, tags, source string, width, height int) error {
	// 1. 更新本地缓存
	d.History[postID] = true

	// 2. 构造 SQL
	sql := `INSERT INTO images (post_id, file_id, origin_id, caption, tags, source, width, height, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	// 注意：D1 HTTP API 的 params 需要对应
	params := []interface{}{
		postID,
		fileID,
		originFileID,
		caption,
		tags,
		source,
		width,
		height,
		time.Now().Unix(), // timestamp
	}

	return d.executeSQL(sql, params)
}

// PushHistory (为了兼容旧代码接口，这里可以留空，或者用来做批量提交)
func (d *D1Client) PushHistory() {
	// 实时 SaveImage 模式下，不需要手动 Push
}

// executeSQL 发送请求给 Cloudflare D1 API
func (d *D1Client) executeSQL(sqlStr string, params []interface{}) error {
	if d.Cfg.CfApiToken == "" || d.Cfg.D1DatabaseId == "" {
		return nil // 没配数据库就跳过
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/query", d.Cfg.CfAccountId, d.Cfg.D1DatabaseId)

	payload := D1Query{
		SQL:    sqlStr,
		Params: params,
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+d.Cfg.CfApiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("D1 API Error: %s | Body: %s", resp.Status, string(body))
	}

	return nil
}
