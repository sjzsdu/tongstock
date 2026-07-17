package picoclaw

import (
	"fmt"
	"os"
	"path/filepath"

	pcpkg "github.com/sipeed/picoclaw/pkg"
	pcbus "github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pcskills "github.com/sipeed/picoclaw/pkg/skills"
)

type Options struct {
	Home   string
	Config string
	Model  string
}

type Runtime struct {
	Home       string
	ConfigPath string
	Config     *pcconfig.Config
	Skills     []pcskills.SkillInfo
}

func Load(opt Options) (*Runtime, error) {
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
