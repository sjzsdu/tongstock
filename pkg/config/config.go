package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// 服务器配置的环境变量覆盖项（容器部署用）。优先级：env > config.yaml > 默认值。
const (
	// EnvServerPort 覆盖 Server.Port
	EnvServerPort = "TONGSTOCK_SERVER_PORT"
	// EnvServerBindAddress 覆盖 Server.BindAddress
	EnvServerBindAddress = "TONGSTOCK_SERVER_BIND_ADDRESS"
	// EnvServerAccessToken 覆盖 Server.AccessToken
	EnvServerAccessToken = "TONGSTOCK_SERVER_ACCESS_TOKEN"
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

// AgentConfig AI Agent 配置。模型访问统一走 TongStock 自有的
// OpenAI-compatible 适配层，配置只有一组字段：provider / base_url /
// api_key_env / model（外加 timeout / max_tokens / streaming 三个可选项）。
type AgentConfig struct {
	Enabled bool `yaml:"enabled"`
	// Provider is the provider family name used for diagnostics and the
	// default API-key environment variable, e.g. "deepseek" or "openai".
	Provider string `yaml:"provider"`
	// BaseURL is the OpenAI-compatible endpoint, e.g.
	// "https://api.deepseek.com/v1". Required when the agent capability runs a
	// remote provider.
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
	// Timeout is the per-request provider timeout as a Go duration string
	// ("60s", "1m30s"). Empty keeps the adapter default.
	Timeout string `yaml:"timeout"`
	// MaxTokens caps the completion length for every request. Zero keeps the
	// adapter default.
	MaxTokens int `yaml:"max_tokens"`
	// Streaming records the preferred response mode. It is a caller-side
	// preference: the HTTP layer still streams when the client asks for it.
	Streaming bool `yaml:"streaming"`
	// AgentPaths is an ordered, file-level replacement list. It does not merge
	// with defaults when a configuration file supplies one or more paths.
	AgentPaths []string `yaml:"agent_paths"`
	Model      string   `yaml:"model"`
	Session    string   `yaml:"session"`
	// Defaults maps a usage scenario to the default agent ID. It is the
	// single place where "which agent serves which scene" is declared;
	// both the HTTP handlers and the web UI read it through
	// /api/agent/state.
	Defaults AgentScenarioDefaults `yaml:"defaults"`

	// Agent and StockAgent are the legacy flat fields. They still load and
	// feed the corresponding scenario when that scenario is not set under
	// defaults, so existing config.yaml files keep working. Prefer defaults.
	Agent      string `yaml:"agent"`
	StockAgent string `yaml:"stock_agent"`

	// Retired legacy fields. They are parsed only so Load can reject them with
	// an actionable migration error (see Validate); nothing reads their
	// values. Keeping the YAML tags is what makes the rejection possible at
	// all: without them an old config would be silently ignored.
	LegacyBackend string `yaml:"backend"`
	LegacyHome    string `yaml:"home"`
	LegacyConfig  string `yaml:"config"`
	LegacyAPIBase string `yaml:"api_base"`
}

// AgentScenarioDefaults declares the default agent per usage scenario.
type AgentScenarioDefaults struct {
	// Chat is the general conversation default: the /agent page, API
	// requests without an explicit agent, and transcript lookup.
	Chat string `yaml:"chat"`
	// Stock is the stock-detail analysis panel default.
	Stock string `yaml:"stock"`
}

// ErrLegacyAgentConfig reports an agent section that still relies on the
// PicoClaw-era fields (backend / home / config / api_base). TongStock has a
// single runtime now, so these fields are a hard configuration error instead
// of a silent backend switch.
var ErrLegacyAgentConfig = errors.New("agent 配置使用了已退役字段")

// MigrationDocHint points at the field-by-field migration table.
const MigrationDocHint = "迁移表见 README.md「Agent 配置迁移」"

// Validate rejects retired fields and malformed option values. Every caller
// that reads config.yaml must run it before using the agent section: a
// leftover `backend: picoclaw` has to fail loudly rather than select a
// runtime that no longer exists.
func (c AgentConfig) Validate() error {
	var legacy []string
	for _, field := range []struct{ name, value string }{
		{"backend", c.LegacyBackend},
		{"home", c.LegacyHome},
		{"config", c.LegacyConfig},
	} {
		if v := strings.TrimSpace(field.value); v != "" {
			legacy = append(legacy, field.name+": "+v)
		}
	}
	if len(legacy) > 0 {
		return fmt.Errorf("%w（%s）：请改用 provider / base_url / api_key_env / model，%s",
			ErrLegacyAgentConfig, strings.Join(legacy, "，"), MigrationDocHint)
	}
	if v := strings.TrimSpace(c.LegacyAPIBase); v != "" {
		return fmt.Errorf("%w（api_base: %s）：请改用 base_url，%s", ErrLegacyAgentConfig, v, MigrationDocHint)
	}
	if _, err := c.ParseTimeout(); err != nil {
		return err
	}
	return nil
}

// ParseTimeout turns the YAML timeout string into a duration. Empty means the
// adapter default applies.
func (c AgentConfig) ParseTimeout() (time.Duration, error) {
	value := strings.TrimSpace(c.Timeout)
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("agent.timeout %q 不是合法的时长（示例：60s、1m30s）", c.Timeout)
	}
	return d, nil
}

