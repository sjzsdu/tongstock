package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// Shared characterization helpers for legacy error-site migration rounds.
// Each golden case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emitted for a legacy {"error": ...} response,
// so the WriteError conversion must not change any observable output.

const legacyErrorContractRequestID = "error-contract-req"

func legacyErrorGoldenBody(status int) string {
	code, message := statusError(status)
	envelope := map[string]any{"error": APIError{
		Code:      code,
		Message:   message,
		RequestID: legacyErrorContractRequestID,
	}}
	data, err := json.Marshal(envelope)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// newContractGinEngine builds a router with the same middleware stack and
// route table every error-contract test exercises. All contract routers
// (market_contract_test.go's contractRouter included) share this helper.
func newContractGinEngine(t *testing.T, deps Dependencies) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery())
	NewServer(deps).SetupRoutes(router)
	return router
}

func doLegacyErrorRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Request-ID", legacyErrorContractRequestID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertLegacyErrorGolden(t *testing.T, response *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, wantStatus, response.Body.String())
	}
	want := legacyErrorGoldenBody(wantStatus)
	if got := response.Body.String(); got != want {
		t.Fatalf("body mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// stubExecutor satisfies tdx.Executor without any network dependency. When
// doErr is non-nil every Do call fails, which drives client-backed service
// methods into their error branches.
type stubExecutor struct {
	doErr error
}

func (e stubExecutor) Do(fn func(c *tdx.Client) error) error                        { return e.doErr }
func (e stubExecutor) DoContext(_ context.Context, _ func(*tdx.Client) error) error { return e.doErr }
func (e stubExecutor) Close() error                                                 { return nil }
func (e stubExecutor) Len() int                                                     { return 0 }
func (e stubExecutor) Status() tdx.ExecutorStatus                                   { return tdx.ExecutorStatus{} }

// testLocalNormalizeJSON replicates the middleware's value treatment: it
// re-marshals values parsed from the legacy body, so structs come out with
// sorted json-key order and numbers keep float64 formatting. Deliberately
// test-local: the production normalizeJSONValue must not be called from
// tests, or a shared bug would pass both sides.
func testLocalNormalizeJSON(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return value
	}
	return out
}

// legacyErrorMixedGoldenBody pins the exact bytes ErrorEnvelopeMiddleware
// produces for a legacy {"error": ...} body carrying extra top-level keys:
// the error envelope plus every extra value after the json round-trip,
// marshaled from a map (sorted top-level keys).
func legacyErrorMixedGoldenBody(status int, extra map[string]any) string {
	code, message := statusError(status)
	envelope := map[string]any{"error": APIError{
		Code:      code,
		Message:   message,
		RequestID: legacyErrorContractRequestID,
	}}
	for key, value := range extra {
		if key == "error" {
			continue
		}
		envelope[key] = testLocalNormalizeJSON(value)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// assertLegacyErrorMixedGolden pins status + byte-exact body for a
// mixed-key legacy response.
func assertLegacyErrorMixedGolden(t *testing.T, response *httptest.ResponseRecorder, status int, extra map[string]any) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	want := legacyErrorMixedGoldenBody(status, extra)
	if got := response.Body.String(); got != want {
		t.Fatalf("body mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// parsedErrorExtras returns the response's non-error top-level keys, parsed
// the same way the middleware parses the legacy body before re-marshaling.
func parsedErrorExtras(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response is not a JSON object: %v; body = %s", err, response.Body.String())
	}
	extras := make(map[string]any, len(parsed))
	for key, value := range parsed {
		if key != "error" {
			extras[key] = value
		}
	}
	return extras
}
