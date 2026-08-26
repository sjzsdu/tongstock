package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentDiagnoseReportsBuiltinModelConfigurationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}
	err := s.InitAgentStateWithOptions(AgentRuntimeOptions{Backend: "builtin"})
	if err == nil {
		t.Fatal("expected initialization error")
	}

	router := gin.New()
	router.GET("/diagnose", s.handleAgentDiagnose)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnose", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got agentDiagnosticResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Ready {
		t.Fatalf("diagnostic state = enabled:%v ready:%v, want enabled and degraded", got.Enabled, got.Ready)
	}
	if len(got.Errors) == 0 || !strings.Contains(got.Errors[0], "agent.model") {
		t.Fatalf("errors = %#v, want explicit agent.model error", got.Errors)
	}
	if len(got.Hints) == 0 || !strings.Contains(got.Hints[0], "backend: picoclaw") {
		t.Fatalf("hints = %#v, want migration guidance", got.Hints)
	}
}
