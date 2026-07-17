package picoclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcbus "github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

type DirectRunner struct {
	rt             *Runtime
	defaultAgent   string
	modelOverride  string
	loop           *pcagent.AgentLoop
	closeProvider  func()
	embeddedAgents []EmbeddedAgent
	workspace      string
	onDelta        func(string)
	closeOnce      sync.Once
}

func (rt *Runtime) NewDirectRunner(opt RunOptions) (*DirectRunner, error) {
	if rt == nil || rt.Config == nil {
		return nil, fmt.Errorf("picoclaw runtime not loaded")
	}

	resolved, err := rt.ResolveRunOptions(opt)
	if err != nil {
		return nil, err
	}

	cfg := cloneConfig(rt.Config)
	workspace := resolveRunWorkspace(opt.Workspace)
	if workspace != "" {
		cfg = configureProjectWorkspace(cfg, workspace)
	}
	cfg = prepareConfigForRun(cfg, resolved)
	embeddedAgents := opt.embeddedAgents()
	if err := applyEmbeddedAgentConfigs(cfg, embeddedAgents, resolved.Model); err != nil {
		return nil, err
	}
	configureLogging(opt)
	if str(resolved.Model) != "" {
		cfg.Agents.Defaults.ModelName = str(resolved.Model)
	}
	if opt.OnDelta != nil {
		enableDirectStreaming(cfg, str(resolved.Model))
	}

	provider, modelID, err := pcproviders.CreateProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("create picoclaw provider failed: %w", err)
	}
	closeProvider := func() {}
	if stateful, ok := provider.(pcproviders.StatefulProvider); ok {
		closeProvider = stateful.Close
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	loop := newAgentLoopWithStream(cfg, provider, opt.OnDelta)
	if err := registerEmbeddedAgentPrompts(loop, embeddedAgents); err != nil {
		loop.Close()
		closeProvider()
		return nil, err
	}

	return &DirectRunner{
		rt:             rt,
		defaultAgent:   DefaultAgent(cfg),
		modelOverride:  str(opt.Model),
		loop:           loop,
		closeProvider:  closeProvider,
		embeddedAgents: embeddedAgents,
		workspace:      workspace,
		onDelta:        opt.OnDelta,
	}, nil
}

func (dr *DirectRunner) Close() {
	if dr == nil {
		return
	}
	dr.closeOnce.Do(func() {
		if dr.loop != nil {
			dr.loop.Close()
			dr.loop = nil
		}
		if dr.closeProvider != nil {
			dr.closeProvider()
			dr.closeProvider = nil
		}
	})
}

func (dr *DirectRunner) ProcessDirect(opt RunOptions) (string, error) {
	return dr.ProcessDirectContext(context.Background(), opt)
}

func (dr *DirectRunner) ProcessDirectContext(ctx context.Context, opt RunOptions) (string, error) {
	if dr == nil || dr.rt == nil || dr.loop == nil {
		return "", fmt.Errorf("picoclaw direct runner not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if str(opt.Model) == "" {
		opt.Model = dr.modelOverride
	}
	if len(opt.EmbeddedAgents) == 0 && len(dr.embeddedAgents) > 0 {
		opt.EmbeddedAgents = dr.embeddedAgents
	}
	if opt.OnDelta == nil && dr.onDelta != nil {
		opt.OnDelta = dr.onDelta
	}
	if workspace := resolveRunWorkspace(opt.Workspace); workspace != "" && !sameWorkspace(workspace, dr.workspace) {
		child, err := dr.rt.NewDirectRunner(RunOptions{
			Model:          str(opt.Model),
			Workspace:      workspace,
			Debug:          opt.Debug,
			Quiet:          opt.Quiet,
			EmbeddedAgents: opt.EmbeddedAgents,
			OnDelta:        opt.OnDelta,
		})
		if err != nil {
			return "", err
		}
		defer child.Close()
		opt.Workspace = ""
		return child.ProcessDirectContext(ctx, opt)
	}
	resolved, err := dr.rt.ResolveRunOptions(opt)
	if err != nil {
		return "", err
	}

	return processDirect(dr.loop, resolved.Message, resolved.Session, resolved.Agent, dr.defaultAgent)
}

func newAgentLoopWithStream(cfg *pcconfig.Config, provider pcproviders.LLMProvider, onDelta func(string)) *pcagent.AgentLoop {
	msgBus := newMessageBus()
	if onDelta != nil {
		msgBus.SetStreamDelegate(directStreamDelegate{onDelta: onDelta})
	}
	return pcagent.NewAgentLoop(cfg, msgBus, provider)
}

type directStreamDelegate struct {
	onDelta func(string)
}

func (d directStreamDelegate) GetStreamer(ctx context.Context, channel, chatID, sessionKey string) (pcbus.Streamer, bool) {
	if d.onDelta == nil {
		return nil, false
	}
	return &directStreamer{onDelta: d.onDelta}, true
}

type directStreamer struct {
	onDelta func(string)
	mu      sync.Mutex
	last    string
}

func (s *directStreamer) Update(ctx context.Context, content string) error {
	s.emitDelta(content)
	return nil
}

func (s *directStreamer) Finalize(ctx context.Context, content string) error {
	s.emitDelta(content)
	return nil
}

func (s *directStreamer) Cancel(context.Context) {}

func (s *directStreamer) emitDelta(content string) {
	if s == nil || s.onDelta == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if content == "" || content == s.last {
		return
	}
	delta := content
	if strings.HasPrefix(content, s.last) {
		delta = strings.TrimPrefix(content, s.last)
	}
	s.last = content
	if strings.TrimSpace(delta) != "" {
		s.onDelta(delta)
	}
}

func enableDirectStreaming(cfg *pcconfig.Config, model string) {
	if cfg == nil {
		return
	}
	if cfg.Channels == nil {
		cfg.Channels = pcconfig.ChannelsConfig{}
	}
	settings, _ := json.Marshal(map[string]any{"streaming": map[string]any{"enabled": true}})
	ch := cfg.Channels["cli"]
	if ch == nil {
		ch = &pcconfig.Channel{Enabled: true, Type: "pico"}
		cfg.Channels["cli"] = ch
	}
	ch.Enabled = true
	if strings.TrimSpace(ch.Type) == "" {
		ch.Type = "pico"
	}
	ch.Settings = pcconfig.RawNode(settings)

	model = strings.TrimSpace(model)
	for _, modelCfg := range cfg.ModelList {
		if modelCfg == nil {
			continue
		}
		if model == "" || strings.EqualFold(modelCfg.ModelName, model) || strings.EqualFold(modelCfg.Model, model) {
			modelCfg.Streaming.Enabled = true
		}
	}
}

func sameWorkspace(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return a == b
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
