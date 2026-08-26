package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestMethodRegistryReadOnlyProductAPI(t *testing.T) {
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "methods-api.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo, err := methodregistryrepo.New(store)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := methodregistry.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	pct := 0.1
	compiled, _, err := methods.Compile(&methods.Candidate{Name: "20日均线方法", Universe: "universe_all", Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"}, HoldingMaxDays: 8, PositionMode: "pct_equity", PositionPct: &pct})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := registry.Register(t.Context(), methodregistry.Registration{FamilyID: "api-family", VariantID: "v1", Market: "A", EntrySummary: "收盘价站上20日均线", ExitSummary: "最长持有8个交易日", Method: compiled})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := NewServer(Dependencies{})
	api.SetMethodRegistry(registry)
	api.SetupRoutes(router)
	for _, path := range []string{"/api/methods?status=candidate&market=A&holding_max_days=10", "/api/methods/" + registered.ID, "/api/method-families/api-family", "/api/methods/" + registered.ID + "/audit"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
		if path == "/api/methods/"+registered.ID && contains(response.Body.String(), "compiled_at") {
			t.Fatal("product method card leaked compiled experiment details")
		}
	}
}
func contains(value, part string) bool {
	return len(value) >= len(part) && strings.Contains(value, part)
}
