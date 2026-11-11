// Package openai implements the first concrete llm.Provider: a narrow,
// OpenAI-compatible HTTP adapter built only on the standard library. It does
// not import any vendor SDK and does not add module dependencies.
//
// Credentials are never part of a Request or a Response. Config.APIKeyEnv names
// the environment variable that holds the API key; New reads it once at startup.
// The key is only ever sent in the Authorization header and is never written to
// logs, error strings, ProviderError.Message, or tests.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/llm"
)

const (
	providerName     = "openai-compatible"
	defaultTimeout   = 60 * time.Second
	maxResponseBytes = 8 << 20 // 8 MiB guard on a non-streaming response body
)

// Config configures a Provider. APIKeyEnv is the NAME of an environment
// variable (for example "DEEPSEEK_API_KEY"), never the key value itself.
// BaseURL, APIKeyEnv and Model are required; Timeout/MaxTokens/Streaming have
// usable defaults.
type Config struct {
	BaseURL   string
	APIKeyEnv string
	Model     string
	Timeout   time.Duration
	MaxTokens int
	Streaming bool
}

// Provider is an OpenAI-compatible llm.Provider. It is safe for concurrent use:
// it holds only immutable configuration and a reusable http.Client.
type Provider struct {
	cfg     Config
	baseURL string // cfg.BaseURL with trailing slashes removed
	apiKey  string // resolved from the environment named by cfg.APIKeyEnv
	client  *http.Client
}

// New validates cfg, resolves the API key from the environment named by
// cfg.APIKeyEnv, and returns a ready Provider. Missing required fields or a
// missing/empty environment variable produce actionable errors naming exactly
// what is missing; the error never contains a key value.
func New(cfg Config) (*Provider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("openai: Config.BaseURL is required (e.g. https://api.deepseek.com/v1)")
	}
	if strings.TrimSpace(cfg.APIKeyEnv) == "" {
		return nil, fmt.Errorf("openai: Config.APIKeyEnv is required: it must name the environment variable holding the API key")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("openai: Config.Model is required (e.g. deepseek-chat)")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("openai: Config.BaseURL %q is not a valid absolute URL", cfg.BaseURL)
	}

	key, ok := os.LookupEnv(cfg.APIKeyEnv)
	if !ok || strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("openai: environment variable %q (named by Config.APIKeyEnv) is not set or empty; export it with the provider API key", cfg.APIKeyEnv)
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Provider{
		cfg:     cfg,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  key,
		client:  &http.Client{},
	}, nil
}

// Name implements llm.Provider.
func (p *Provider) Name() string { return providerName }

// --- wire types for the chat/completions request ---------------------------------

