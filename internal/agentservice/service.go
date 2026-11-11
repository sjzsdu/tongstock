// Package agentservice is the single entrypoint for the agent capability.
// It turns config.AgentConfig into a runnable service: an llm.Provider, the
// embedded plus user-defined agent definition table, and the default
// agent/session/model selection all live behind one constructor and one Run
// method, so the HTTP server, the CLI, and any future subsystem share the same
// configuration resolution and execution semantics.
//
// The service depends only on TongStock-owned types (internal/agents,
// internal/llm) and the provider-agnostic llm.Provider interface. Credentials
// are read by the concrete adapter from the environment named by
// Config.APIKeyEnv; the service never sees, logs, or stores an API key.
package agentservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/internal/agents"
	"github.com/sjzsdu/tongstock/internal/llm"
	"github.com/sjzsdu/tongstock/internal/llm/openai"
	"github.com/sjzsdu/tongstock/pkg/config"
)

// ErrDisabled reports that the agent capability is switched off in
// config.yaml (agent.enabled).
var ErrDisabled = errors.New("AI agent 未启用：请在配置中设置 agent.enabled=true")

// ErrRunner wraps provider-construction/setup failures so callers can still
// tell construction errors apart from execution errors. It is a business
// sentinel: provider execution failures are surfaced as llm.ProviderError.
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
	Enabled   bool
	Provider  string
	BaseURL   string
	APIKeyEnv string
	Model     string
	Session   string
	// Timeout is the per-request provider timeout. Zero keeps the adapter
	// default.
	Timeout time.Duration
	// MaxTokens caps completion length on every request. Zero keeps the
	// adapter default.
	MaxTokens int
	// Streaming records the configured response-mode preference. The HTTP
	// layer still decides per request based on the endpoint the client calls.
	Streaming bool
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
	Lister func() ([]agents.Definition, error)
	// Provider injects a ready llm.Provider (for example llm.FakeProvider in
	// tests, or a provider built by the composition root). When nil, New builds
	// an OpenAI-compatible provider from Config. Either way the Service only
	// ever holds an llm.Provider, never a concrete runtime.
	Provider llm.Provider
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
	// OnDelta, when set, receives streamed response chunks. It lives only at
	// this business layer: it is translated from llm.Event{EventTypeTextDelta}
	// by the service and is never pushed down into a provider.
	OnDelta func(string)
}

// RunOptions is the TongStock-owned options shape consumed by Process. It
// replaces the legacy runtime options so this package no longer references
// the vendor SDK. Process only consumes Message/Agent/Session/Model/OnDelta;
// the remaining fields preserve the legacy shape for adapters and are ignored.
type RunOptions struct {
	Message        string
	Session        string
	Agent          string
	Model          string
	Workspace      string
	Debug          bool
	Quiet          bool
	EmbeddedAgents []agents.Definition
	OnDelta        func(string)
}

// Service owns the provider, the agent definitions, and the default selection.
// It is safe for concurrent use: the provider is immutable and concurrency-safe,
// so parallel requests never serialize on model execution.
type Service struct {
	mu        sync.Mutex
	cfg       Config
	provider  llm.Provider
	defs      []agents.Definition
	defaults  Defaults
	workspace string
	started   time.Time
}

