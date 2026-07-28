package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorEnvelopeConvertsLegacyFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), ErrorEnvelopeMiddleware())
	router.GET("/api/fail", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "dial tcp 10.0.0.1: password=secret"})
	})

	request := httptest.NewRequest(http.MethodGet, "/api/fail", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"code":"internal_error"`) || !strings.Contains(body, `"request_id":`) {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, "10.0.0.1") || strings.Contains(body, "secret") {
		t.Fatalf("internal error leaked: %s", body)
	}
}

func TestReadinessReflectsUnavailableAndDegradedModules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		diagnostic Diagnostics
		status     int
	}{
		{
			name: "degraded optional module remains ready",
			diagnostic: Diagnostics{
				Status: "degraded", Service: "tongstock",
				Modules: map[string]ModuleHealth{"agent": {Status: "degraded"}},
			},
			status: http.StatusOK,
		},
		{
			name: "unavailable core module fails readiness",
			diagnostic: Diagnostics{
				Status: "unavailable", Service: "tongstock",
				Modules: map[string]ModuleHealth{"database": {Status: "unavailable"}},
			},
			status: http.StatusServiceUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := NewServer(Dependencies{Diagnostics: DiagnosticsFunc(func(context.Context) Diagnostics {
				return test.diagnostic
			})})
			router := gin.New()
			api.setupHealthRoutes(router)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			if response.Code != test.status {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRecoveryReturnsCorrelatedSafeEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery(), ErrorEnvelopeMiddleware())
	router.GET("/panic", func(*gin.Context) { panic("password=secret") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	var envelope ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "internal_error" || envelope.Error.RequestID == "" {
		t.Fatalf("envelope = %+v", envelope)
	}
	if strings.Contains(response.Body.String(), "secret") {
		t.Fatal("panic detail leaked")
	}
}

func TestAgentStreamPreflightUsesErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &Server{}
	router := gin.New()
	router.Use(RequestID())
	group := router.Group("/api")
	api.SetupAgentRoutes(group)
	request := httptest.NewRequest(http.MethodPost, "/api/agent/chat/stream", strings.NewReader(`{"message":"hi"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var envelope ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || envelope.Error.Code != "agent_unavailable" {
		t.Fatalf("status=%d envelope=%+v", response.Code, envelope)
	}
}

func TestAgentStreamPostHeaderFailureUsesCodedCorrelatedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Set(requestIDKey, "stream-request-1")

	writeSSEError(context, response, "upstream_timeout", "Agent 处理超时")
	body := strings.TrimSpace(response.Body.String())
	if !strings.HasPrefix(body, "data: ") {
		t.Fatalf("SSE body = %q", body)
	}
	var event agentStreamEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(body, "data: ")), &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "error" || event.Code != "upstream_timeout" ||
		event.RequestID != "stream-request-1" || event.Message == "" {
		t.Fatalf("event = %+v", event)
	}
}

func TestRequestIDPreservesValidCallerID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, RequestIDFromContext(c))
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", "client-request-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get("X-Request-ID"); got != "client-request-1" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	if response.Body.String() != "client-request-1" {
		t.Fatalf("body = %q", response.Body.String())
	}
}
