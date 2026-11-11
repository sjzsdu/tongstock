package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/llm"
)

// The key is only ever read from an environment variable named by Config.
// This is a non-secret placeholder used by httptest.Server tests; no real key
// is ever hard-coded, logged, or written to an error string.
const (
	testKeyEnv = "WAN65_OPENAI_TEST_KEY"
	testKey    = "placeholder-not-a-secret"
)

func newTestProvider(t *testing.T, baseURL string, mutate func(*Config)) *Provider {
	t.Helper()
	t.Setenv(testKeyEnv, testKey)
	cfg := Config{
		BaseURL:   baseURL,
		APIKeyEnv: testKeyEnv,
		Model:     "test-model",
		Timeout:   3 * time.Second,
		MaxTokens: 64,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return p
}

func serveJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func helloRequest() llm.Request {
	return llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}}}
}

func TestName(t *testing.T) {
	p := newTestProvider(t, "https://example.com/v1", nil)
	if p.Name() != "openai-compatible" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "openai-compatible")
	}
}

func TestNewValidation(t *testing.T) {
	t.Setenv(testKeyEnv, testKey)
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"missing BaseURL", Config{APIKeyEnv: testKeyEnv, Model: "m"}, "BaseURL"},
		{"missing APIKeyEnv", Config{BaseURL: "https://example.com/v1", Model: "m"}, "APIKeyEnv"},
		{"missing Model", Config{BaseURL: "https://example.com/v1", APIKeyEnv: testKeyEnv}, "Model"},
		{"invalid URL", Config{BaseURL: "not-a-url", APIKeyEnv: testKeyEnv, Model: "m"}, "absolute URL"},
		{"missing env var", Config{BaseURL: "https://example.com/v1", APIKeyEnv: "WAN65_UNSET_ENV_XYZ", Model: "m"}, "WAN65_UNSET_ENV_XYZ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := New(c.cfg)
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
	if _, err := New(Config{BaseURL: "https://example.com/v1", APIKeyEnv: testKeyEnv, Model: "m"}); err != nil {
		t.Fatalf("New() with valid config returned %v", err)
	}
}

func TestCompleteNormalResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(w, 200, map[string]any{
			"id":    "chatcmpl-1",
			"model": "test-model",
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "Hello!"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, nil)
	got, err := p.Complete(context.Background(), helloRequest())
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got.Text != "Hello!" || got.FinishReason != llm.FinishReasonStop {
		t.Fatalf("text/finish = %q/%q", got.Text, got.FinishReason)
	}
	if got.Provider != "openai-compatible" || got.Model != "test-model" || got.RequestID != "chatcmpl-1" {
		t.Fatalf("metadata = %+v", got)
	}
	if got.Latency <= 0 {
		t.Fatalf("Latency = %v, want > 0", got.Latency)
	}
	if len(got.ToolCalls) != 0 {
		t.Fatalf("unexpected tool calls: %+v", got.ToolCalls)
	}
}

func TestCompleteUsageParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(w, 200, map[string]any{
			"id": "chatcmpl-usage",
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 4, "total_tokens": 15},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, nil)
	got, err := p.Complete(context.Background(), helloRequest())
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	want := llm.Usage{InputTokens: 11, OutputTokens: 4, TotalTokens: 15}
	if got.Usage != want {
		t.Fatalf("Usage = %+v, want %+v", got.Usage, want)
	}
}

