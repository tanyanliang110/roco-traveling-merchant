package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ============================================================
// 配置结构体 - 对应 config.json
// ============================================================

// Config 总配置
type Config struct {
	Server     ServerConfig     `json:"server"`
	Crawl      CrawlConfig      `json:"crawl"`
	ServerChan ServerChanConfig `json:"serverchan"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port string `json:"port"`
}

// CrawlConfig 爬虫配置
type CrawlConfig struct {
	TargetURL string `json:"target_url"`
	Interval  int    `json:"interval"` // 间隔（分钟）
}

// ServerChanConfig Server酱³ 推送配置
type ServerChanConfig struct {
	UID     string `json:"uid"`
	SendKey string `json:"sendkey"`
}

// 全局配置实例
var appConfig Config

// ============================================================
// 加载配置
// ============================================================

// LoadConfig 从 JSON 文件加载配置。文件不存在时返回默认配置。
func LoadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件失败: %w", err)
	}
	applyDefaults(&cfg)
	return cfg, nil
}

// ApplyEnvironment applies non-empty secret values from the environment.
func ApplyEnvironment(cfg Config, getenv func(string) string) Config {
	if getenv == nil {
		return cfg
	}
	if sendKey := getenv("SERVERCHAN_SENDKEY"); sendKey != "" {
		cfg.ServerChan.SendKey = sendKey
	}
	return cfg
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{Port: ":8008"},
		Crawl: CrawlConfig{
			TargetURL: "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0",
			Interval:  3,
		},
	}
}

func applyDefaults(cfg *Config) {
	defaults := defaultConfig()
	if cfg.Server.Port == "" {
		cfg.Server.Port = defaults.Server.Port
	}
	if cfg.Crawl.TargetURL == "" {
		cfg.Crawl.TargetURL = defaults.Crawl.TargetURL
	}
	if cfg.Crawl.Interval <= 0 {
		cfg.Crawl.Interval = defaults.Crawl.Interval
	}
}

// CrawlInterval 返回爬取间隔的 time.Duration
func CrawlInterval() time.Duration {
	return time.Duration(appConfig.Crawl.Interval) * time.Minute
}

// ServerChanEnabled 判断 Server酱 推送是否已配置
func ServerChanEnabled() bool {
	k := appConfig.ServerChan.SendKey
	return k != "" && k != "your_sendkey"
}