type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
	Tools       []wireToolDef `json:"tools,omitempty"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireToolDef struct {
	Type     string          `json:"type"`
	Function wireToolDefFunc `json:"function"`
}

type wireToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// buildRequestBody translates the provider-agnostic llm.Request into the
// OpenAI chat/completions JSON shape. The same builder serves Complete and
// Stream; only the Stream flag differs.
func (p *Provider) buildRequestBody(req llm.Request, stream bool) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	wr := wireRequest{
		Model:       model,
		Messages:    make([]wireMessage, 0, len(req.Messages)),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      stream,
	}
	if wr.MaxTokens == 0 && p.cfg.MaxTokens > 0 {
		wr.MaxTokens = p.cfg.MaxTokens
	}
	for _, m := range req.Messages {
		wm := wireMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: wireFunction{
					Name:      tc.Name,
					Arguments: string(tc.Arguments),
				},
			})
		}
		wr.Messages = append(wr.Messages, wm)
	}
	for _, td := range req.Tools {
		wr.Tools = append(wr.Tools, wireToolDef{
			Type: "function",
			Function: wireToolDefFunc{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Parameters,
			},
		})
	}
	return json.Marshal(wr)
}

// --- wire types for the chat/completions response --------------------------------

type wireRespToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type wireRespMessage struct {
	Role      string             `json:"role"`
	Content   string             `json:"content"`
	ToolCalls []wireRespToolCall `json:"tool_calls"`
}

type wireChoice struct {
	Index        int             `json:"index"`
	Message      wireRespMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []wireChoice `json:"choices"`
	Usage   wireUsage    `json:"usage"`
}

// effectiveTimeout returns the per-request timeout declared on llm.Request when
// the caller set one, otherwise the provider-level Config.Timeout default. Both
// Complete and Stream use this so the interface's Timeout field is honored.
func (p *Provider) effectiveTimeout(req llm.Request) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}
	return p.cfg.Timeout
}

// Complete implements llm.Provider. It performs a single non-streaming POST to
// {BaseURL}/chat/completions and maps backend failures onto llm's stable
// sentinels via llm.ProviderError.
func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	body, err := p.buildRequestBody(req, false)
	if err != nil {
		return llm.Response{}, p.providerError(llm.CodeInvalidResponse, "cannot encode request")
	}

	callCtx := ctx
	var cancel context.CancelFunc = func() {}
	if d := p.effectiveTimeout(req); d > 0 {
		callCtx, cancel = context.WithTimeout(ctx, d)
	}
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, p.providerError(llm.CodeInvalidResponse, "cannot build request")
	}
	p.setHeaders(httpReq)

	start := time.Now()
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, p.mapTransportError(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llm.Response{}, p.httpStatusError(resp.StatusCode, resp.Header)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return llm.Response{}, p.providerError(llm.CodeInvalidResponse, "cannot read response body")
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return llm.Response{}, p.providerError(llm.CodeInvalidResponse, "response is not valid JSON")
	}
	if len(cr.Choices) == 0 {
		return llm.Response{}, p.providerError(llm.CodeInvalidResponse, "response has no choices")
	}

	ch := cr.Choices[0]
	toolCalls := make([]llm.ToolCall, 0, len(ch.Message.ToolCalls))
	for _, tc := range ch.Message.ToolCalls {
		args := normalizeArgs(tc.Function.Arguments)
		if !json.Valid(args) {
			return llm.Response{}, p.providerError(llm.CodeToolCallFailed, "tool call arguments are not valid JSON")
		}
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	model := cr.Model
	if model == "" {
		model = req.Model
	}
	if model == "" {
		model = p.cfg.Model
	}

	return llm.Response{
		Text:         ch.Message.Content,
		FinishReason: llm.FinishReason(ch.FinishReason),
		Usage: llm.Usage{
			InputTokens:  cr.Usage.PromptTokens,
			OutputTokens: cr.Usage.CompletionTokens,
			TotalTokens:  cr.Usage.TotalTokens,
		},
		ToolCalls: toolCalls,
		Provider:  p.Name(),
		Model:     model,
		RequestID: cr.ID,
		Latency:   time.Since(start),
	}, nil
}

// setHeaders applies the headers common to every request. The API key is used
// here and nowhere else: it is never logged or embedded in an error.
func (p *Provider) setHeaders(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+p.apiKey)
	r.Header.Set("Accept", "application/json")
}

// normalizeArgs accepts both OpenAI-style arguments (a JSON string that itself
// contains JSON) and compatible endpoints that return an object directly, and
// always returns raw JSON bytes suitable for llm.ToolCall.Arguments.
func normalizeArgs(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if s == "" {
				return json.RawMessage("{}")
			}
			return json.RawMessage(s)
		}
	}
	return raw
}

// --- error mapping -------------------------------------------------------------

// providerError builds a short, non-sensitive llm.ProviderError carrying a
// stable ErrorCode so callers can branch with errors.Is.
func (p *Provider) providerError(code llm.ErrorCode, msg string) *llm.ProviderError {
	return &llm.ProviderError{Provider: p.Name(), Code: code, Message: msg}
}

func (p *Provider) httpStatusError(status int, h http.Header) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return p.providerError(llm.CodeAuth, "authentication failed")
	case status == http.StatusTooManyRequests:
		pe := p.providerError(llm.CodeRateLimited, "rate limited")
		if d := parseRetryAfter(h.Get("Retry-After")); d > 0 {
			pe.RetryAfter = d
		}
		return pe
	case status == http.StatusRequestTimeout || status >= 500:
		return p.providerError(llm.CodeTimeout, "upstream timeout or unavailable")
	default:
		return p.providerError(llm.CodeUnknown, fmt.Sprintf("unexpected HTTP status %d", status))
	}
}

// mapTransportError classifies errors returned by http.Client.Do. Context
// deadlines, client timeouts and net.Error timeouts all map to ErrTimeout; a
// cancelled context is returned as-is so callers still see context.Canceled.
func (p *Provider) mapTransportError(ctx context.Context, err error) error {
	if cerr := ctx.Err(); cerr != nil {
		if errors.Is(cerr, context.DeadlineExceeded) {
			return p.providerError(llm.CodeTimeout, "request deadline exceeded")
		}
		return cerr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return p.providerError(llm.CodeTimeout, "request timed out")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return p.providerError(llm.CodeTimeout, "request timed out")
	}
	return p.providerError(llm.CodeUnknown, "request could not be completed")
}

// parseRetryAfter understands both Retry-After forms: integer seconds and an
// HTTP date. Unparseable or non-positive values yield 0.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// truncate keeps error messages short so a ProviderError.Message can never carry
// a full prompt or response body.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
