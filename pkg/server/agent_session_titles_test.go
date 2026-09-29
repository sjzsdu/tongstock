package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionTitleUsesFirstUserMessage(t *testing.T) {
	title := sessionTitleFromMessages([]ChatMessage{
		{Role: "assistant", Content: "我是助手"},
		{Role: "user", Content: "  帮我看看\n  600519 现在能不能买？  "},
		{Role: "assistant", Content: "好的"},
	})
	if title != "帮我看看 600519 现在能不能买？" {
		t.Fatalf("title=%q", title)
	}
}

func TestSessionTitleIgnoresAssistantOnlyHistory(t *testing.T) {
	if title := sessionTitleFromMessages([]ChatMessage{{Role: "assistant", Content: "hello"}}); title != "" {
		t.Fatalf("title=%q, want empty", title)
	}
}

func TestAgentSessionTitleTruncatesToShortLabel(t *testing.T) {
	title := agentSessionTitle(strings.Repeat("字", 60))
	if got := len([]rune(title)); got != 31 {
		t.Fatalf("rune length=%d, want 31 (30 + ellipsis)", got)
	}
	if !strings.HasSuffix(title, "…") {
		t.Fatalf("truncated title should end with ellipsis: %q", title)
	}
}

func TestMergeStoredAgentSessionsExposesTitle(t *testing.T) {
	merged := mergeStoredAgentSessions(nil, []*ChatSession{{
		ID:        "web:demo",
		Agent:     "stock-analyst",
		Messages:  []ChatMessage{{Role: "user", Content: "看看 000001"}},
		UpdatedAt: time.Now(),
	}})
	if len(merged) != 1 {
		t.Fatalf("merged=%d, want 1", len(merged))
	}
	if merged[0].Title != "看看 000001" {
		t.Fatalf("title=%q", merged[0].Title)
	}
}

func TestListSessionsReadsTitleFromTranscriptFile(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".tongstock", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{
		`{"role":"assistant","content":"hi"}`,
		`{"role":"user","content":"帮我看看大盘"}`,
		`{"role":"assistant","content":"大盘上行"}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "tongstock_default.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := listSessions(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions=%d, want 1", len(sessions))
	}
	if sessions[0].Title != "帮我看看大盘" {
		t.Fatalf("title=%q", sessions[0].Title)
	}
}
