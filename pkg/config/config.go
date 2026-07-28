package config

import (
	"os"
	"path/filepath"
	"sync"

	yaml "gopkg.in/yaml.v3"
)

// Config 架构定义
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	TDX      TDXConfig      `yaml:"tdx"`
	Cache    CacheConfig    `yaml:"cache"`
	Database DatabaseConfig `yaml:"database"`
	Agent    AgentConfig    `yaml:"agent"`
}

// ServerConfig HTTP 服务器配置
type ServerConfig struct {
	// Port is the TCP port the server binds to.
	Port int `yaml:"port"`
	// BindAddress is the address the server listens on. Use "127.0.0.1"
	// (default) for local-only access, or "0.0.0.0" for all interfaces.
	// Binding to a non-loopback address requires AccessToken to be set.
	BindAddress string `yaml:"bind_address"`
	// AccessToken is a shared secret required for non-loopback access. When
	// empty the server only accepts requests from the loopback interface
	// (see BindAddress). When set, clients must send
	// "Authorization: Bearer <token>" to access protected routes.
	AccessToken string `yaml:"access_token"`
}

// TDXConfig 通达信相关配置
type TDXConfig struct {
	Hosts []string `yaml:"hosts"`
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Backend string `yaml:"backend"`
	Dir     string `yaml:"dir"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

// AgentConfig AI Agent 配置
type AgentConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Home       string `yaml:"home"`
	Config     string `yaml:"config"`
	Model      string `yaml:"model"`
	Agent      string `yaml:"agent"`
	Session    string `yaml:"session"`
	StockAgent string `yaml:"stock_agent"` // default agent for stock analysis panel
}

// DefaultConfig 返回一个包含默认值的 Config 实例
func DefaultConfig() *Config {
	return &Config{
		Server:   ServerConfig{Port: 8080, BindAddress: "127.0.0.1"},
		TDX:      TDXConfig{Hosts: nil},
		Cache:    CacheConfig{Backend: "sqlite", Dir: CacheDir()},
		Database: DatabaseConfig{Driver: "sqlite3", DSN: DBPath()},
	}
}

func defaultConfigTemplate() string {
	return `# TongStock 配置文件
# 通达信股票数据工具配置

# HTTP 服务配置
server:
  # 监听端口
  port: 8080
  # 监听地址，默认仅本机访问
  # - 127.0.0.1: 仅本机 (安全默认值)
  # - 0.0.0.0  所有网卡，此时必须配置 access_token
  bind_address: 127.0.0.1
  # 远程访问时需要的 Bearer Token。留空则仅允许本机访问
  # access_token: "please-change-me"

# 通达信服务器配置
tdx:
  # 服务器地址列表 (留空使用内置默认地址)
  # hosts:
  #   - "124.71.187.122:7709"
  #   - "122.51.120.217:7709"

# 缓存配置
cache:
  # 缓存后端: sqlite 或 file
  backend: sqlite
  # 缓存目录 (留空使用默认路径 ~/.tongstock/cache)
  # dir: ~/.tongstock/cache

# 数据库配置 (用于 K 线、交易日历等)
database:
  # 驱动: sqlite3, postgres, mysql
  driver: sqlite3
  # 连接字符串
  # dsn: ~/.tongstock/cache/tongstock.db

# AI Agent 配置 (可选，需要 picoclaw)
# agent:
#   enabled: true
#   home: ~/.picoclaw
#   config: ~/.picoclaw/config.json
#   model: ""
#   agent: ""
#   session: ""
#   stock_agent: "stock-analyst"  # 个股分析面板默认 agent
`
}

// Load 读取并合并配置，若无配置文件则写入默认模板并返回默认配置
func Load() (*Config, error) {
	if err := EnsureHomeDir(); err != nil {
		return nil, err
	}
	path := ConfigPath()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := writeDefaultConfig(path); err != nil {
			return nil, err
		}
		return DefaultConfig(), nil
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tmp := &Config{}
	if err := yaml.Unmarshal(data, tmp); err != nil {
		return nil, err
	}

	merged := DefaultConfig()
	if tmp.Server.Port != 0 {
		merged.Server.Port = tmp.Server.Port
	}
	if tmp.Server.BindAddress != "" {
		merged.Server.BindAddress = tmp.Server.BindAddress
	}
	if tmp.Server.AccessToken != "" {
		merged.Server.AccessToken = tmp.Server.AccessToken
	}
	if len(tmp.TDX.Hosts) > 0 {
		merged.TDX.Hosts = tmp.TDX.Hosts
	}
	if tmp.Cache.Backend != "" {
		merged.Cache.Backend = tmp.Cache.Backend
	}
	if tmp.Cache.Dir != "" {
		merged.Cache.Dir = tmp.Cache.Dir
	}
	if merged.Cache.Dir == "" {
		merged.Cache.Dir = CacheDir()
	}
	if tmp.Database.Driver != "" {
		merged.Database.Driver = tmp.Database.Driver
	}
	if tmp.Database.DSN != "" {
		merged.Database.DSN = tmp.Database.DSN
	}
	if tmp.Agent.Enabled {
		merged.Agent.Enabled = tmp.Agent.Enabled
	}
	if tmp.Agent.Home != "" {
		merged.Agent.Home = tmp.Agent.Home
	}
	if tmp.Agent.Config != "" {
		merged.Agent.Config = tmp.Agent.Config
	}
	if tmp.Agent.Model != "" {
		merged.Agent.Model = tmp.Agent.Model
	}
	if tmp.Agent.Agent != "" {
		merged.Agent.Agent = tmp.Agent.Agent
	}
	if tmp.Agent.Session != "" {
		merged.Agent.Session = tmp.Agent.Session
	}
	if tmp.Agent.StockAgent != "" {
		merged.Agent.StockAgent = tmp.Agent.StockAgent
	}

	return merged, nil
}

// Save 将 Config 写入配置文件
func Save(cfg *Config) error {
	if err := EnsureHomeDir(); err != nil {
		return err
	}
	path := ConfigPath()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return nil
}

func writeDefaultConfig(path string) error {
	template := defaultConfigTemplate()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(template), 0644)
}

var (
	globalConfig *Config
	configMu     sync.RWMutex
	configOnce   sync.Once
)

// Get 返回全局配置，在需要时延迟加载
func Get() *Config {
	configMu.RLock()
	if globalConfig != nil {
		defer configMu.RUnlock()
		return globalConfig
	}
	configMu.RUnlock()

	configOnce.Do(func() {
		cfg, err := Load()
		if err != nil {
			cfg = DefaultConfig()
		}
		configMu.Lock()
		globalConfig = cfg
		configMu.Unlock()
	})

	configMu.RLock()
	defer configMu.RUnlock()
	return globalConfig
}

// Init 显式初始化配置，应用启动阶段调用
func Init() error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	configMu.Lock()
	globalConfig = cfg
	configMu.Unlock()
	return nil
}
