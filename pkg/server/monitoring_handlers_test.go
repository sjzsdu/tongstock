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
	api.registerMonitoringRoutes(&router.RouterGroup)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/monitoring/report", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"available":false`) ||
		!strings.Contains(response.Body.String(), "真实观测") {
		t.Fatalf("missing report did not fail closed: %s", response.Body.String())
	}
}
