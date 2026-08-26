package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentHTTPAPIsResolveAliasesToCanonicalIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := newFakeAgentProvider(t)
	defer provider.Close()
	t.Setenv("TEST_AGENT_ALIAS_KEY", "test-key")

	agents := []EmbeddedAgent{
		{ID: "Risk-Reviewer", Name: "Risk Reviewer", Aliases: []string{"risk", "风险复核"}, Prompt: "Review risk."},
		{ID: "stock-discussion-host", Name: "Host", Prompt: "Summarize."},
	}
	s := &Server{}
	s.SetAgentLister(func() ([]EmbeddedAgent, error) { return agents, nil })
	if err := s.InitAgentStateWithOptions(AgentRuntimeOptions{
		Backend: "builtin", Provider: "openai", APIBase: provider.URL,
		APIKeyEnv: "TEST_AGENT_ALIAS_KEY", Model: "test-model", Agent: "Risk-Reviewer",
		Workspace: t.TempDir(),
	}); err != nil {
		t.Fatalf("InitAgentStateWithOptions: %v", err)
	}
	t.Cleanup(func() { s.agentState.runner.Close() })
	store, err := NewChatStore("")
	if err != nil {
		t.Fatal(err)
	}
	s.agentState.chatStore = store

	router := gin.New()
	api := router.Group("/api")
	s.SetupAgentRoutes(api)

	postJSON(t, router, "/api/agent/chat", `{"message":"hello","agent":"RISK-REVIEWER","session":"alias-chat"}`, http.StatusOK)
	chat, err := store.Get("alias-chat")
	if err != nil {
		t.Fatalf("load canonical chat session: %v", err)
	}
	if chat.Agent != "Risk-Reviewer" {
		t.Fatalf("chat agent = %q, want canonical ID", chat.Agent)
	}

	streamBody := postJSON(t, router, "/api/agent/chat/stream", `{"message":"hello","agent":"风险复核","session":"alias-stream"}`, http.StatusOK)
	if !strings.Contains(streamBody, `"type":"done"`) {
		t.Fatalf("stream did not complete: %s", streamBody)
	}
	stream, err := store.Get("alias-stream")
	if err != nil {
		t.Fatalf("load canonical stream session: %v", err)
	}
	if stream.Agent != "Risk-Reviewer" {
		t.Fatalf("stream agent = %q, want canonical ID", stream.Agent)
	}

	debateBody := postJSON(t, router, "/api/agent/debate", `{"stock_code":"000001","agents":["risk"]}`, http.StatusOK)
	var debate agentDebateResponse
	if err := json.Unmarshal([]byte(debateBody), &debate); err != nil {
		t.Fatalf("decode debate response: %v; body=%s", err, debateBody)
	}
	if len(debate.Participants) != 1 || debate.Participants[0].Agent != "Risk-Reviewer" {
		t.Fatalf("debate participants did not use canonical ID: %#v", debate.Participants)
	}
}

func newFakeAgentProvider(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if streaming, _ := body["stream"].(bool); streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "ok"}, "finish_reason": "stop",
			}},
		})
	}))
}

func postJSON(t *testing.T, handler http.Handler, path, body string, wantStatus int) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("POST %s status = %d, want %d; body=%s", path, recorder.Code, wantStatus, recorder.Body.String())
	}
	return recorder.Body.String()
}
