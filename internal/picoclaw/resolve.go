package picoclaw

import (
	"fmt"
	"strings"

	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

type RunOptions struct {
	Message        string
	Session        string
	Agent          string
	Model          string
	Workspace      string
	Debug          bool
	Quiet          bool
	EmbeddedAgents []EmbeddedAgent
	BeforeOutput   func()
	OnDelta        func(string)
}

type EmbeddedAgent struct {
	ID          string
	Name        string
	Description string
	Prompt      string
	Soul        string
	Aliases     []string
	Skills      []string
	Tools       []string
	NoHistory   bool
}

type ResolvedRunOptions struct {
	Message string `json:"message,omitempty"`
	Session string `json:"session"`
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model"`
}

func (rt *Runtime) ResolveRunOptions(opt RunOptions) (ResolvedRunOptions, error) {
	if rt == nil || rt.Config == nil {
		return ResolvedRunOptions{}, fmt.Errorf("picoclaw runtime not loaded")
	}

	resolved := ResolvedRunOptions{
		Message: str(opt.Message),
		Session: str(opt.Session),
		Agent:   str(opt.Agent),
		Model:   str(opt.Model),
	}

	if resolved.Session == "" {
		resolved.Session = defaultSession
	}

	var agentCfg *pcconfig.AgentConfig
	if embedded, ok := opt.findEmbeddedAgent(resolved.Agent); ok {
		resolved.Agent = str(embedded.ID)
	} else {
		var err error
		agentCfg, err = rt.resolveAgentConfig(resolved.Agent)
		if err != nil {
			return ResolvedRunOptions{}, err
		}
		if agentCfg != nil {
			resolved.Agent = str(agentCfg.ID)
		}
	}

	if resolved.Model == "" && agentCfg != nil && agentCfg.Model != nil {
		resolved.Model = str(agentCfg.Model.Primary)
	}
	if resolved.Model == "" {
		resolved.Model = rt.PreferredModelName()
	}
	if resolved.Model == "" {
		return ResolvedRunOptions{}, fmt.Errorf("no model specified and no default model configured")
	}

	if opt.Model == "" && str(opt.Agent) == "" {
		resolved.Model = rt.safeModelForAgent(resolved.Model)
	}

	if _, err := rt.ResolveModel(resolved.Model); err != nil {
		return ResolvedRunOptions{}, err
	}

	return resolved, nil
}

func (opt RunOptions) embeddedAgents() []EmbeddedAgent {
	return append([]EmbeddedAgent(nil), opt.EmbeddedAgents...)
}

func (opt RunOptions) findEmbeddedAgent(id string) (EmbeddedAgent, bool) {
	want := str(id)
	if want == "" {
		return EmbeddedAgent{}, false
	}
	for _, agent := range opt.embeddedAgents() {
		if strings.EqualFold(str(agent.ID), want) {
			return agent, true
		}
	}
	return EmbeddedAgent{}, false
}

func (rt *Runtime) safeModelForAgent(name string) string {
	model := str(name)
	if model == "" {
		return model
	}
	cfg, err := rt.ResolveModel(model)
	if err != nil || cfg == nil {
		return model
	}
	protocol, _ := pcproviders.ExtractProtocol(cfg)
	if protocol != "ollama" {
		return model
	}
	fallback := str(rt.PreferredModelName())
	if fallback == "" || strings.EqualFold(fallback, model) {
		return model
	}
	fallbackCfg, err := rt.ResolveModel(fallback)
	if err != nil || fallbackCfg == nil {
		return model
	}
	fallbackProtocol, _ := pcproviders.ExtractProtocol(fallbackCfg)
	if fallbackProtocol == "ollama" {
		return model
	}
	return fallback
}

func (rt *Runtime) resolveAgentConfig(name string) (*pcconfig.AgentConfig, error) {
	if rt == nil || rt.Config == nil {
		return nil, fmt.Errorf("picoclaw runtime not loaded")
	}
	want := str(name)
	if want == "" {
		return defaultAgentConfig(rt.Config), nil
	}
	for i := range rt.Config.Agents.List {
		item := &rt.Config.Agents.List[i]
		if strings.EqualFold(str(item.ID), want) {
			return item, nil
		}
		if strings.EqualFold(str(item.Name), want) {
			return item, nil
		}
	}
	if strings.EqualFold(want, defaultAgentID) {
		return defaultAgentConfig(rt.Config), nil
	}
	return nil, fmt.Errorf("agent %q not found", want)
}

func defaultAgentConfig(cfg *pcconfig.Config) *pcconfig.AgentConfig {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Agents.List {
		if strings.EqualFold(str(cfg.Agents.List[i].ID), DefaultAgent(cfg)) {
			return &cfg.Agents.List[i]
		}
	}
	return nil
}
