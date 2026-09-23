// Package agentservice is the single entrypoint for the agent capability.
// It turns config.AgentConfig into a runnable service: the PicoClaw/builtin
// runtime, the embedded plus user-defined agent table, and the per-request
// runner lifecycle all live behind one constructor and one Run method, so the
// HTTP server, the CLI, and any future subsystem share the same configuration
// resolution and execution semantics.
package agentservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/internal/agents"
	"github.com/sjzsdu/tongstock/internal/picoclaw"
	"github.com/sjzsdu/tongstock/pkg/config"
)

// ErrDisabled reports that the agent capability is switched off in
// config.yaml (agent.enabled).
var ErrDisabled = errors.New("AI agent 未启用：请在配置中设置 agent.enabled=true")

// ErrRunner wraps runner-construction failures so callers can still tell
// setup errors apart from execution errors (the SSE stream uses distinct
// codes for each).
var ErrRunner = errors.New("agent runner initialization failed")

const defaultSession = "tongstock:default"

const (
	// ScenarioChat is the general conversation scenario: the /agent page,
	// API requests without an explicit agent, and transcript lookup.
	ScenarioChat = "chat"
	// ScenarioStock is the stock-detail analysis panel scenario.
	ScenarioStock = "stock"
)

// Config is the fully resolved runtime configuration for the agent
// capability. It is the transport-neutral form of config.AgentConfig: build it
// with FromAgentConfig instead of filling it in by hand.
type Config struct {
	// Enabled gates the capability. Only FromAgentConfig/Open consult it;
	// New treats an empty Config as explicitly opted in so programmatic
	// callers (tests, embedders) can construct a runtime directly.
	Enabled    bool
	Backend    string
	Home       string
	ConfigPath string
	Provider   string
	APIBase    string
	APIKeyEnv  string
	Model      string
	Session    string
	// DefaultAgents maps a scenario (ScenarioChat, ScenarioStock, ...) to
	// the agent ID that serves it. Empty scenarios are dropped on load.
	DefaultAgents map[string]string
	// Workspace overrides the default working directory (process cwd).
	Workspace string
	// AgentPaths is the ordered list of extra Markdown agent definitions.
	AgentPaths []string
}

// Defaults carries the effective default selection per scenario. The Agents
// map is the single source of truth for "which agent serves which scene";
// handlers and the web UI both read it (the latter via /api/agent/state).
type Defaults struct {
	Model   string            `json:"model"`
	Session string            `json:"session"`
	Debug   bool              `json:"debug"`
	Agents  map[string]string `json:"agents"` // scenario -> agent ID
}

// Agent returns the default agent for a scenario, or "" when unset.
func (d Defaults) Agent(scenario string) string {
	return strings.TrimSpace(d.Agents[scenario])
}

// Options customizes Service construction.
type Options struct {
	// Lister overrides agent definition loading. When nil the service loads
	// the embedded agents plus Config.AgentPaths itself, which is the single
	// definition source shared by server and CLI.
	Lister func() ([]picoclaw.EmbeddedAgent, error)
}

// Request is one agent invocation.
type Request struct {
	// Agent selects the target by canonical ID or alias; empty uses the
	// configured default agent.
	Agent string
	// Session empty falls back to Defaults.Session.
	Session string
	// Model empty falls back to Defaults.Model, then to the runtime default.
	Model string
	// Prompt is the user message.
	Prompt string
	// OnDelta, when set, receives streamed response chunks.
	OnDelta func(string)
}

// Service owns the runtime, the agent definitions, and the readiness runner.
// It is safe for concurrent use: every Run creates its own short-lived runner
// so parallel requests (e.g. agent debate) never serialize on model execution.
type Service struct {
	mu        sync.Mutex
	cfg       Config
	rt        *picoclaw.Runtime
	warm      *picoclaw.DirectRunner
	defs      []picoclaw.EmbeddedAgent
	defaults  Defaults
	workspace string
	started   time.Time
}

// FromAgentConfig converts the YAML agent section into a resolved runtime
// config: legacy backend detection, "~/" expansion, and the fold of the
// legacy flat agent/stock_agent fields into the scenario map happen here,
// once. Explicit defaults.* always win over the legacy fields.
func FromAgentConfig(cfg config.AgentConfig) Config {
	paths := make([]string, 0, len(cfg.AgentPaths))
	for _, path := range cfg.AgentPaths {
		if path = config.ExpandHome(path); path != "" {
			paths = append(paths, path)
		}
	}
	scenarios := make(map[string]string, 2)
	setScenario := func(scenario, primary, legacy string) {
		value := strings.TrimSpace(primary)
		if value == "" {
			value = strings.TrimSpace(legacy)
		}
		if value != "" {
			scenarios[scenario] = value
		}
	}
	setScenario(ScenarioChat, cfg.Defaults.Chat, cfg.Agent)
	setScenario(ScenarioStock, cfg.Defaults.Stock, cfg.StockAgent)
	return Config{
		Enabled:       cfg.Enabled,
		Backend:       cfg.EffectiveBackend(),
		Home:          config.ExpandHome(cfg.Home),
		ConfigPath:    config.ExpandHome(cfg.Config),
		Provider:      cfg.Provider,
		APIBase:       cfg.APIBase,
		APIKeyEnv:     cfg.APIKeyEnv,
		Model:         cfg.Model,
		Session:       cfg.Session,
		Workspace:     "",
		AgentPaths:    paths,
		DefaultAgents: scenarios,
	}
}

