package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

func TestCompleteHappyPath(t *testing.T) {
	temp := 0.25
	req := Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "you are a helper"},
			{Role: RoleUser, Content: "hello"},
		},
		Model:       "fake-model",
		Temperature: &temp,
		MaxTokens:   128,
		Timeout:     3 * time.Second,
		SessionID:   "session-fake",
		Metadata:    map[string]string{"trace": "fake-trace"},
	}
	want := Response{
		Text:         "all good",
		FinishReason: FinishReasonStop,
		Usage:        Usage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10},
		Provider:     "fake",
		Model:        "fake-model",
		RequestID:    "req-fake",
		Latency:      12 * time.Millisecond,
	}
	p := NewFakeProvider("fake")
	p.QueueComplete(want, nil)
	got, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if got.Text != want.Text || got.FinishReason != want.FinishReason || got.Usage != want.Usage ||
		got.Provider != want.Provider || got.Model != want.Model || got.RequestID != want.RequestID || got.Latency != want.Latency {
		t.Fatalf("Complete() = %+v, want %+v", got, want)
	}
	recorded, ok := p.LastRequest()
	if !ok {
		t.Fatal("Complete did not record the request")
	}
	if recorded.Model != "fake-model" || recorded.MaxTokens != 128 || recorded.SessionID != "session-fake" {
		t.Fatalf("recorded request = %+v", recorded)
	}
	if recorded.Temperature == nil || *recorded.Temperature != 0.25 {
		t.Fatalf("recorded temperature = %v", recorded.Temperature)
	}
	if recorded.Metadata["trace"] != "fake-trace" || len(recorded.Messages) != 2 {
		t.Fatalf("recorded messages/metadata not preserved: %+v", recorded)
	}
}

func TestCompleteErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		code ErrorCode
		want error
	}{
		{"timeout", CodeTimeout, ErrTimeout},
		{"rate limited", CodeRateLimited, ErrRateLimited},
		{"invalid response", CodeInvalidResponse, ErrInvalidResponse},
		{"auth", CodeAuth, ErrAuth},
		{"tool call failed", CodeToolCallFailed, ErrToolCallFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewFakeProvider("fake")
			pe := &ProviderError{Provider: "fake", Code: c.code, Message: "scripted failure"}
			p.QueueComplete(Response{}, pe)
			_, err := p.Complete(context.Background(), Request{Model: "fake-model"})
			if err == nil {
				t.Fatal("Complete returned a nil error")
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("errors.Is(err, %v) = false", c.want)
			}
		})
	}
}

func TestCompletePlainErrorPassthrough(t *testing.T) {
	plain := errors.New("connection reset by fake")
	p := NewFakeProvider("fake")
	p.QueueComplete(Response{}, plain)
	_, err := p.Complete(context.Background(), Request{})
	if !errors.Is(err, plain) {
		t.Fatalf("plain error not returned as-is: %v", err)
	}
	if errors.Is(err, ErrTimeout) {
		t.Fatal("a plain error must not be mapped to a sentinel")
	}
}

func TestCompleteCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewFakeProvider("fake")
	p.QueueComplete(Response{Text: "should not surface"}, nil)
	if _, err := p.Complete(ctx, Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete on cancelled ctx = %v, want context.Canceled", err)
	}
}

