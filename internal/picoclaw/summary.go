package picoclaw

import (
	"fmt"
	"sort"
	"strings"

	pcconfig "github.com/sipeed/picoclaw/pkg/config"
)

type Summary struct {
	Home         string   `json:"home"`
	ConfigPath   string   `json:"config_path"`
	Workspace    string   `json:"workspace"`
	DefaultAgent string   `json:"default_agent,omitempty"`
	DefaultModel string   `json:"default_model"`
	Agents       []string `json:"agents,omitempty"`
	Models       []string `json:"models"`
	Skills       []string `json:"skills"`
}

func (rt *Runtime) Summary() Summary {
	return Summary{
		Home:         rt.Home,
		ConfigPath:   rt.ConfigPath,
		Workspace:    Workspace(rt.Config),
		DefaultAgent: DefaultAgent(rt.Config),
		DefaultModel: rt.PreferredModelName(),
		Agents:       agentNames(rt.Config),
		Models:       availableModelNames(rt.Config),
		Skills:       skillNames(rt),
	}
}

func DefaultAgent(cfg *pcconfig.Config) string {
	if cfg == nil {
		return defaultAgentID
	}
	for _, item := range cfg.Agents.List {
		if item.Default {
			return str(item.ID)
		}
	}
	return defaultAgentID
}

func (rt *Runtime) ResolveModel(name string) (*pcconfig.ModelConfig, error) {
	if rt == nil || rt.Config == nil {
		return nil, fmt.Errorf("picoclaw runtime not loaded")
	}
	model := str(name)
	if model == "" {
		model = rt.PreferredModelName()
	}
	if model == "" {
		return nil, fmt.Errorf("no model specified and no default model configured")
	}
	return rt.Config.GetModelConfig(model)
}

func availableModelNames(cfg *pcconfig.Config) []string {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(cfg.ModelList))
	for _, item := range cfg.ModelList {
		if !isAvailableModel(item) {
			continue
		}
		name := str(item.ModelName)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func agentNames(cfg *pcconfig.Config) []string {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(cfg.Agents.List)+1)
	seen[DefaultAgent(cfg)] = struct{}{}
	out = append(out, DefaultAgent(cfg))
	for _, item := range cfg.Agents.List {
		name := str(item.ID)
		if name == "" {
			name = str(item.Name)
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func skillNames(rt *Runtime) []string {
	if rt == nil {
		return nil
	}
	out := make([]string, 0, len(rt.Skills))
	for _, skill := range rt.Skills {
		name := str(skill.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (rt *Runtime) PreferredModelName() string {
	if rt == nil || rt.Config == nil {
		return ""
	}
	return preferredModelName(rt.Config)
}

func preferredModelName(cfg *pcconfig.Config) string {
	if cfg == nil {
		return ""
	}
	if hasAvailableModel(cfg, defaultModel) {
		return defaultModel
	}
	if name := str(cfg.Agents.Defaults.ModelName); hasAvailableModel(cfg, name) {
		return name
	}
	for _, name := range cfg.Agents.Defaults.ModelFallbacks {
		name = str(name)
		if hasAvailableModel(cfg, name) {
			return name
		}
	}
	for _, item := range cfg.ModelList {
		if isAvailableModel(item) {
			return str(item.ModelName)
		}
	}
	return ""
}

func hasAvailableModel(cfg *pcconfig.Config, name string) bool {
	name = str(name)
	if cfg == nil || name == "" {
		return false
	}
	for _, item := range cfg.ModelList {
		if str(item.ModelName) != name {
			continue
		}
		if isAvailableModel(item) {
			return true
		}
	}
	return false
}

func isAvailableModel(item *pcconfig.ModelConfig) bool {
	if item == nil {
		return false
	}
	if str(item.ModelName) == "" || str(item.Model) == "" {
		return false
	}
	return strings.EqualFold(str(item.LastTestStatus), "ok")
}
