package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestParseConfigReadsAgentProviderFields(t *testing.T) {
	cfg, err := parseConfig([]byte(`
agent:
  enabled: true
  provider: deepseek
  base_url: https://api.deepseek.com/v1
  api_key_env: DEEPSEEK_API_KEY
  model: deepseek-chat
  session: research
  timeout: 90s
  max_tokens: 2048
  streaming: true
  agent_paths: [~/.tongstock/agents]
  defaults:
    chat: stock-analyst
`))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	agent := cfg.Agent
	if !agent.Enabled || agent.Provider != "deepseek" || agent.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("agent = %+v, want provider/base_url parsed", agent)
	}
	if agent.APIKeyEnv != "DEEPSEEK_API_KEY" || agent.Model != "deepseek-chat" || agent.Session != "research" {
		t.Fatalf("agent = %+v, want api_key_env/model/session parsed", agent)
	}
	if agent.Timeout != "90s" || agent.MaxTokens != 2048 || !agent.Streaming {
		t.Fatalf("agent = %+v, want timeout/max_tokens/streaming parsed", agent)
	}
	if len(agent.AgentPaths) != 1 || agent.AgentPaths[0] != "~/.tongstock/agents" {
		t.Fatalf("agent paths = %#v", agent.AgentPaths)
	}
	if agent.Defaults.Chat != "stock-analyst" {
		t.Fatalf("defaults.chat = %q", agent.Defaults.Chat)
	}
	if d, err := agent.ParseTimeout(); err != nil || d != 90*time.Second {
		t.Fatalf("ParseTimeout() = (%v, %v), want 90s", d, err)
	}
}

// legacyRuntimeValue is what real upgraded configs carry in the retired
// `backend` key. Validate does not special-case it (any non-empty value is
// rejected), but the fixture keeps it so the production scenario stays
// covered verbatim.
const legacyRuntimeValue = "picoclaw"

// Retired runtime-era fields must fail loudly with an actionable message.
// Silently ignoring them would leave the user pointing at a runtime that no
// longer exists.
func TestParseConfigRejectsRetiredAgentFields(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantHint []string
	}{
		{
			name:     "backend runtime name",
			yaml:     "agent:\n  enabled: true\n  backend: " + legacyRuntimeValue + "\n",
			wantHint: []string{"backend: " + legacyRuntimeValue, "provider", "base_url", "api_key_env", "model", "Agent 配置迁移"},
		},
		{
			name:     "home only",
			yaml:     "agent:\n  enabled: true\n  home: /opt/legacy-agent\n",
			wantHint: []string{"home: /opt/legacy-agent", "provider", "base_url", "api_key_env", "model", "Agent 配置迁移"},
		},
		{
			name:     "config only",
			yaml:     "agent:\n  enabled: true\n  config: /opt/legacy-agent/config.json\n",
			wantHint: []string{"config: /opt/legacy-agent/config.json", "provider", "base_url", "api_key_env", "model", "Agent 配置迁移"},
		},
		{
			name:     "api_base rename",
			yaml:     "agent:\n  enabled: true\n  api_base: https://api.deepseek.com/v1\n",
			wantHint: []string{"api_base", "base_url", "Agent 配置迁移"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig([]byte(tt.yaml))
			if !errors.Is(err, ErrLegacyAgentConfig) {
				t.Fatalf("error = %v, want ErrLegacyAgentConfig", err)
			}
			for _, hint := range tt.wantHint {
				if !strings.Contains(err.Error(), hint) {
					t.Fatalf("error = %q, want it to mention %q", err, hint)
				}
			}
		})
	}
}

func TestParseConfigRejectsInvalidTimeout(t *testing.T) {
	_, err := parseConfig([]byte("agent:\n  timeout: soon\n"))
	if err == nil || !strings.Contains(err.Error(), "agent.timeout") {
		t.Fatalf("error = %v, want explicit agent.timeout error", err)
	}
}