// FromAgentConfig converts the YAML agent section into a resolved runtime
// config: timeout parsing, "~/" expansion, and the fold of the legacy flat
// agent/stock_agent fields into the scenario map happen here, once. Explicit
// defaults.* always win over the legacy fields. A malformed agent.timeout is
// already rejected by config.Load; here it degrades to the adapter default so
// programmatic callers keep a non-failing constructor.
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
	timeout, _ := cfg.ParseTimeout()
	return Config{
		Enabled:       cfg.Enabled,
		Provider:      cfg.Provider,
		BaseURL:       cfg.BaseURL,
		APIKeyEnv:     cfg.APIKeyEnv,
		Model:         cfg.Model,
		Session:       cfg.Session,
		Timeout:       timeout,
		MaxTokens:     cfg.MaxTokens,
		Streaming:     cfg.Streaming,
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

// New builds the service from a resolved Config. It fails fast: provider
// construction and definition load are validated here so callers degrade at
// startup instead of on the first request. Empty Agent/Session/Model defaults
// are resolved here (Defaults + Config) and re-applied on every Run.
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

	provider := opts.Provider
	if provider == nil {
		if strings.TrimSpace(cfg.Model) == "" {
			return nil, fmt.Errorf("agent.model is required when no llm.Provider is injected; configure agent.model or inject one via Options.Provider")
		}
		p, err := openai.New(openai.Config{
			BaseURL:   cfg.BaseURL,
			APIKeyEnv: cfg.APIKeyEnv,
			Model:     cfg.Model,
			Timeout:   cfg.Timeout,
			MaxTokens: cfg.MaxTokens,
			Streaming: cfg.Streaming,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRunner, err)
		}
		provider = p
	}

	defs, err := loadDefinitions(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("load embedded agents failed: %w", err)
	}

	return &Service{
		cfg:       cfg,
		provider:  provider,
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

func loadDefinitions(cfg Config, opts Options) ([]agents.Definition, error) {
	if opts.Lister != nil {
		return opts.Lister()
	}
	return agents.ListWithPaths(cfg.AgentPaths)
}

// Run executes one request. Blank Agent/Session/Model fields fall back to the
// service defaults; when OnDelta is set the service pulls llm.Stream events and
// translates text deltas into the business callback, otherwise it uses a single
// Complete call. The caller's context is passed straight to the provider so a
// client disconnect cancels the upstream request.
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
	provider := s.provider
	defs := s.defs
	s.mu.Unlock()
	if provider == nil {
		return "", errors.New("agent service not initialized")
	}

	def, _ := agents.Resolve(defs, agent)
	llmReq := buildRequest(def, model, session, agent, req.Prompt)
	llmReq.MaxTokens = s.cfg.MaxTokens

	start := time.Now()
	var (
		text string
		resp llm.Response
		err  error
	)
	if req.OnDelta != nil {
		text, resp, err = s.runStream(ctx, provider, llmReq, req.OnDelta)
	} else {
		resp, err = provider.Complete(ctx, llmReq)
		text = resp.Text
	}

	// Audit: provider/model/request/session IDs, latency, token usage and a
	// short error summary. Never log the API key or the (possibly sensitive)
	// prompt/response content.
	s.audit(provider, llmReq, resp, time.Since(start), err)
	if err != nil {
		return "", mapRunError(err)
	}
	return normalizeResponse(text)
}

// buildRequest assembles an llm.Request from a resolved definition. Soul and
// the Markdown body prompt become a system message; tools are restricted to
// the read-only allowlist. Session and the resolved agent identity are carried
// in SessionID/Metadata so chat and research scenarios stay distinguishable.
func buildRequest(def agents.Definition, model, session, agent, prompt string) llm.Request {
	var sys strings.Builder
	if def.Soul != "" {
		sys.WriteString(def.Soul)
	}
	if def.Prompt != "" {
		if sys.Len() > 0 {
			sys.WriteString("\n\n")
		}
		sys.WriteString(def.Prompt)
	}
	messages := make([]llm.Message, 0, 2)
	if sys.Len() > 0 {
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: sys.String()})
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: prompt})
	return llm.Request{
		Messages:  messages,
		Model:     model,
		Tools:     toolDefinitions(def.Tools),
		SessionID: session,
		Metadata:  map[string]string{"agent": agent},
	}
}

// runStream pulls events and forwards text deltas to OnDelta in order. The
// llm abstraction has no callback streaming: this translation is deliberately
// kept at the service layer. OnDelta never reaches the provider.
func (s *Service) runStream(ctx context.Context, provider llm.Provider, req llm.Request, onDelta func(string)) (string, llm.Response, error) {
	stream, err := provider.Stream(ctx, req)
	if err != nil {
		return "", llm.Response{}, err
	}
	defer stream.Close()

	var (
		sb   strings.Builder
		resp llm.Response
	)
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return sb.String(), resp, err
		}
		switch ev.Type {
		case llm.EventTypeTextDelta:
			sb.WriteString(ev.Text)
			if ev.Text != "" {
				onDelta(ev.Text)
			}
		case llm.EventTypeDone:
			if ev.Usage != nil {
				resp.Usage = *ev.Usage
			}
		case llm.EventTypeError:
			if ev.Err != nil {
				return sb.String(), resp, ev.Err
			}
			return sb.String(), resp, &llm.ProviderError{Code: llm.CodeUnknown, Message: "stream error"}
		}
	}
	resp.Text = sb.String()
	return sb.String(), resp, nil
}

