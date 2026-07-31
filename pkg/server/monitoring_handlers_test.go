package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestMonitoringReportOnlyReturnsSubmittedObservedSeries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewServer(Dependencies{})
	router := gin.New()
	api.registerMonitoringRoutes(&router.RouterGroup)

	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	requestBody := monitoringRunRequest{Source: "forward-ledger:run-real-1"}
	for index := 0; index < 30; index++ {
		requestBody.BaselineReturns = append(requestBody.BaselineReturns, float64(index%5-2)/100)
		requestBody.ForwardReturns = append(requestBody.ForwardReturns, float64(index%3-1)/100)
		requestBody.ForwardDates = append(requestBody.ForwardDates, start.AddDate(0, 0, index))
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	runResponse := httptest.NewRecorder()
	runRequest := httptest.NewRequest(http.MethodPost, "/monitoring/run", bytes.NewReader(payload))
	runRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(runResponse, runRequest)
	if runResponse.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", runResponse.Code, runResponse.Body.String())
	}

	reportResponse := httptest.NewRecorder()
	router.ServeHTTP(reportResponse, httptest.NewRequest(http.MethodGet, "/monitoring/report", nil))
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("report status=%d body=%s", reportResponse.Code, reportResponse.Body.String())
	}
	var result monitoringReportResponse
	if err := json.Unmarshal(reportResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Report.Source != requestBody.Source ||
		!result.Report.Period.StartDate.Equal(requestBody.ForwardDates[0]) ||
		!result.Report.Period.EndDate.Equal(requestBody.ForwardDates[len(requestBody.ForwardDates)-1]) {
		t.Fatalf("report provenance does not match submitted observations: %+v", result.Report)
	}
}

func TestMonitoringRunRequiresDatedObservedSeries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewServer(Dependencies{})
	router := gin.New()
	api.registerMonitoringRoutes(&router.RouterGroup)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/monitoring/run", strings.NewReader(
		`{"source":"forward-ledger:run-real-1","baseline_returns":[0.01],"forward_returns":[0.02]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "forward_dates") {
		t.Fatalf("undated observations were accepted: status=%d body=%s", response.Code, response.Body.String())
	}
}
