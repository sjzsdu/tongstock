package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/paradigmspromote"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// promoteTestBars 故意返回无数据的 bar provider：让晋级在验证阶段快速 fail-closed，
// 专测 HTTP 层的异步调度与状态透出，不重复验证引擎本身的测试。
type promoteTestBars struct{}

func (promoteTestBars) LoadBars(context.Context, string, string, string, string) ([]validation.BacktestBar, error) {
	return nil, errors.New("no bars: test fail-closed")
}

type promoteTestBenchmark struct{}

func (promoteTestBenchmark) LoadDailyReturns(context.Context, string, string, string, string) (map[string]float64, error) {
	return nil, errors.New("no benchmark: test fail-closed")
}

type promoteTestSnapshots struct{}

func (promoteTestSnapshots) List(int, int) ([]*paradigm.DatasetSnapshot, error) {
	return []*paradigm.DatasetSnapshot{{
		ID: "snap-x", Universe: []string{"600000", "600001", "600002", "600003", "600004", "600005"},
		DateRange: paradigm.DateRange{Start: "2023-01-02", End: "2023-12-31"},
	}}, nil
}
func (promoteTestSnapshots) GetByID(id string) (*paradigm.DatasetSnapshot, error) {
	if id == "snap-x" {
		return &paradigm.DatasetSnapshot{ID: id, Universe: []string{"600000", "600001", "600002", "600003", "600004", "600005"}}, nil
	}
	return nil, errors.New("not found")
}
func (promoteTestSnapshots) VerifyContent(string) error { return nil }

type promoteTestUniverse struct{}

func (promoteTestUniverse) ResolveUniverse(context.Context, string, int, int) ([]string, int, error) {
	return []string{"600000", "600001", "600002", "600003", "600004", "600005"}, 0, nil
}

func newParadigmPromoteTestServer(t *testing.T) (*Server, *paradigms.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "promote.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	repo, err := methodregistryrepo.New(store)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := methodregistry.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	paradigmStore, err := paradigms.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := paradigmspromote.New(paradigmspromote.Deps{
		Paradigms: paradigmStore, Registry: registry,
		Snapshots: promoteTestSnapshots{}, Universe: promoteTestUniverse{},
		Bars: promoteTestBars{}, Benchmark: promoteTestBenchmark{},
	})
	if err != nil {
		t.Fatal(err)
	}
	api := NewServer(Dependencies{})
	api.SetParadigmStore(paradigmStore)
	api.SetParadigmPromote(svc)
	return api, paradigmStore
}

func TestParadigmPromoteAsyncLifecycle(t *testing.T) {
	api, paradigmStore := newParadigmPromoteTestServer(t)
	p := &paradigms.Paradigm{
		ID: "p-http", Name: "HTTP 范式", Side: "buy",
		BuyConds: []paradigms.Condition{{Indicator: "close", Operator: "gt", Value: "MA20"}},
	}
	if err := paradigmStore.Save(p); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	api.SetupParadigmRoutes(&router.RouterGroup)

	// POST /paradigm/:id/promote → 异步启动，立即返回 started + status_url。
	request := httptest.NewRequest(http.MethodPost, "/paradigm/p-http/promote", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var started struct {
		Started   bool   `json:"started"`
		StatusURL string `json:"status_url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if !started.Started || started.StatusURL != "/api/paradigm/p-http/promotion/status" {
		t.Fatalf("async contract broken: %+v", started)
	}

	// GET status → 等 run 跑完，断言 fail-closed 结果被透出。
	deadline := time.Now().Add(5 * time.Second)
	var result struct {
		ParadigmID string `json:"paradigm_id"`
		Run        *struct {
			Status  string `json:"status"`
			Outcome *struct {
				Status string `json:"status"`
			} `json:"outcome"`
		} `json:"run"`
	}
	for {
		request := httptest.NewRequest(http.MethodGet, "/paradigm/p-http/promotion/status", nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Run != nil && (result.Run.Status == "done" || result.Run.Status == "failed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("promotion did not finish in time, last = %s", response.Body.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if result.Run == nil || result.Run.Status != "done" || result.Run.Outcome == nil || result.Run.Outcome.Status != "blocked" {
		t.Fatalf("expected done + blocked outcome, got %+v", result)
	}

	updated, err := paradigmStore.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ReviewStatus != "" {
		t.Errorf("blocked promotion must not change review status, got %q", updated.ReviewStatus)
	}
	if updated.Evidence == nil || len(updated.Evidence.MustFix) == 0 {
		t.Errorf("blockers must be persisted for the frontend, got %+v", updated.Evidence)
	}
}

func TestParadigmPromoteUnknownParadigmReturns404(t *testing.T) {
	api, _ := newParadigmPromoteTestServer(t)
	router := gin.New()
	api.SetupParadigmRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/paradigm/missing/promote", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestParadigmPromoteSellSideRejectedBeforeStart(t *testing.T) {
	api, paradigmStore := newParadigmPromoteTestServer(t)
	p := &paradigms.Paradigm{
		ID: "p-sell-http", Name: "卖出范式", Side: "sell",
		BuyConds: []paradigms.Condition{{Indicator: "close", Operator: "gt", Value: "MA20"}},
	}
	if err := paradigmStore.Save(p); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.SetupParadigmRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/paradigm/p-sell-http/promote", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.Code)
	}
}

func TestParadigmPromoteNotConfiguredReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewServer(Dependencies{})
	router := gin.New()
	api.SetupParadigmRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/paradigm/any/promote", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}
