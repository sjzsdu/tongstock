package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type contractRepository struct {
	coverage stockdata.Coverage
	dataset  stockdata.Dataset
	err      error
}

func (r contractRepository) InspectCoverage(context.Context, stockdata.DataSpec) (stockdata.Coverage, error) {
	return r.coverage, r.err
}

func (r contractRepository) Query(context.Context, stockdata.DataSpec) (stockdata.Dataset, error) {
	return r.dataset, r.err
}

func (contractRepository) SaveSynced(context.Context, stockdata.DataSpec, stockdata.Dataset, stockdata.SyncMetadata) error {
	return errors.New("unexpected synchronization")
}

type contractProvider struct{}

func (contractProvider) Sync(context.Context, stockdata.SyncRequest) (stockdata.Dataset, stockdata.SyncMetadata, error) {
	return stockdata.Dataset{}, stockdata.SyncMetadata{}, errors.New("unexpected provider call")
}

type contractFreshnessPolicy struct{}

func (contractFreshnessPolicy) Evaluate(
	context.Context,
	time.Time,
	stockdata.DataSpec,
	stockdata.Coverage,
) (stockdata.FreshnessDecision, error) {
	return stockdata.FreshnessDecision{Fresh: true, Reason: "contract_fixture"}, nil
}

func contractRouter(t *testing.T, repository stockdata.Repository) *gin.Engine {
	t.Helper()
	service, err := stockdata.NewService(repository, contractProvider{}, contractFreshnessPolicy{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), ErrorEnvelopeMiddleware(), Recovery())
	NewServer(Dependencies{UnifiedData: service}).SetupRoutes(router)
	return router
}

func TestQuoteHandlerSuccessFollowsContract(t *testing.T) {
	asOf := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	router := contractRouter(t, contractRepository{
		coverage: stockdata.Coverage{Exists: true, SourceUpdatedAt: asOf},
		dataset: stockdata.Dataset{Quote: &protocol.QuoteItem{
			Code: "600000", Name: "浦发银行", Price: 12.34,
		}},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/quote?code=600000&consistency=cache_only", nil)
	request.Header.Set("X-Request-ID", "contract-success")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["Code"] != "600000" || body["Price"] != 12.34 {
		t.Fatalf("unexpected quote response: %#v", body)
	}
	if response.Header().Get("X-Data-Freshness") != "fresh" ||
		response.Header().Get("X-Data-Sync-Status") != "cache" ||
		response.Header().Get("X-Request-ID") != "contract-success" {
		t.Fatalf("missing contract headers: %#v", response.Header())
	}
}

func TestQuoteHandlerDomainErrorFollowsContract(t *testing.T) {
	router := contractRouter(t, contractRepository{})
	request := httptest.NewRequest(http.MethodGet, "/api/quote?code=600000&consistency=cache_only", nil)
	request.Header.Set("X-Request-ID", "contract-domain")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != string(stockdata.ErrCacheMiss) ||
		body.Error.RequestID != "contract-domain" {
		t.Fatalf("unexpected error envelope: %#v", body)
	}
}

func TestQuoteHandlerPersistenceErrorIsSafe(t *testing.T) {
	router := contractRouter(t, contractRepository{err: errors.New("sqlite password=secret")})
	request := httptest.NewRequest(http.MethodGet, "/api/quote?code=600000", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); body == "" ||
		responseContainsAny(body, "sqlite", "password", "secret") {
		t.Fatalf("internal detail leaked: %s", body)
	}
}

func TestSyncStateUsesUnifiedKlineCoverage(t *testing.T) {
	first := time.Date(2026, 7, 27, 0, 0, 0, 0, time.Local)
	last := time.Date(2026, 7, 28, 0, 0, 0, 0, time.Local)
	router := contractRouter(t, contractRepository{
		coverage: stockdata.Coverage{
			Exists: true, Start: first, End: last, Points: []time.Time{first, last},
			LastSyncAt: time.Date(2026, 7, 28, 16, 0, 0, 0, time.Local), Status: "ok",
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/sync/state?code=600000&ktype=day", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["last_date"] != "20260728" || body["row_count"] != float64(2) || body["freshness"] != "fresh" {
		t.Fatalf("unexpected sync state: %#v", body)
	}
}

func TestSyncStateDoesNotInferSuccessfulSyncFromCachedRows(t *testing.T) {
	day := time.Date(2026, 8, 3, 0, 0, 0, 0, time.Local)
	router := contractRouter(t, contractRepository{
		coverage: stockdata.Coverage{
			Exists: true, Start: day, End: day, Points: []time.Time{day},
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/sync/state?code=601688&ktype=day", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "unknown" {
		t.Fatalf("status = %v, want unknown", body["status"])
	}
	if _, exists := body["last_sync_at"]; exists {
		t.Fatalf("missing sync timestamp was serialized: %#v", body)
	}
}

func responseContainsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
