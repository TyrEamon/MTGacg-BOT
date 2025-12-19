package config

import (
	"fmt"
	"os"
)

type Config struct {
	// Telegram 相关
	BotToken  string
	ChannelID string

	// Cloudflare D1 相关
	CfApiToken   string
	CfAccountId  string
	D1DatabaseId string
}

func Load() (*Config, error) {
	cfg := &Config{
		BotToken:     os.Getenv("BOT_TOKEN"),
		ChannelID:    os.Getenv("CHANNEL_ID"),
		CfApiToken:   os.Getenv("CF_API_TOKEN"),
		CfAccountId:  os.Getenv("CF_ACCOUNT_ID"),
		D1DatabaseId: os.Getenv("D1_DATABASE_ID"),
	}

	// 简单检查必填项
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("missing BOT_TOKEN")
	}
	if cfg.ChannelID == "" {
		return nil, fmt.Errorf("missing CHANNEL_ID")
	}
	// D1 配置如果为空，只会导致数据库功能失效，暂不强制报错，视你需求而定
	// 但为了转发记录能保存，建议还是检查一下
	if cfg.CfApiToken == "" || cfg.D1DatabaseId == "" {
		// return nil, fmt.Errorf("missing Cloudflare D1 config")
		// 暂时允许为空，方便本地测试，但线上务必配置
	}

	return cfg, nil
}