// Open is the config.yaml entrypoint: it enforces agent.enabled and then
// builds the service. CLI call sites should use Open; the server composition
// root gates on Enabled itself to report a distinct "disabled" module state
// and then calls New with FromAgentConfig.
func Open(cfg config.AgentConfig, opts Options) (*Service, error) {
	if !cfg.Enabled {
		return nil, ErrDisabled
	}
	return New(FromAgentConfig(cfg), opts)
}

// New builds the service from a resolved Config. It fails fast — runtime
// load, definition load, and the readiness runner are all validated here so
// callers degrade at startup instead of on the first request.
func New(cfg Config, opts Options) (*Service, error) {
	workspace := strings.TrimSpace(cfg.Workspace)
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	session := strings.TrimSpace(cfg.Session)
	if session == "" {
		session = defaultSession
	}
	scenarios := make(map[string]string, len(cfg.DefaultAgents))
	for scenario, id := range cfg.DefaultAgents {
		if id = strings.TrimSpace(id); id != "" {
			scenarios[scenario] = id
		}
	}
	// Effective default: the stock panel falls back to the chat default so
	// every declared scenario resolves to a usable agent.
	if scenarios[ScenarioStock] == "" && scenarios[ScenarioChat] != "" {
		scenarios[ScenarioStock] = scenarios[ScenarioChat]
	}
	chatAgent := scenarios[ScenarioChat]

	rt, err := picoclaw.Load(picoclaw.Options{
		Backend:   cfg.Backend,
		Home:      cfg.Home,
		Config:    cfg.ConfigPath,
		Provider:  cfg.Provider,
		APIBase:   cfg.APIBase,
		APIKeyEnv: cfg.APIKeyEnv,
		Model:     cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("load agent runtime failed: %w", err)
	}

	defs, err := loadDefinitions(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("load embedded agents failed: %w", err)
	}

	warm, err := rt.NewDirectRunner(picoclaw.RunOptions{
		Agent:          chatAgent,
		Model:          cfg.Model,
		Workspace:      workspace,
		Quiet:          true,
		EmbeddedAgents: defs,
	})
	if err != nil {
		return nil, fmt.Errorf("create direct runner failed: %w", err)
	}

	return &Service{
		cfg:       cfg,
		rt:        rt,
		warm:      warm,
		defs:      defs,
		workspace: workspace,
		started:   time.Now(),
		defaults: Defaults{
			Model:   cfg.Model,
			Session: session,
			Agents:  scenarios,
		},
	}, nil
}

func loadDefinitions(cfg Config, opts Options) ([]picoclaw.EmbeddedAgent, error) {
	if opts.Lister != nil {
		return opts.Lister()
	}
	return agents.ListWithPaths(cfg.AgentPaths)
}

// Run executes one request. Blank Agent/Session/Model fields fall back to the
// service defaults, and the underlying runner is created and closed per call.
func (s *Service) Run(ctx context.Context, req Request) (string, error) {
	if s == nil {
		return "", errors.New("agent service not initialized")
	}
	agent := strings.TrimSpace(req.Agent)
	if agent == "" {
		agent = s.defaults.Agent(ScenarioChat)
	}
	session := strings.TrimSpace(req.Session)
	if session == "" {
		session = s.defaults.Session
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = s.defaults.Model
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.rt == nil {
		s.mu.Unlock()
		return "", errors.New("agent service not initialized")
	}
	runner, err := s.rt.NewDirectRunner(picoclaw.RunOptions{
		Agent:          agent,
		Model:          model,
		Workspace:      s.workspace,
		Quiet:          true,
		EmbeddedAgents: s.defs,
		OnDelta:        req.OnDelta,
	})
	s.mu.Unlock()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRunner, err)
	}
	defer runner.Close()

	return runner.ProcessDirectContext(ctx, picoclaw.RunOptions{
		Message: req.Prompt,
		Agent:   agent,
		Session: session,
	})
}

// Process adapts picoclaw.RunOptions to Run so existing adapters written
// against the legacy runner signature (e.g. method research) keep working.
// EmbeddedAgents inside opt are ignored: the service owns the definition list.
func (s *Service) Process(ctx context.Context, opt picoclaw.RunOptions) (string, error) {
	return s.Run(ctx, Request{
		Agent:   opt.Agent,
		Session: opt.Session,
		Model:   opt.Model,
		Prompt:  opt.Message,
	})
}

// Resolve returns the canonical definition for an ID or alias.
func (s *Service) Resolve(id string) (picoclaw.EmbeddedAgent, bool) {
	if s == nil {
		return picoclaw.EmbeddedAgent{}, false
	}
	return picoclaw.ResolveEmbeddedAgent(s.defs, id)
}

// Definitions returns a copy of the loaded agent table.
func (s *Service) Definitions() []picoclaw.EmbeddedAgent {
	if s == nil {
		return nil
	}
	return append([]picoclaw.EmbeddedAgent(nil), s.defs...)
}

// Defaults returns the effective default agent/session/model selection. The
// returned scenario map is a copy, safe for the caller to keep.
func (s *Service) Defaults() Defaults {
	if s == nil {
		return Defaults{}
	}
	d := s.defaults
	d.Agents = make(map[string]string, len(s.defaults.Agents))
	for scenario, id := range s.defaults.Agents {
		d.Agents[scenario] = id
	}
	return d
}

// Workspace returns the working directory used for sessions and transcripts.
func (s *Service) Workspace() string {
	if s == nil {
		return ""
	}
	return s.workspace
}

// StartedAt reports when the service finished initializing.
func (s *Service) StartedAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.started
}

// Ready reports whether initialization completed successfully and the service
// has not been closed yet.
func (s *Service) Ready() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.warm != nil
}

// Close releases the readiness runner and marks the service unusable. It is
// idempotent and nil-safe.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.warm != nil {
		s.warm.Close()
		s.warm = nil
	}
	s.rt = nil
}
