package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAgentChatSessionSavedAndListedFromChatStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := NewChatStore("")
	if err != nil {
		t.Fatalf("NewChatStore: %v", err)
	}
	s := &Server{agentState: &AgentState{workspace: t.TempDir(), defaults: AgentDefaults{Agent: "stock", Session: "web:default"}, chatStore: store}}

	s.saveAgentChatSession("web:stock:test", "stock", "分析 300418", "这是分析结果")

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/agent/transcript?session=web:stock:test&agent=stock", nil)
	s.handleAgentTranscript(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var transcript agentTranscriptResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &transcript); err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
	if transcript.Missing {
		t.Fatalf("transcript missing: %#v", transcript)
	}
	if transcript.Path != "chat_store:web:stock:test" {
		t.Fatalf("path = %q", transcript.Path)
	}
	if len(transcript.Messages) != 2 || transcript.Messages[0].Role != "user" || transcript.Messages[1].Content != "这是分析结果" {
		t.Fatalf("messages not persisted: %#v", transcript.Messages)
	}

	rec = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/agent/sessions", nil)
	s.handleAgentSessions(ctx)

	var sessions agentSessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1: %#v", len(sessions.Sessions), sessions)
	}
	if sessions.Sessions[0].Session != "web:stock:test" || sessions.Sessions[0].Path != "chat_store:web:stock:test" {
		t.Fatalf("stored session not listed: %#v", sessions.Sessions[0])
	}
}

func TestMergeStoredAgentSessionsPrefersStoredWhenNewer(t *testing.T) {
	old := time.Now().Add(-time.Hour).Format(time.RFC3339)
	newer := time.Now().Format(time.RFC3339)
	storedUpdated, err := time.Parse(time.RFC3339, newer)
	if err != nil {
		t.Fatal(err)
	}
	merged := mergeStoredAgentSessions(
		[]agentSessionInfo{{Session: "same", Agent: "old", Path: "file", UpdatedAt: old}},
		[]*ChatSession{{ID: "same", Agent: "new", UpdatedAt: storedUpdated, Messages: []ChatMessage{{Role: "user", Content: "hi"}}}},
	)
	if len(merged) != 1 {
		t.Fatalf("merged len = %d", len(merged))
	}
	if merged[0].Agent != "new" || merged[0].Path != "chat_store:same" {
		t.Fatalf("stored session did not replace older transcript: %#v", merged[0])
	}
}
