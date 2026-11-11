package llm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRoleConstants(t *testing.T) {
	cases := []struct {
		got  Role
		want string
	}{
		{RoleSystem, "system"},
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{RoleTool, "tool"},
		{RoleDeveloper, "developer"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("role = %q, want %q", c.got, c.want)
		}
	}
}

func TestEventTypeConstants(t *testing.T) {
	cases := []struct {
		got  EventType
		want string
	}{
		{EventTypeTextDelta, "text_delta"},
		{EventTypeToolCallDelta, "tool_call_delta"},
		{EventTypeDone, "done"},
		{EventTypeError, "error"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("event type = %q, want %q", c.got, c.want)
		}
	}
}

// Every ErrorCode maps to exactly one stable sentinel, so callers can branch
// with errors.Is without parsing vendor error strings.
func TestProviderErrorMapsToSentinels(t *testing.T) {
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
		{"unknown", CodeUnknown, nil},
	}
	sentinels := []error{ErrTimeout, ErrRateLimited, ErrInvalidResponse, ErrAuth, ErrToolCallFailed}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pe := &ProviderError{
				Provider: "fake",
				Code:     c.code,
				Message:  "short non-sensitive summary",
			}
			var asError error = pe
			if c.want == nil {
				for _, s := range sentinels {
					if errors.Is(asError, s) {
						t.Errorf("unknown code unexpectedly matched %v", s)
					}
				}
				return
			}
			if !errors.Is(asError, c.want) {
				t.Fatalf("errors.Is(err, %v) = false, want true", c.want)
			}
			// A value (not a pointer) must behave identically.
			if !errors.Is(ProviderError{Code: c.code}, c.want) {
				t.Errorf("value ProviderError did not match %v", c.want)
			}
			// It must not cross-match any other sentinel.
			for _, s := range sentinels {
				if s != c.want && errors.Is(asError, s) {
					t.Errorf("code %s unexpectedly matched %v", c.code, s)
				}
			}
		})
	}
}

func TestProviderErrorIsWrappable(t *testing.T) {
	pe := &ProviderError{Provider: "fake", Code: CodeAuth, Message: "bad credentials"}
	wrapped := fmt.Errorf("complete failed: %w", pe)
	if !errors.Is(wrapped, ErrAuth) {
		t.Fatal("wrapped ProviderError lost its sentinel mapping")
	}
	var target *ProviderError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As did not recover *ProviderError")
	}
	if target.Provider != "fake" || target.Code != CodeAuth {
		t.Fatalf("recovered ProviderError = %+v", target)
	}
}

// Error() must stay a short, non-sensitive summary: provider, code and the
// caller-supplied message only. Callers are responsible for keeping Message
// free of prompts and secrets.
func TestProviderErrorStringIsMinimal(t *testing.T) {
	pe := ProviderError{Provider: "fake", Code: CodeTimeout, Message: "deadline exceeded"}
	got := pe.Error()
	for _, want := range []string{"fake", "timeout", "deadline exceeded"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}
