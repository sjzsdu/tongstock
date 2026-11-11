// Package llm defines TongStock's provider-agnostic LLM abstraction.
//
// It contains only standard-library types and interfaces: no vendor SDK, no
// agent framework, and no credentials. Concrete provider adapters live in
// sub-packages and are responsible for reading API keys from the environment
// or secure configuration. Neither this package nor its errors should ever
// carry a full prompt or an API key.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Role identifies the author of a Message.
type Role string

// Roles covering the messages a provider may see. ToolCallID is only meaningful
// for RoleTool, where it links the message back to the originating ToolCall.ID.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleDeveloper Role = "developer"
)

// Message is a single turn in a conversation.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolCall is a request from the model to invoke a tool. Arguments is kept as
// raw JSON so callers receive the exact bytes the provider sent, without a
// decode/re-encode round trip.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolDefinition describes a tool the model may call. Parameters is a JSON
// Schema document, kept raw for the same reason as ToolCall.Arguments.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// Usage reports token consumption. Fields are zero when a provider does not
// report usage.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Request is the input to a provider call. It deliberately holds no API key;
// credentials are adapter configuration, never request data.
type Request struct {
	Messages    []Message
	Model       string
	Temperature *float64
	MaxTokens   int
	Timeout     time.Duration
	Tools       []ToolDefinition
	SessionID   string
	Metadata    map[string]string
}

// FinishReason explains why a response stopped. Providers may return values
// outside this set; convert with FinishReason("...").
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonError         FinishReason = "error"
)

// Response is the result of a non-streaming Complete call.
type Response struct {
	Text         string
	FinishReason FinishReason
	Usage        Usage
	ToolCalls    []ToolCall
	Provider     string
	Model        string
	RequestID    string
	Latency      time.Duration
}

// EventType classifies a Stream Event.
type EventType string

const (
	EventTypeTextDelta     EventType = "text_delta"
	EventTypeToolCallDelta EventType = "tool_call_delta"
	EventTypeDone          EventType = "done"
	EventTypeError         EventType = "error"
)

// Event is one increment of a streaming response. Only the fields relevant to
// Type are populated; ToolCall and Usage are nil when absent. Err carries the
// terminal failure on EventTypeError.
type Event struct {
	Type         EventType
	Text         string
	ToolCall     *ToolCall
	FinishReason FinishReason
	Err          error
	Usage        *Usage
}

// Stream is a pull-based, cancellable stream of events.
//
// A provider must bind the context passed to Provider.Stream to the upstream
// request: after cancellation Recv returns the context error rather than more
// events. Recv returns io.EOF once the stream is exhausted. Close releases
// resources and must be idempotent; callers should always call it. There is no
// callback-based streaming (e.g. Request.OnDelta) in this abstraction.
type Stream interface {
	Recv() (Event, error)
	Close() error
}

// Provider is the contract every LLM backend implements.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
	Stream(ctx context.Context, req Request) (Stream, error)
}

// Stable sentinel errors. Provider adapters map backend failures onto these so
// callers can branch with errors.Is without depending on vendor error strings.
var (
	ErrTimeout         = errors.New("llm: request timed out")
	ErrRateLimited     = errors.New("llm: rate limited")
	ErrInvalidResponse = errors.New("llm: invalid response")
	ErrAuth            = errors.New("llm: authentication failed")
	ErrToolCallFailed  = errors.New("llm: tool call failed")
)

// ErrorCode is a stable, provider-independent classification carried by
// ProviderError.
type ErrorCode string

const (
	CodeTimeout         ErrorCode = "timeout"
	CodeRateLimited     ErrorCode = "rate_limited"
	CodeInvalidResponse ErrorCode = "invalid_response"
	CodeAuth            ErrorCode = "auth"
	CodeToolCallFailed  ErrorCode = "tool_call_failed"
	CodeUnknown         ErrorCode = "unknown"
)

func sentinelFor(code ErrorCode) error {
	switch code {
	case CodeTimeout:
		return ErrTimeout
	case CodeRateLimited:
		return ErrRateLimited
	case CodeInvalidResponse:
		return ErrInvalidResponse
	case CodeAuth:
		return ErrAuth
	case CodeToolCallFailed:
		return ErrToolCallFailed
	default:
		return nil
	}
}

// ProviderError is a structured backend failure. Message must be a short,
// non-sensitive summary: never a full prompt, a response body containing
// secrets, or an API key.
type ProviderError struct {
	Provider   string
	Code       ErrorCode
	Message    string
	RetryAfter time.Duration
}

// Error implements error. It emits only provider, code and the short message.
func (e ProviderError) Error() string {
	return fmt.Sprintf("llm provider %q returned %s: %s", e.Provider, e.Code, e.Message)
}

// Is lets errors.Is match a ProviderError against the stable sentinel that
// corresponds to its Code. A value receiver keeps both ProviderError and
// *ProviderError usable as errors.
func (e ProviderError) Is(target error) bool {
	s := sentinelFor(e.Code)
	return s != nil && target == s
}