// Empty agent/session/model must keep parsing and stay empty so the service
// layer applies its defaults; an absent agent section is not an error.
func TestParseConfigAllowsEmptyAgentDefaults(t *testing.T) {
	cfg, err := parseConfig([]byte("server:\n  port: 8200\n"))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Agent.Enabled || cfg.Agent.Model != "" || cfg.Agent.Session != "" || cfg.Agent.BaseURL != "" {
		t.Fatalf("agent = %+v, want zero values from an absent section", cfg.Agent)
	}
	if cfg.Server.Port != 8200 {
		t.Fatalf("server.port = %d, want 8200", cfg.Server.Port)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no user home directory")
	}
	if got := ExpandHome("~/agents"); got != filepath.Join(home, "agents") {
		t.Fatalf("ExpandHome(~/agents) = %q, want %q", got, filepath.Join(home, "agents"))
	}
	if got := ExpandHome(" /tmp/x "); got != "/tmp/x" {
		t.Fatalf("ExpandHome passthrough = %q, want trimmed input", got)
	}
}

func TestDefaultConfigTemplateIsValidYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(defaultConfigTemplate()), &cfg); err != nil {
		t.Fatalf("default config template is invalid: %v", err)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	t.Setenv(EnvServerPort, "9999")
	t.Setenv(EnvServerBindAddress, "0.0.0.0")
	t.Setenv(EnvServerAccessToken, "test123")

	cfg := DefaultConfig()
	// 模拟 config.yaml 里已有的值：env 优先级更高，必须覆盖
	cfg.Server.Port = 8106
	cfg.Server.BindAddress = "127.0.0.1"
	cfg.Server.AccessToken = "from-yaml"
	applyEnvOverrides(cfg)

	if cfg.Server.Port != 9999 {
		t.Errorf("Port = %d, want 9999 (env override)", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "0.0.0.0" {
		t.Errorf("BindAddress = %q, want %q (env override)", cfg.Server.BindAddress, "0.0.0.0")
	}
	if cfg.Server.AccessToken != "test123" {
		t.Errorf("AccessToken = %q, want %q (env beats yaml)", cfg.Server.AccessToken, "test123")
	}
}

func TestApplyEnvOverridesNoEnvKeepsConfig(t *testing.T) {
	// 确保没有相关环境变量时不改动配置（防止继承自 shell 的同名变量干扰断言）
	t.Setenv(EnvServerPort, "")
	t.Setenv(EnvServerBindAddress, "")
	t.Setenv(EnvServerAccessToken, "")

	cfg := DefaultConfig()
	cfg.Server.Port = 8200
	cfg.Server.BindAddress = "127.0.0.1"
	cfg.Server.AccessToken = "keep"
	applyEnvOverrides(cfg)

	if cfg.Server.Port != 8200 || cfg.Server.BindAddress != "127.0.0.1" || cfg.Server.AccessToken != "keep" {
		t.Fatalf("empty env must not override: got %+v", cfg.Server)
	}
}

func TestApplyEnvOverridesInvalidPortIgnored(t *testing.T) {
	t.Setenv(EnvServerPort, "not-a-number")

	cfg := DefaultConfig()
	cfg.Server.Port = 8106
	applyEnvOverrides(cfg)

	if cfg.Server.Port != 8106 {
		t.Errorf("invalid port must be ignored, got %d", cfg.Server.Port)
	}
}

func TestApplyEnvOverridesPartial(t *testing.T) {
	// 只设置 token 一项，其余字段不动 —— 对应 compose 只注入密钥的场景
	t.Setenv(EnvServerAccessToken, "only-token")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Server.AccessToken != "only-token" {
		t.Errorf("AccessToken = %q, want %q", cfg.Server.AccessToken, "only-token")
	}
	if cfg.Server.Port != 8106 || cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("unrelated fields must keep defaults: got %+v", cfg.Server)
	}
}