// normalizeResponse is the pure string post-processing migrated from the old
// runtime: blank output or the provider's empty-response sentinel is turned
// into a stable error instead of being returned as a successful empty answer.
func normalizeResponse(text string) (string, error) {
	const emptyResponseSentinel = "The model returned an empty response. This may indicate a provider error or token limit."
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", errors.New("model returned an empty response")
	}
	if trimmed == emptyResponseSentinel {
		return "", errors.New("model returned an empty response (provider may have returned no final content)")
	}
	return text, nil
}

// mapRunError keeps the error surface stable. Client-side cancellation is
// returned verbatim (a disconnected client is not a provider failure);
// llm.ProviderError already implements errors.Is against the stable llm
// sentinels (ErrAuth/ErrRateLimited/ErrTimeout/ErrInvalidResponse/
// ErrToolCallFailed) and is passed through so those match unchanged.
func mapRunError(err error) error {
	return err
}

func (s *Service) audit(provider llm.Provider, req llm.Request, resp llm.Response, elapsed time.Duration, err error) {
	model := resp.Model
	if model == "" {
		model = req.Model
	}
	log.Printf("agentservice: provider=%s model=%s request_id=%s session=%s latency=%s tokens(in=%d out=%d total=%d) status=%s",
		provider.Name(), model, resp.RequestID, req.SessionID, elapsed,
		resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens, errShort(err))
}

// errShort returns a short, non-sensitive error summary for the audit log. It
// never includes a full prompt, response body, or an API key.
func errShort(err error) string {
	if err == nil {
		return "ok"
	}
	var pe *llm.ProviderError
	if errors.As(err, &pe) {
		return string(pe.Code) + ": " + pe.Message
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "error"
	}
}

// readOnlyToolDefs is the ONLY set of tools the service may advertise to a
// provider. It mirrors the read-only (PermRead) tools in internal/ai_tools:
// data_snapshot, feature_query, experiment_report, paradigm_version. The old
// runtime had an applyEmbeddedAgentTools switch that could enable write tools
// (exec/write_file/edit_file/append_file/spawn/subagent) from a `tools:` list;
// that switch is deliberately NOT reproduced here. A `tools:` declaration can
// only ever select a subset of this read-only set, and tool execution stays on
// internal/ai_tools' ReadOnlyGuard / audit boundary.
var readOnlyToolDefs = map[string]llm.ToolDefinition{
	"data_snapshot": {
		Name:        "data_snapshot",
		Description: "Query TongStock internal data snapshots (read-only).",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	},
	"feature_query": {
		Name:        "feature_query",
		Description: "Query TongStock feature definitions (read-only).",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	},
	"experiment_report": {
		Name:        "experiment_report",
		Description: "Query TongStock experiment and backtest reports (read-only).",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	},
	"paradigm_version": {
		Name:        "paradigm_version",
		Description: "Query TongStock paradigm versions and evidence (read-only).",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	},
}

// toolDefinitions intersects an agent's `tools:` declarations with the
// read-only allowlist. Unknown names (including every write tool) are silently
// dropped, so the registration result is always read-only.
func toolDefinitions(declared []string) []llm.ToolDefinition {
	seen := make(map[string]bool, len(declared))
	out := make([]llm.ToolDefinition, 0, len(declared))
	for _, name := range declared {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		if def, ok := readOnlyToolDefs[key]; ok {
			seen[key] = true
			out = append(out, def)
		}
	}
	return out
}

// Process adapts the legacy RunOptions shape to Run so existing adapters
// written against the old runner signature keep working. Definition lists and
// other fields inside opt are ignored: the service owns the definition table.
func (s *Service) Process(ctx context.Context, opt RunOptions) (string, error) {
	return s.Run(ctx, Request{
		Agent:   opt.Agent,
		Session: opt.Session,
		Model:   opt.Model,
		Prompt:  opt.Message,
		OnDelta: opt.OnDelta,
	})
}

// Resolve returns the canonical definition for an ID or alias.
func (s *Service) Resolve(id string) (agents.Definition, bool) {
	if s == nil {
		return agents.Definition{}, false
	}
	return agents.Resolve(s.defs, id)
}

// Definitions returns a copy of the loaded agent table.
func (s *Service) Definitions() []agents.Definition {
	if s == nil {
		return nil
	}
	return append([]agents.Definition(nil), s.defs...)
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
	return s.provider != nil
}

// Close marks the service unusable. It is idempotent and nil-safe. The
// provider holds only a reusable http.Client, so there is nothing to tear down.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider = nil
}