func TestStreamEventsInOrder(t *testing.T) {
	p := NewFakeProvider("fake")
	usage := &Usage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}
	events := []Event{
		{Type: EventTypeTextDelta, Text: "Hel"},
		{Type: EventTypeTextDelta, Text: "lo"},
		{Type: EventTypeToolCallDelta, ToolCall: &ToolCall{ID: "call-1", Name: "quote", Arguments: json.RawMessage(`{"s":"SH600519"}`)}},
		{Type: EventTypeDone, FinishReason: FinishReasonStop, Usage: usage},
	}
	p.QueueStream(events, nil)
	s, err := p.Stream(context.Background(), Request{Model: "fake-model"})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	for i, want := range events {
		got, err := s.Recv()
		if err != nil {
			t.Fatalf("Recv(%d) returned error: %v", i, err)
		}
		if got.Type != want.Type || got.Text != want.Text || got.FinishReason != want.FinishReason {
			t.Fatalf("Recv(%d) = %+v, want %+v", i, got, want)
		}
		if want.Usage != nil && (got.Usage == nil || *got.Usage != *want.Usage) {
			t.Fatalf("Recv(%d) usage = %+v, want %+v", i, got.Usage, want.Usage)
		}
		if want.ToolCall != nil {
			if got.ToolCall == nil || got.ToolCall.ID != want.ToolCall.ID || !bytes.Equal(got.ToolCall.Arguments, want.ToolCall.Arguments) {
				t.Fatalf("Recv(%d) tool call = %+v, want %+v", i, got.ToolCall, want.ToolCall)
			}
		}
	}
	if _, err := s.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv after events = %v, want io.EOF", err)
	}
	if _, err := s.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("second Recv after events = %v, want io.EOF", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Close(); err != nil {
			t.Fatalf("Close(%d) = %v", i, err)
		}
	}
}

func TestStreamTerminalErrorMapping(t *testing.T) {
	p := NewFakeProvider("fake")
	pe := &ProviderError{Provider: "fake", Code: CodeInvalidResponse, Message: "bad json"}
	p.QueueStream([]Event{{Type: EventTypeTextDelta, Text: "partial"}}, pe)
	s, err := p.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	if _, err := s.Recv(); err != nil {
		t.Fatalf("first Recv error: %v", err)
	}
	_, err = s.Recv()
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("terminal error = %v, want ErrInvalidResponse", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamOpenError(t *testing.T) {
	want := &ProviderError{Provider: "fake", Code: CodeAuth, Message: "no credentials"}
	p := NewFakeProvider("fake")
	p.QueueStreamError(want)
	s, err := p.Stream(context.Background(), Request{})
	if s != nil {
		t.Fatal("Stream should return a nil stream on open error")
	}
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("open error = %v, want ErrAuth", err)
	}
}

// Cancellation must be observable through Recv and take priority over both
// buffered events and io.EOF, while Close stays idempotent.
func TestStreamContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := NewFakeProvider("fake")
	p.QueueStream([]Event{
		{Type: EventTypeTextDelta, Text: "A"},
		{Type: EventTypeTextDelta, Text: "B"},
	}, nil)
	s, err := p.Stream(ctx, Request{})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	cancel()
	for i := 0; i < 2; i++ {
		_, err := s.Recv()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Recv(%d) after cancel = %v, want context.Canceled", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := s.Close(); err != nil {
			t.Fatalf("Close(%d) = %v", i, err)
		}
	}
}

func TestStreamCancellationAfterPartialRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := NewFakeProvider("fake")
	p.QueueStream([]Event{
		{Type: EventTypeTextDelta, Text: "first"},
		{Type: EventTypeTextDelta, Text: "second"},
	}, nil)
	s, err := p.Stream(ctx, Request{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Recv()
	if err != nil || first.Text != "first" {
		t.Fatalf("first Recv = %+v, %v", first, err)
	}
	cancel()
	if _, err := s.Recv(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Recv after cancel during stream = %v, want context.Canceled", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	p := NewFakeProvider("fake")
	p.QueueStream([]Event{{Type: EventTypeTextDelta, Text: "A"}}, nil)
	s, err := p.Stream(ctx, Request{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stream on expired deadline = %v, want context.DeadlineExceeded", err)
	}
	if s != nil {
		t.Fatal("Stream should not return a stream when ctx is already done")
	}
}

func TestStreamCloseMakesRecvEOF(t *testing.T) {
	p := NewFakeProvider("fake")
	p.QueueStream([]Event{{Type: EventTypeTextDelta, Text: "never read"}}, nil)
	s, err := p.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Close(); err != nil {
			t.Fatalf("Close(%d) = %v", i, err)
		}
	}
	if _, err := s.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv on closed stream = %v, want io.EOF", err)
	}
}

// Tool call arguments and tool parameters are raw JSON and must survive the
// abstraction byte-for-byte (no decode/re-encode round trip).
func TestToolCallArgumentsPassthrough(t *testing.T) {
	args := json.RawMessage(`{"symbol":"SH600519","limit":10,"nested":{"flag":true}}`)
	params := json.RawMessage(`{"type":"object","properties":{"symbol":{"type":"string"}}}`)
	req := Request{
		Model: "fake-model",
		Messages: []Message{
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call-1", Name: "quote", Arguments: args}}},
			{Role: RoleTool, ToolCallID: "call-1", Content: `{"price":1700}`},
		},
		Tools: []ToolDefinition{{Name: "quote", Description: "fetch a quote", Parameters: params}},
	}
	respArgs := json.RawMessage(`{"codes":["000001","600519"],"top":3}`)

	p := NewFakeProvider("fake")
	p.QueueComplete(Response{ToolCalls: []ToolCall{{ID: "call-2", Name: "screen", Arguments: respArgs}}}, nil)
	got, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 1 || !bytes.Equal(got.ToolCalls[0].Arguments, respArgs) {
		t.Fatalf("response tool call args = %+v, want %s", got.ToolCalls, respArgs)
	}
	recorded, _ := p.LastRequest()
	if len(recorded.Messages) != 2 || len(recorded.Messages[0].ToolCalls) != 1 {
		t.Fatal("recorded request lost tool calls")
	}
	if !bytes.Equal(recorded.Messages[0].ToolCalls[0].Arguments, args) {
		t.Fatalf("request tool call args = %s, want %s", recorded.Messages[0].ToolCalls[0].Arguments, args)
	}
	if recorded.Messages[1].ToolCallID != "call-1" || recorded.Messages[1].Role != RoleTool {
		t.Fatal("tool result message not preserved")
	}
	if len(recorded.Tools) != 1 || !bytes.Equal(recorded.Tools[0].Parameters, params) {
		t.Fatalf("tool definition parameters = %s, want %s", recorded.Tools[0].Parameters, params)
	}
	if !json.Valid(recorded.Messages[0].ToolCalls[0].Arguments) {
		t.Fatal("tool call arguments stopped being valid JSON")
	}
}

func TestToolCallArgumentsPassthroughInStream(t *testing.T) {
	args := json.RawMessage(`{"x":1,"y":[2,3]}`)
	p := NewFakeProvider("fake")
	p.QueueStream([]Event{{Type: EventTypeToolCallDelta, ToolCall: &ToolCall{ID: "c", Name: "calc", Arguments: args}}}, nil)
	s, err := p.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := s.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if ev.ToolCall == nil || !bytes.Equal(ev.ToolCall.Arguments, args) {
		t.Fatalf("stream tool call args = %+v, want %s", ev.ToolCall, args)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFakeProviderRecordsRequestsInOrder(t *testing.T) {
	p := NewFakeProvider("fake")
	p.QueueComplete(Response{Text: "ok"}, nil)
	p.QueueStream([]Event{{Type: EventTypeDone}}, nil)
	p.QueueComplete(Response{Text: "ok2"}, nil)
	_, _ = p.Complete(context.Background(), Request{Model: "m1"})
	st, _ := p.Stream(context.Background(), Request{Model: "m2"})
	_ = st.Close()
	_, _ = p.Complete(context.Background(), Request{Model: "m3"})
	reqs := p.Requests()
	if len(reqs) != 3 {
		t.Fatalf("recorded %d requests, want 3", len(reqs))
	}
	want := []string{"m1", "m2", "m3"}
	for i, w := range want {
		if reqs[i].Model != w {
			t.Errorf("request[%d].Model = %q, want %q", i, reqs[i].Model, w)
		}
	}
}
