package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMonitoringReportRefusesToInventMissingObservations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewServer(Dependencies{})
	router := gin.New()
	router.Use(RequestID(), ErrorEnvelopeMiddleware(), Recovery())
	api.registerMonitoringRoutes(&router.RouterGroup)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/monitoring/report", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	// The handler must fail closed: 404 with available=false inside the
	// error envelope, instead of inventing a report.
	if !strings.Contains(response.Body.String(), `"available":false`) ||
		!strings.Contains(response.Body.String(), `"code":"not_found"`) {
		t.Fatalf("missing report did not fail closed: %s", response.Body.String())
	}
}
