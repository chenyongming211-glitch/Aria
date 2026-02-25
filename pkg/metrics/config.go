package metrics

import "time"

// Config 指标配置
type Config struct {
	// Enabled 是否启用指标采集
	Enabled bool `yaml:"enabled"`

	// ListenAddr HTTP 服务监听地址（Agent）
	ListenAddr string `yaml:"listen_addr"`

	// PushGateway Push Gateway 地址（VictoriaMetrics）
	PushGateway string `yaml:"push_gateway"`

	// PushInterval Push 推送间隔
	PushInterval time.Duration `yaml:"push_interval"`

	// CollectInterval 采集间隔（内部使用）
	CollectInterval time.Duration `yaml:"collect_interval"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		ListenAddr:      ":9090",
		PushGateway:     "", // 由 Agent 从 Controller 获取配置
		PushInterval:    15 * time.Second,
		CollectInterval: 10 * time.Second,
	}
}
