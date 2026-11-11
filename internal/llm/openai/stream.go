package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/sjzsdu/tongstock/internal/llm"
)

// streamChunk is one JSON object carried by an SSE `data:` line.
type streamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string              `json:"role"`
			Content   string              `json:"content"`
			ToolCalls []wireToolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type wireToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// toolAcc accumulates a single tool call's arguments across SSE frames by
// index. OpenAI sends function.arguments as arbitrary string fragments; only
// once the accumulated bytes form complete, valid JSON is the tool call handed
// to the upper layer.
type toolAcc struct {
	id      string
	name    string
	args    strings.Builder
	emitted bool
}

// stream is a pull-based llm.Stream. The context passed to Stream is bound to
// the upstream http.Request via a derived context, so cancelling it interrupts
// the connection. A timeout is applied only when the caller explicitly sets
// llm.Request.Timeout; otherwise the caller's ctx alone governs the lifetime.
// Close is idempotent and never panics.
type stream struct {
	ctx      context.Context // parent ctx, used for cancellation errors
	reqCtx   context.Context // derived request ctx; carries req.Timeout when set
	cancel   context.CancelFunc
	provider *Provider
	body     io.ReadCloser
	scanner  *bufio.Scanner

	mu      sync.Mutex
	pending []llm.Event
	done    bool // terminal (done/error) event has been emitted
	closed  bool

	// Owned by the single Recv goroutine (a pull stream is consumed
	// sequentially); never touched by Close.
	acc    map[int]*toolAcc
	finish llm.FinishReason
	usage  *llm.Usage
}

// Stream implements llm.Provider. It opens a streaming chat/completions request
// and returns a pull-based event stream bound to ctx. Config.Timeout is NOT
// applied to the total stream duration: a healthy stream that outlasts the
// configured timeout must not be cut (research / long-form output). Only an
// explicit llm.Request.Timeout opts into a total-time deadline on the request
// (covering the connection/first-byte phase); otherwise the caller's ctx alone
// governs. No per-frame idle timer is used.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, p.providerError(llm.CodeInvalidResponse, "cannot encode request")
	}

	var requestCtx context.Context
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		// Explicit opt-in only. Config.Timeout is intentionally ignored here:
		// it is a single-request timeout, not a cap on a whole SSE stream.
		requestCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	} else {
		requestCtx, cancel = context.WithCancel(ctx)
	}
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, p.providerError(llm.CodeInvalidResponse, "cannot build request")
	}
	p.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		cancel()
		return nil, p.mapTransportError(ctx, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		cancel()
		return nil, p.httpStatusError(resp.StatusCode, resp.Header)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 4096), 1<<20)
	return &stream{
		ctx:      ctx,
		reqCtx:   requestCtx,
		cancel:   cancel,
		provider: p,
		body:     resp.Body,
		scanner:  sc,
		acc:      make(map[int]*toolAcc),
		finish:   llm.FinishReasonStop,
	}, nil
}

// ctxError returns the error a Recv call should surface when a context ends.
// A caller-driven cancellation is returned as-is (context.Canceled); a deadline
// — from the caller's ctx or the explicit opt-in llm.Request.Timeout — is
// mapped onto llm.ErrTimeout via a short llm.ProviderError. Config.Timeout is
// never a stream deadline.
func (s *stream) ctxError() error {
	if err := s.ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return s.provider.providerError(llm.CodeTimeout, "request deadline exceeded")
		}
		return err
	}
	if s.reqCtx != nil && errors.Is(s.reqCtx.Err(), context.DeadlineExceeded) {
		return s.provider.providerError(llm.CodeTimeout, "request timed out")
	}
	return nil
}

// Recv implements llm.Stream. Cancellation takes priority over buffered events
// and io.EOF. Once the stream is exhausted it returns io.EOF.
func (s *stream) Recv() (llm.Event, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return llm.Event{}, io.EOF
	}
	if err := s.ctxError(); err != nil {
		s.mu.Unlock()
		return llm.Event{}, err
	}
	if len(s.pending) > 0 {
		ev := s.pending[0]
		s.pending = s.pending[1:]
		s.mu.Unlock()
		return ev, nil
	}
	if s.done {
		s.mu.Unlock()
		return llm.Event{}, io.EOF
	}
	s.mu.Unlock()

	// Slow path: consume SSE lines until at least one event is produced or the
	// stream terminates. No lock is held across the blocking scanner read, so
	// Close can always unblock it by closing the body.
	for {
		if err := s.ctxError(); err != nil {
			return llm.Event{}, err
		}
		if !s.scanner.Scan() {
			return s.endOfStream()
		}
		payload, ok := ssePayload(s.scanner.Text())
		if !ok {
			continue
		}

		var events []llm.Event
		terminal := false
		if payload == "[DONE]" {
			events = s.finalize()
			terminal = true
		} else {
			var parseErr error
			events, parseErr = s.parseChunk(payload)
			if parseErr != nil {
				s.mu.Lock()
				s.done = true
				s.mu.Unlock()
				return llm.Event{}, parseErr
			}
			for _, ev := range events {
				if ev.Type == llm.EventTypeError {
					terminal = true
				}
			}
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return llm.Event{}, io.EOF
		}
		if err := s.ctxError(); err != nil {
			s.mu.Unlock()
			return llm.Event{}, err
		}
		if terminal {
			s.done = true
			s.cancel()
		}
		if len(events) == 0 {
			s.mu.Unlock()
			continue
		}
		s.pending = append(s.pending, events...)
		first := s.pending[0]
		s.pending = s.pending[1:]
		s.mu.Unlock()
		return first, nil
	}
}

