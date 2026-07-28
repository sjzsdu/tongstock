package server

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCoreRouteCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewServer(Dependencies{}).SetupRoutes(router)

	got := make(map[string]bool)
	for _, route := range router.Routes() {
		got[route.Method+" "+route.Path] = true
	}

	expected := []string{
		"GET /health",
		"GET /health/live",
		"GET /health/ready",
		"GET /health/diagnostics",
		"GET /health/data-sync",
		"GET /api/quote",
		"GET /api/quotes",
		"GET /api/codes",
		"GET /api/codes/list",
		"GET /api/codes/market",
		"GET /api/codes/marketcap",
		"GET /api/codes/stats",
		"GET /api/kline",
		"GET /api/index",
		"GET /api/minute",
		"GET /api/trade",
		"GET /api/auction",
		"GET /api/xdxr",
		"GET /api/finance",
		"GET /api/finance/trends",
		"GET /api/finance/metrics",
		"GET /api/company",
		"GET /api/company/content",
		"GET /api/block",
		"GET /api/block/files",
		"GET /api/block/list",
		"GET /api/block/show",
		"GET /api/count",
		"GET /api/indicator",
		"GET /api/screen",
		"GET /api/signal-analysis",
		"GET /api/stock/compare",
		"POST /api/strategy/overnight",
		"GET /api/history",
		"POST /api/history",
		"DELETE /api/history/:code",
		"GET /api/watchlist",
		"POST /api/watchlist",
		"DELETE /api/watchlist/:code",
		"PUT /api/watchlist/:code/note",
		"PUT /api/watchlist/:code/group",
		"GET /api/watchlist/groups",
		"GET /api/stockpool",
		"POST /api/stockpool",
		"DELETE /api/stockpool/:id",
		"POST /api/trades",
		"GET /api/trades",
		"GET /api/trades/positions",
		"DELETE /api/trades/:id",
		"POST /api/sync/daily",
		"GET /api/sync/state",
		"GET /api/sync/freshness",
		"POST /api/kline/clean",
		"GET /api/settings/indicator",
		"PUT /api/settings/indicator",
		"GET /api/stocks/search",
		"GET /api/stocks/search-index",
		"GET /api/stockinfo",
		"GET /api/stockinfo/:code",
		"POST /api/stockinfo/sync",
		"GET /api/stockinfo/count",
	}
	for _, route := range expected {
		if !got[route] {
			t.Errorf("public route disappeared during vertical split: %s", route)
		}
	}
}

func TestOpenAPIExactlyMatchesRegisteredPublicRoutes(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.json")
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var contract struct {
		Paths map[string]map[string]struct {
			OperationID string                     `json:"operationId"`
			Responses   map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewServer(Dependencies{
		Newsfeed: NewNewsfeedHandler(nil),
	}).SetupRoutes(router)

	registered := make(map[string]bool)
	pathParameter := regexp.MustCompile(`:([A-Za-z0-9_]+)`)
	for _, route := range router.Routes() {
		path := pathParameter.ReplaceAllString(route.Path, `{$1}`)
		method := strings.ToLower(route.Method)
		key := method + " " + path
		registered[key] = true
		operation, ok := contract.Paths[path][method]
		if !ok {
			t.Errorf("registered route is missing from OpenAPI: %s", key)
			continue
		}
		if operation.OperationID == "" {
			t.Errorf("OpenAPI operation has no operationId: %s", key)
		}
		if strings.HasPrefix(path, "/api/") {
			if _, ok := operation.Responses["500"]; !ok {
				t.Errorf("API operation does not declare the stable error envelope: %s", key)
			}
		}
	}

	operationIDs := make(map[string]string)
	for path, pathItem := range contract.Paths {
		for method, operation := range pathItem {
			key := method + " " + path
			if !registered[key] {
				t.Errorf("OpenAPI contains a stale route: %s", key)
			}
			if previous, duplicate := operationIDs[operation.OperationID]; duplicate {
				t.Errorf("duplicate operationId %q on %s and %s", operation.OperationID, previous, key)
			}
			operationIDs[operation.OperationID] = key
		}
	}
}