func TestCompleteWithToolCalls(t *testing.T) {
	wantArgs := `{"symbol":"SH600519","limit":10}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(w, 200, map[string]any{
			"id": "chatcmpl-tools",
			"choices": []any{map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "",
					"tool_calls": []any{map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "screen",
							"arguments": wantArgs,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 9, "completion_tokens": 5, "total_tokens": 14},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, nil)
	got, err := p.Complete(context.Background(), helloRequest())
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got.FinishReason != llm.FinishReasonToolCalls {
		t.Fatalf("finish = %q, want tool_calls", got.FinishReason)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(got.ToolCalls))
	}
	tc := got.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "screen" {
		t.Fatalf("tool call = %+v", tc)
	}
	if string(tc.Arguments) != wantArgs || !json.Valid(tc.Arguments) {
		t.Fatalf("Arguments = %s, want valid JSON %s", tc.Arguments, wantArgs)
	}
}

func TestCompleteErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		retryAfter string
		body       string
		slow       bool
		cfgTimeout time.Duration
		want       error
		wantRetry  time.Duration
	}{
		{"401 auth", 401, "", "", false, 0, llm.ErrAuth, 0},
		{"403 auth", 403, "", "", false, 0, llm.ErrAuth, 0},
		{"429 rate limited with Retry-After", 429, "7", "", false, 0, llm.ErrRateLimited, 7 * time.Second},
		{"500 timeout", 500, "", "", false, 0, llm.ErrTimeout, 0},
		{"408 timeout", 408, "", "", false, 0, llm.ErrTimeout, 0},
		{"invalid JSON", 200, "", "this is not json", false, 0, llm.ErrInvalidResponse, 0},
		{"missing choices", 200, "", `{"id":"x"}`, false, 0, llm.ErrInvalidResponse, 0},
		{"deadline timeout", 200, "", "", true, 30 * time.Millisecond, llm.ErrTimeout, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if c.slow {
					select {
					case <-r.Context().Done():
					case <-time.After(2 * time.Second):
					}
					return
				}
				if c.retryAfter != "" {
					w.Header().Set("Retry-After", c.retryAfter)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				if c.body != "" {
					_, _ = io.WriteString(w, c.body)
				}
			}))
			defer srv.Close()

			p := newTestProvider(t, srv.URL, func(cfg *Config) {
				if c.cfgTimeout > 0 {
					cfg.Timeout = c.cfgTimeout
				}
			})
			_, err := p.Complete(context.Background(), helloRequest())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("errors.Is(err, %v) = false; err = %v", c.want, err)
			}
			if c.wantRetry > 0 {
				var pe *llm.ProviderError
				if !errors.As(err, &pe) {
					t.Fatal("error is not a *llm.ProviderError")
				}
				if pe.RetryAfter != c.wantRetry {
					t.Fatalf("RetryAfter = %v, want %v", pe.RetryAfter, c.wantRetry)
				}
			}
		})
	}
}

// A single table-driven case proves compatible endpoints need only BaseURL +
// Model changed: the request path, body shape and auth header are asserted
// against a local httptest.Server for each provider style. No real calls.
func TestCompatibleEndpoints(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		model  string
	}{
		{"DeepSeek", "/v1", "deepseek-chat"},
		{"Tongyi", "/compatible-mode/v1", "qwen-plus"},
		{"OpenRouter", "/api/v1", "openai/gpt-4o-mini"},
		{"Ollama", "/v1", "llama3.2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotPath, gotMethod, gotAuth string
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotMethod = r.Method
				gotAuth = r.Header.Get("Authorization")
				b, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(b, &gotBody)
				serveJSON(w, 200, map[string]any{
					"id":    "chatcmpl-x",
					"model": c.model,
					"choices": []any{map[string]any{
						"index":         0,
						"message":       map[string]any{"role": "assistant", "content": "ok"},
						"finish_reason": "stop",
					}},
				})
			}))
			defer srv.Close()

			p := newTestProvider(t, srv.URL+c.prefix, func(cfg *Config) {
				cfg.Model = c.model
			})
			if _, err := p.Complete(context.Background(), helloRequest()); err != nil {
				t.Fatalf("Complete() error: %v", err)
			}
			if gotMethod != http.MethodPost {
				t.Fatalf("method = %q, want POST", gotMethod)
			}
			if want := c.prefix + "/chat/completions"; gotPath != want {
				t.Fatalf("path = %q, want %q", gotPath, want)
			}
			if gotAuth != "Bearer "+testKey {
				t.Fatalf("Authorization = %q, key was not read from the environment", gotAuth)
			}
			if gotBody["model"] != c.model {
				t.Fatalf("body model = %v, want %q", gotBody["model"], c.model)
			}
			if gotBody["stream"] != false {
				t.Fatalf("body stream = %v, want false", gotBody["stream"])
			}
			if _, ok := gotBody["messages"]; !ok {
				t.Fatal("body is missing messages")
			}
		})
	}
}

// llm.Request.Timeout must override Config.Timeout: a long config timeout and a
// short per-request timeout must produce llm.ErrTimeout against a slow server,
// for both Complete and Stream. The server still answers (after a delay shorter
// than cfg.Timeout) so ignoring req.Timeout would make the call succeed instead
// of timing out — the assertion therefore proves the override, not the fallback.
func TestRequestTimeoutOverridesConfig(t *testing.T) {
	const serverDelay = 400 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A client-side timeout cancels the request, so exit without writing.
		// Otherwise reply with a valid completion after the delay.
		select {
		case <-time.After(serverDelay):
		case <-r.Context().Done():
			return
		}
		serveJSON(w, 200, map[string]any{
			"id": "chatcmpl-slow",
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "late"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	req := helloRequest()
	req.Timeout = 40 * time.Millisecond // short; must win over cfg.Timeout
	p := newTestProvider(t, srv.URL, func(cfg *Config) {
		cfg.Timeout = 2 * time.Second // long
	})

	if _, err := p.Complete(context.Background(), req); !errors.Is(err, llm.ErrTimeout) {
		t.Fatalf("Complete with req.Timeout override: err = %v, want llm.ErrTimeout", err)
	}
	s, err := p.Stream(context.Background(), req)
	if err == nil {
		_ = s.Close()
		t.Fatal("Stream with req.Timeout override: expected llm.ErrTimeout, got a stream")
	}
	if !errors.Is(err, llm.ErrTimeout) {
		t.Fatalf("Stream with req.Timeout override: err = %v, want llm.ErrTimeout", err)
	}
}