// endOfStream handles the scanner finishing without an explicit [DONE]. It
// still surfaces a final done event (and any buffered tool calls) so callers
// always observe a terminal; a context deadline maps to ErrTimeout, any other
// read error maps to ErrInvalidResponse.
func (s *stream) endOfStream() (llm.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llm.Event{}, io.EOF
	}
	if err := s.ctxError(); err != nil {
		return llm.Event{}, err
	}
	if s.done {
		return llm.Event{}, io.EOF
	}
	s.done = true
	s.cancel()
	if err := s.scanner.Err(); err != nil {
		return llm.Event{}, s.provider.providerError(llm.CodeInvalidResponse, "SSE stream ended with a read error")
	}
	events := s.finalize()
	if len(events) == 0 {
		return llm.Event{}, io.EOF
	}
	s.pending = append(s.pending, events...)
	first := s.pending[0]
	s.pending = s.pending[1:]
	return first, nil
}

// parseChunk turns one SSE data payload into zero or more llm.Event values.
// Text deltas are emitted per frame; tool call arguments are accumulated by
// index and only emitted once they form complete, valid JSON.
func (s *stream) parseChunk(payload string) ([]llm.Event, error) {
	var c streamChunk
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		return nil, s.provider.providerError(llm.CodeInvalidResponse, "invalid SSE chunk JSON")
	}
	if c.Error != nil {
		msg := c.Error.Type
		if msg == "" {
			msg = "stream error"
		}
		return []llm.Event{{
			Type: llm.EventTypeError,
			Err:  s.provider.providerError(llm.CodeUnknown, truncate(msg, 120)),
		}}, nil
	}
	if c.Usage != nil {
		s.usage = &llm.Usage{
			InputTokens:  c.Usage.PromptTokens,
			OutputTokens: c.Usage.CompletionTokens,
			TotalTokens:  c.Usage.TotalTokens,
		}
	}

	var events []llm.Event
	for _, ch := range c.Choices {
		if ch.FinishReason != "" {
			s.finish = llm.FinishReason(ch.FinishReason)
		}
		if ch.Delta.Content != "" {
			events = append(events, llm.Event{Type: llm.EventTypeTextDelta, Text: ch.Delta.Content})
		}
		for _, tc := range ch.Delta.ToolCalls {
			acc := s.acc[tc.Index]
			if acc == nil {
				acc = &toolAcc{}
				s.acc[tc.Index] = acc
			}
			if acc.id == "" && tc.ID != "" {
				acc.id = tc.ID
			}
			if acc.name == "" && tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.args.WriteString(tc.Function.Arguments)
			if !acc.emitted && acc.args.Len() > 0 && json.Valid([]byte(acc.args.String())) {
				events = append(events, llm.Event{
					Type: llm.EventTypeToolCallDelta,
					ToolCall: &llm.ToolCall{
						ID:        acc.id,
						Name:      acc.name,
						Arguments: json.RawMessage(acc.args.String()),
					},
				})
				acc.emitted = true
			}
		}
	}
	return events, nil
}

// finalize emits any tool call whose arguments never reached a valid-JSON
// state, followed by the terminal done event. Indices are sorted for a stable
// order. Incomplete arguments surface an error event (ErrInvalidResponse).
func (s *stream) finalize() []llm.Event {
	indices := make([]int, 0, len(s.acc))
	for i := range s.acc {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	var events []llm.Event
	for _, i := range indices {
		acc := s.acc[i]
		if acc.emitted {
			continue
		}
		raw := acc.args.String()
		if raw == "" {
			raw = "{}"
		}
		if !json.Valid([]byte(raw)) {
			events = append(events, llm.Event{
				Type: llm.EventTypeError,
				Err:  s.provider.providerError(llm.CodeInvalidResponse, "incomplete tool call arguments"),
			})
			continue
		}
		events = append(events, llm.Event{
			Type: llm.EventTypeToolCallDelta,
			ToolCall: &llm.ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: json.RawMessage(raw),
			},
		})
		acc.emitted = true
	}
	events = append(events, llm.Event{
		Type:         llm.EventTypeDone,
		FinishReason: s.finish,
		Usage:        s.usage,
	})
	return events
}

// ssePayload extracts the payload of a `data:` line. Blank lines, comments and
// other SSE fields are ignored. Both "data: x" and "data:x" are accepted.
func ssePayload(line string) (string, bool) {
	line = strings.TrimSuffix(line, "\r")
	if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}

// Close implements llm.Stream. It is idempotent and never panics: repeated
// calls (or a call racing with Recv) are safe. It also aborts the upstream
// request so an in-flight read is always released.
func (s *stream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.cancel()
	if s.body != nil {
		_ = s.body.Close()
	}
	return nil
}

var _ llm.Provider = (*Provider)(nil)
var _ llm.Stream = (*stream)(nil)