// ExpandHome resolves a leading "~/" against the current user's home
// directory. Server and CLI both run agent paths through it so a single
// config.yaml value behaves identically in every entrypoint.
func ExpandHome(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	return value
}

// DefaultConfig 返回一个包含默认值的 Config 实例
func DefaultConfig() *Config {
	return &Config{
		Server:   ServerConfig{Port: 8106, BindAddress: "127.0.0.1"},
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
  port: 8106
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
  # TongStock 正式仅支持 sqlite3
  driver: sqlite3
  # 连接字符串
  # dsn: ~/.tongstock/cache/tongstock.db

# AI Agent 配置（可选；Server 与 CLI 共用同一段配置）
# 模型访问统一走 TongStock 自有的 OpenAI-compatible 适配层。
# 旧字段 backend / home / config 已退役：仍配置它们会在启动时直接报错，
# 字段对应关系见 README「Agent 配置迁移」。
# agent:
#   enabled: true
#   provider: deepseek
#   base_url: https://api.deepseek.com/v1
#   api_key_env: DEEPSEEK_API_KEY
#   model: deepseek-chat
#   # timeout: 60s          # 单次请求超时，留空用适配层默认值
#   # max_tokens: 4096      # 单次补全 token 上限，留空用适配层默认值
#   # streaming: true       # 优先流式返回（客户端仍可选择非流式接口）
#   # agent_paths: [~/.tongstock/agents]
#   session: ""
#   # 按场景声明默认角色（HTTP handler 和 Web UI 都从这里取）
#   defaults:
#     chat: stock-analyst    # 通用对话：/agent 页、不带 agent 的 API 请求
#     stock: stock-analyst   # 个股详情分析面板
#
# 常见兼容端点：DeepSeek https://api.deepseek.com/v1、
# OpenRouter https://openrouter.ai/api/v1、Ollama http://localhost:11434/v1、
# vLLM http://localhost:8000/v1。
`
}

// Load 读取并合并配置，若无配置文件则写入默认模板并返回默认配置。
// 合并优先级：环境变量 > config.yaml > 内置默认值。
func Load() (*Config, error) {
	if err := EnsureHomeDir(); err != nil {
		return nil, err
	}
	path := ConfigPath()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := writeDefaultConfig(path); err != nil {
			return nil, err
		}
		cfg := DefaultConfig()
		applyEnvOverrides(cfg)
		return cfg, nil
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseConfig(data)
}

// parseConfig validates and merges one config.yaml payload. It is the single
// place where retired agent fields are rejected, shared by Load and tests.
// 合并优先级：环境变量 > config.yaml > 内置默认值。
func parseConfig(data []byte) (*Config, error) {
	tmp := &Config{}
	if err := yaml.Unmarshal(data, tmp); err != nil {
		return nil, err
	}
	if err := tmp.Agent.Validate(); err != nil {
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
	if tmp.Agent.Model != "" {
		merged.Agent.Model = tmp.Agent.Model
	}
	if tmp.Agent.Provider != "" {
		merged.Agent.Provider = tmp.Agent.Provider
	}
	if tmp.Agent.BaseURL != "" {
		merged.Agent.BaseURL = tmp.Agent.BaseURL
	}
	if tmp.Agent.APIKeyEnv != "" {
		merged.Agent.APIKeyEnv = tmp.Agent.APIKeyEnv
	}
	if tmp.Agent.Timeout != "" {
		merged.Agent.Timeout = tmp.Agent.Timeout
	}
	if tmp.Agent.MaxTokens != 0 {
		merged.Agent.MaxTokens = tmp.Agent.MaxTokens
	}
	if tmp.Agent.Streaming {
		merged.Agent.Streaming = tmp.Agent.Streaming
	}
	if len(tmp.Agent.AgentPaths) > 0 {
		// A configured list intentionally replaces the default list. Path order is
		// significant because later custom agent definitions override earlier ones.
		merged.Agent.AgentPaths = append([]string(nil), tmp.Agent.AgentPaths...)
	}
	if tmp.Agent.Agent != "" {
		merged.Agent.Agent = tmp.Agent.Agent
	}
	if tmp.Agent.Session != "" {
		merged.Agent.Session = tmp.Agent.Session
	}
	if tmp.Agent.Defaults.Chat != "" {
		merged.Agent.Defaults.Chat = tmp.Agent.Defaults.Chat
	}
	if tmp.Agent.Defaults.Stock != "" {
		merged.Agent.Defaults.Stock = tmp.Agent.Defaults.Stock
	}
	if tmp.Agent.StockAgent != "" {
		merged.Agent.StockAgent = tmp.Agent.StockAgent
	}

	applyEnvOverrides(merged)
	return merged, nil
}

// applyEnvOverrides 用环境变量覆盖服务器配置，供容器部署通过 compose 的
// environment/secret 注入端口、监听地址与 access_token，密钥无需落盘。
// 仅覆盖实际设置了的变量；非法值（如非数字端口）保持原配置不变。
func applyEnvOverrides(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv(EnvServerPort)); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			cfg.Server.Port = port
		}
	}
	if v := strings.TrimSpace(os.Getenv(EnvServerBindAddress)); v != "" {
		cfg.Server.BindAddress = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvServerAccessToken)); v != "" {
		cfg.Server.AccessToken = v
	}
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
