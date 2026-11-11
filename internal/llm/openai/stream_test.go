package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/llm"
)

// writeSSE writes one SSE `data:` line (a JSON object) and flushes.
func writeSSE(w http.ResponseWriter, fl http.Flusher, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "data: %s\n\n", b)
	fl.Flush()
}

func sseServer(t *testing.T, frames []any, done bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		for _, f := range frames {
			writeSSE(w, fl, f)
		}
		if done {
			fmt.Fprint(w, "data: [DONE]\n\n")
			fl.Flush()
		}
	}))
}

// Cross-frame tool call arguments must be accumulated by index into one
// complete, valid JSON value before being handed to the upper layer.
func TestStreamSSEFramesAndToolCallAccumulation(t *testing.T) {
	frames := []any{
		map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"role": "assistant", "content": "Hel"},
		}}},
		map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": "lo"},
		}}},
		// First fragment of tool call #0: id/name + partial arguments.
		map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call_1", "type": "function",
				"function": map[string]any{"name": "quote", "arguments": `{"sym`},
			}}},
		}}},
		// Second fragment completes the JSON object.
		map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{
				"index":    0,
				"function": map[string]any{"arguments": `bol":"SH600519"}`},
			}}},
		}}},
		// finish_reason + usage, empty delta.
		map[string]any{
			"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
		},
	}
	srv := sseServer(t, frames, true)
	defer srv.Close()

	p := newTestProvider(t, srv.URL, nil)
	s, err := p.Stream(context.Background(), helloRequest())
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	defer s.Close()

	var events []llm.Event
	for {
		ev, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv() error: %v", err)
		}
		events = append(events, ev)
	}
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	if events[0].Type != llm.EventTypeTextDelta || events[0].Text != "Hel" {
		t.Fatalf("event[0] = %+v, want text_delta 'Hel'", events[0])
	}
	if events[1].Type != llm.EventTypeTextDelta || events[1].Text != "lo" {
		t.Fatalf("event[1] = %+v, want text_delta 'lo'", events[1])
	}
	if events[2].Type != llm.EventTypeToolCallDelta || events[2].ToolCall == nil {
		t.Fatalf("event[2] = %+v, want tool_call_delta", events[2])
	}
	tc := events[2].ToolCall
	if tc.ID != "call_1" || tc.Name != "quote" {
		t.Fatalf("tool call id/name = %q/%q", tc.ID, tc.Name)
	}
	wantArgs := `{"symbol":"SH600519"}`
	if string(tc.Arguments) != wantArgs || !json.Valid(tc.Arguments) {
		t.Fatalf("Arguments = %s, want complete valid JSON %s", tc.Arguments, wantArgs)
	}
	if events[3].Type != llm.EventTypeDone || events[3].FinishReason != llm.FinishReasonToolCalls {
		t.Fatalf("event[3] = %+v, want done/tool_calls", events[3])
	}
	if events[3].Usage == nil || events[3].Usage.TotalTokens != 7 {
		t.Fatalf("done usage = %+v, want TotalTokens 7", events[3].Usage)
	}
	if _, err := s.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv after done = %v, want io.EOF", err)
	}
	// Close is idempotent.
	for i := 0; i < 2; i++ {
		if err := s.Close(); err != nil {
			t.Fatalf("Close(%d) = %v", i, err)
		}
	}
}

// A cancelled context must interrupt the upstream connection, make Recv return
// the context error, and leave Close idempotent.
func TestStreamCancellation(t *testing.T) {
	upstreamDone := make(chan struct{})
	firstFlushed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		writeSSE(w, fl, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": "hi"},
		}}})
		close(firstFlushed)
		select {
		case <-r.Context().Done():
			close(upstreamDone)
		case <-time.After(3 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := newTestProvider(t, srv.URL, nil)
	s, err := p.Stream(ctx, helloRequest())
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	ev, err := s.Recv()
	if err != nil || ev.Text != "hi" {
		t.Fatalf("first Recv = %+v, %v", ev, err)
	}
	<-firstFlushed
	cancel()

	select {
	case <-upstreamDone:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream connection was not interrupted on cancel")
	}
	if _, err := s.Recv(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Recv after cancel = %v, want context.Canceled", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Close(); err != nil {
			t.Fatalf("Close(%d) = %v", i, err)
		}
	}
}

func TestStreamCloseIsIdempotent(t *testing.T) {
	frames := []any{map[string]any{"choices": []any{map[string]any{
		"index": 0, "delta": map[string]any{"content": "x"},
	}}}}
	srv := sseServer(t, frames, true)
	defer srv.Close()

	p := newTestProvider(t, srv.URL, nil)
	s, err := p.Stream(context.Background(), helloRequest())
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.Close(); err != nil {
			t.Fatalf("Close(%d) = %v", i, err)
		}
	}
	if _, err := s.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv after Close = %v, want io.EOF", err)
	}
}

func TestStreamInvalidSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: not-json\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, nil)
	s, err := p.Stream(context.Background(), helloRequest())
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	defer s.Close()
	if _, err := s.Recv(); !errors.Is(err, llm.ErrInvalidResponse) {
		t.Fatalf("Recv() = %v, want ErrInvalidResponse", err)
	}
}

func TestStreamOpenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, nil)
	s, err := p.Stream(context.Background(), helloRequest())
	if s != nil {
		t.Fatal("expected a nil stream on open error")
	}
	if !errors.Is(err, llm.ErrAuth) {
		t.Fatalf("Stream() error = %v, want ErrAuth", err)
	}
}

// A healthy SSE stream must NOT be cut by Config.Timeout. Here the config
// timeout is shorter than the whole stream, the server keeps emitting frames
// the entire time, and llm.Request.Timeout is left unset. The stream must be
// read to completion and end with a done event, never llm.ErrTimeout. With the
// old cfg.Timeout fallback this test would fail at the first deadline.
func TestStreamNotCutByConfigTimeout(t *testing.T) {
	const (
		frameCount = 6
		frameGap   = 30 * time.Millisecond
		cfgTimeout = 40 * time.Millisecond // shorter than frameCount*frameGap
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		for i := 0; i < frameCount; i++ {
			select {
			case <-time.After(frameGap):
			case <-r.Context().Done():
				return
			}
			writeSSE(w, fl, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"content": fmt.Sprintf("tok%d", i)},
			}}})
		}
		writeSSE(w, fl, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, func(cfg *Config) {
		cfg.Timeout = cfgTimeout
	})
	s, err := p.Stream(context.Background(), helloRequest()) // req.Timeout unset
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	defer s.Close()

	var texts []string
	sawDone := false
	for {
		ev, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv() error = %v; a healthy stream was cut by Config.Timeout", err)
		}
		switch ev.Type {
		case llm.EventTypeTextDelta:
			texts = append(texts, ev.Text)
		case llm.EventTypeDone:
			sawDone = true
		}
	}
	if len(texts) != frameCount {
		t.Fatalf("got %d text deltas, want %d (stream was cut early): %v", len(texts), frameCount, texts)
	}
	for i, txt := range texts {
		if want := fmt.Sprintf("tok%d", i); txt != want {
			t.Fatalf("text delta %d = %q, want %q", i, txt, want)
		}
	}
	if !sawDone {
		t.Fatal("stream ended without a done event")
	}
}
