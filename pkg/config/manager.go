package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig represents the agent configuration
type AgentConfig struct {
	ControllerURL    string            `yaml:"controller_url"`
	DeviceID         string            `yaml:"device_id"`
	PrivateKey       string            `yaml:"private_key"`
	PublicKey        string            `yaml:"public_key"`
	AssignedIP       string            `yaml:"assigned_ip"`
	LogLevel         string            `yaml:"log_level"`
	Interface        string            `yaml:"interface"`
	StorageDir       string            `yaml:"storage_dir"`
	RuntimeMode      string            `yaml:"runtime_mode,omitempty"`
	Region           string            `yaml:"region,omitempty"`          // 区域标识 (e.g., "cn-shanghai", "us-west")
	CustomerID       string            `yaml:"customer_id,omitempty"`      // 客户标识 (e.g., "customer-001")
	AdvertisedRoutes []string          `yaml:"advertised_routes,omitempty"` // 宣告的网段 (e.g., ["172.16.0.0/24", "10.0.0.0/16"])
	Hostname         string            `yaml:"hostname,omitempty"`         // 主机名
	Metrics          MetricsConfig     `yaml:"metrics,omitempty"`          // 监控配置
	MultiTunnel      MultiTunnelConfig `yaml:"multi_tunnel,omitempty"`     // 多隧道配置
	CACert           string            `yaml:"ca_cert,omitempty"`          // CA 证书路径（用于 TLS 验证）
}

// MultiTunnelConfig represents multi-tunnel configuration
type MultiTunnelConfig struct {
	Enabled     bool   `yaml:"enabled"`       // 是否启用多隧道
	TunnelCount int    `yaml:"tunnel_count"`  // 隧道数量（0=使用默认值 4）
	BasePort    int    `yaml:"base_port"`     // 起始端口（默认 51820）
	SharedKey   bool   `yaml:"shared_key"`    // 是否共享私钥（默认 true）
}

// MetricsConfig 监控配置
type MetricsConfig struct {
	Enabled         bool          `yaml:"enabled"`
	ListenAddr      string        `yaml:"listen_addr"`
	PushGateway     string        `yaml:"push_gateway"`
	PushInterval    time.Duration `yaml:"push_interval"`
	CollectInterval time.Duration `yaml:"collect_interval"`
	PushAuth        struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"push_auth"`
}

// Manager handles intelligent configuration loading and initialization
type Manager struct {
	configPath string
}

// NewManager creates a new config manager
func NewManager(configPath string) *Manager {
	return &Manager{
		configPath: configPath,
	}
}

// LoadOrInit implements the intelligent decision tree:
// 1. Check if local config exists
// 2. If no DeviceID, return nil (first-time registration needed)
// 3. If has DeviceID, return existing config (re-sync without token)
// 4. If force flag is set, ignore existing config and return nil
func (m *Manager) LoadOrInit(force bool) (*AgentConfig, error) {
	// Check if config file exists
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		// Config doesn't exist - first-time registration needed
		return nil, nil
	}

	// If force flag is set, ignore existing config
	if force {
		return nil, nil
	}

	// Load existing config
	config, err := m.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load existing config: %w", err)
	}

	// Check if DeviceID exists
	if config.DeviceID == "" {
		// No DeviceID - treat as first-time registration
		return nil, nil
	}

	// Has DeviceID - return existing config for re-sync
	return config, nil
}

// Load reads the configuration from file
func (m *Manager) Load() (*AgentConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config AgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 设置默认值（如果未配置）
	m.setDefaults(&config)

	return &config, nil
}

// setDefaults 设置默认值
func (m *Manager) setDefaults(config *AgentConfig) {
	// Metrics 默认配置
	if config.Metrics.ListenAddr == "" {
		config.Metrics.Enabled = true
		config.Metrics.ListenAddr = ":9090"
		config.Metrics.PushGateway = "http://localhost:8428/api/v1/import/prometheus"
		config.Metrics.PushInterval = 15 * time.Second
		config.Metrics.CollectInterval = 10 * time.Second
	}

	// MultiTunnel 默认配置
	if config.MultiTunnel.BasePort == 0 {
		config.MultiTunnel.BasePort = 51820
	}
	// SharedKey defaults to true
	if !config.MultiTunnel.Enabled {
		config.MultiTunnel.SharedKey = true
	}
	// TunnelCount defaults to 0 (will use DefaultTunnelCount = 4)
	// Controller can override this value
}

// Save writes the configuration to file
func (m *Manager) Save(config *AgentConfig) error {
	// Ensure directory exists
	dir := "/etc/aria"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal config to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with restricted permissions
	if err := os.WriteFile(m.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Exists checks if the configuration file exists
func (m *Manager) Exists() bool {
	_, err := os.Stat(m.configPath)
	return err == nil
}

// Delete removes the configuration file (for device deletion scenarios)
func (m *Manager) Delete() error {
	if err := os.Remove(m.configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}
	return nil
}
