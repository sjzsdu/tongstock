package picoclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pcpkg "github.com/sipeed/picoclaw/pkg"
	pcbus "github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pcskills "github.com/sipeed/picoclaw/pkg/skills"
)

type Options struct {
	Backend   string
	Home      string
	Config    string
	Model     string
	Provider  string
	APIBase   string
	APIKeyEnv string
}

type Runtime struct {
	Home       string
	ConfigPath string
	Config     *pcconfig.Config
	Skills     []pcskills.SkillInfo
}

func Load(opt Options) (*Runtime, error) {
	if strings.EqualFold(str(opt.Backend), "builtin") {
		return loadBuiltin(opt)
	}
	if backend := str(opt.Backend); backend != "" && !strings.EqualFold(backend, "picoclaw") {
		return nil, fmt.Errorf("unsupported agent backend %q", backend)
	}
	return loadPicoClaw(opt)
}

func loadPicoClaw(opt Options) (*Runtime, error) {
	home := resolveHome(opt.Home)
	cfgPath := resolveConfigPath(home, opt.Config)

	prevHome, hasHome := os.LookupEnv(pcconfig.EnvHome)
	prevCfg, hasCfg := os.LookupEnv(pcconfig.EnvConfig)
	defer restoreEnv(pcconfig.EnvHome, prevHome, hasHome)
	defer restoreEnv(pcconfig.EnvConfig, prevCfg, hasCfg)
	_ = os.Setenv(pcconfig.EnvHome, home)
	_ = os.Setenv(pcconfig.EnvConfig, cfgPath)

	cfg, err := pcconfig.LoadConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load picoclaw config failed: %w", err)
	}

	rt := &Runtime{
		Home:       home,
		ConfigPath: cfgPath,
		Config:     cfg,
		Skills:     loadSkills(cfg, home),
	}
	return rt, nil
}

func loadBuiltin(opt Options) (*Runtime, error) {
	model := str(opt.Model)
	if model == "" {
		return nil, fmt.Errorf("agent.model is required when agent backend resolves to builtin; configure agent.model or set backend: picoclaw")
	}
	home := str(opt.Home)
	if home == "" {
		home = defaultBuiltinHome()
	}
	provider := strings.ToLower(str(opt.Provider))
	if provider == "" {
		return nil, fmt.Errorf("agent.provider is required for builtin backend")
	}
	apiKeyEnv := str(opt.APIKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = defaultAPIKeyEnv(provider)
	}
	apiKey := ""
	if apiKeyEnv != "" {
		apiKey = strings.TrimSpace(os.Getenv(apiKeyEnv))
		if apiKey == "" {
			return nil, fmt.Errorf("agent api key environment variable %s is not set or is empty", apiKeyEnv)
		}
	} else if !providerAllowsEmptyAPIKey(provider) {
		return nil, fmt.Errorf("agent.api_key_env is required for remote provider %q", provider)
	}

	cfg := pcconfig.DefaultConfig()
	cfg.Agents.Defaults.ModelName = model
	cfg.Agents.Defaults.Workspace = filepath.Join(home, "workspace")
	cfg.ModelList = []*pcconfig.ModelConfig{{
		ModelName: model,
		Provider:  provider,
		Model:     model,
		APIBase:   str(opt.APIBase),
		APIKeys:   pcconfig.SimpleSecureStrings(apiKey),
		Enabled:   true,
	}}
	return &Runtime{
		Home:       home,
		ConfigPath: "",
		Config:     cfg,
		Skills:     loadSkills(cfg, home),
	}, nil
}

func defaultBuiltinHome() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".tongstock", "agent-runtime")
	}
	return filepath.Join(".tongstock", "agent-runtime")
}

func defaultAPIKeyEnv(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "zhipu":
		return "ZHIPU_API_KEY"
	case "ollama", "vllm", "lmstudio", "gpt4free", "claude-cli", "codex-cli":
		return ""
	default:
		return ""
	}
}

func providerAllowsEmptyAPIKey(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "ollama", "vllm", "lmstudio", "gpt4free", "claude-cli", "codex-cli":
		return true
	default:
		return false
	}
}

func resolveHome(home string) string {
	if str(home) != "" {
		return str(home)
	}
	if envHome := str(os.Getenv(pcconfig.EnvHome)); envHome != "" {
		return envHome
	}
	return pcconfig.GetHome()
}

func resolveConfigPath(home, cfg string) string {
	if str(cfg) != "" {
		return str(cfg)
	}
	if envCfg := str(os.Getenv(pcconfig.EnvConfig)); envCfg != "" {
		return envCfg
	}
	return filepath.Join(home, "config.json")
}

func restoreEnv(key, value string, ok bool) {
	if ok {
		_ = os.Setenv(key, value)
		return
	}
	_ = os.Unsetenv(key)
}

func loadSkills(cfg *pcconfig.Config, home string) []pcskills.SkillInfo {
	if cfg == nil {
		return nil
	}
	builtin := str(os.Getenv(pcconfig.EnvBuiltinSkills))
	loader := pcskills.NewSkillsLoader(cfg.WorkspacePath(), filepath.Join(home, "skills"), builtin)
	return loader.ListSkills()
}

func DefaultModel(cfg *pcconfig.Config) string {
	if cfg == nil {
		return ""
	}
	return str(cfg.Agents.Defaults.ModelName)
}

func Workspace(cfg *pcconfig.Config) string {
	if cfg == nil {
		return ""
	}
	ws := str(cfg.WorkspacePath())
	if ws != "" {
		return ws
	}
	home := pcconfig.GetHome()
	return filepath.Join(home, pcpkg.WorkspaceName)
}

func newMessageBus() *pcbus.MessageBus {
	return pcbus.NewMessageBus()
}
