package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/internal/trading"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func newMarketTestServer(t *testing.T) (*Server, *gin.Engine, *methodregistry.Registry, *ledger.SignalLedger, *marketsnapshotrepo.SQLiteRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "method-market.db")})
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
	snapshots, _ := marketsnapshotrepo.New(store)
	runs, _ := selectionrepo.New(store)
	engine, err := selection.NewEngine(snapshots, repo, runs)
	if err != nil {
		t.Fatal(err)
	}
	forward := ledger.NewSignalLedger()
	srv := NewServer(Dependencies{Ledger: forward, Storage: store})
	srv.SetMethodRegistry(registry)
	srv.SetSelectionEngine(engine, snapshots)
	router := gin.New()
	srv.SetupRoutes(router)
	return srv, router, registry, forward, snapshots
}

func doJSON(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func compileTestMethod(t *testing.T, name string) *methods.CompiledMethod {
	t.Helper()
	pct := 0.1
	compiled, _, err := methods.Compile(&methods.Candidate{
		Name: name, Universe: "universe_usable",
		Entry: map[string]any{"type": "compare",
			"left":  map[string]any{"type": "indicator", "indicator": "close"},
			"right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"},
		HoldingMaxDays: 8, StopLossPct: ptrFloat(-0.05), TakeProfitPct: ptrFloat(0.1),
		PositionMode: "pct_equity", PositionPct: &pct,
	})
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func ptrFloat(v float64) *float64 { return &v }

func registerVerifiedMethod(t *testing.T, registry *methodregistry.Registry) *methodregistry.Method {
	t.Helper()
	compiled := compileTestMethod(t, "强证据测试方法")
	registered, err := registry.Register(t.Context(), methodregistry.Registration{
		FamilyID: "family-strong", VariantID: "v1", Market: "A", Method: compiled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.Status != methodregistry.StatusCandidate {
		t.Fatalf("registration without evidence should be candidate, got %s", registered.Status)
	}
	return registered
}

func registerRejectedMethod(t *testing.T, registry *methodregistry.Registry) *methodregistry.Method {
	t.Helper()
	compiled := compileTestMethod(t, "弱夏普方法")
	bundle := &validation.EvidenceBundle{
		JobHash: "job-hash", MethodHash: compiled.ContentHash, SnapshotID: "snap-1",
		Confidence: validation.ConfidenceWeak, Passable: false,
		ConfidenceReason: "oos_sharpe_below_threshold",
		OosStats:         validation.PerformanceStats{TotalTrades: 20, SharpeRatio: 0.2, MaxDrawdown: 0.1},
	}
	bundle.ResultHash = bundle.ComputeResultHash()
	registered, err := registry.Register(t.Context(), methodregistry.Registration{
		FamilyID: "family-weak", VariantID: "v1", Market: "A", Method: compiled,
		Evidence: methodregistry.ValidationEvidence{Bundle: bundle},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.Status != methodregistry.StatusRejected {
		t.Fatalf("expected rejected registration, got %s", registered.Status)
	}
	return registered
}

func seedFrozenMarket(t *testing.T, snapshots *marketsnapshotrepo.SQLiteRepository) error {
	t.Helper()
	market := &marketsnapshot.MarketSnapshot{ID: "real-snapshot", SnapshotDate: "2024-01-02", Universe: marketsnapshot.UniverseDefinition{Name: "universe_usable"}, Market: "CN-A", PriceAdjustment: "forward", ExpectedKlineCodes: 1, ReadyKlineCodes: 1, CoveragePct: 1, Status: marketsnapshot.StatusReady, Frozen: false, UniverseMembers: []marketsnapshot.UniverseMember{{Code: "000001", Selected: true, Status: "normal"}}, Codes: []marketsnapshot.CodeStatus{{Code: "000001", UniverseMember: true, KlineLastDate: "2024-01-02", KlineRowCount: 260}}}
	market.UniverseHash = marketsnapshot.ComputeUniverseHash(market.UniverseMembers)
	market.ContentHash, _ = marketsnapshot.ComputeContentHash(market)
	if err := snapshots.SaveMarketSnapshot(market); err != nil {
		return err
	}
	if err := snapshots.FreezeMarketSnapshot(market.ID); err != nil {
		return err
	}
	feature := &marketsnapshot.FeatureSnapshot{ID: "real-features", MarketSnapshotID: market.ID, SnapshotDate: market.SnapshotDate, FeatureIDs: []string{"close", "ma20", "amount"}, FeatureTotal: 3, RowsWritten: 3, LeakChecked: true, PriceAdjustment: "forward", Status: marketsnapshot.StatusReady, AsOfNs: time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local).UnixNano(), Values: map[string]map[string]float64{"000001": {"close": 10.5, "ma20": 10, "amount": 100000000}}}
	feature.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(feature)
	return snapshots.SaveFeatureSnapshot(feature)
}

func TestMethodRejectStatsAndFeedbackEndpoints(t *testing.T) {
	_, router, registry, _, _ := newMarketTestServer(t)
	rejected := registerRejectedMethod(t, registry)

	// reject-stats：原因按类别聚合，带机器可读类别。
	rec := doJSON(router, http.MethodGet, "/api/methods/reject-stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reject-stats status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"category":"weak_oos_performance"`) {
		t.Fatalf("reject stats missing category: %s", rec.Body.String())
	}

	// feedback：写审计注释，供研究反哺读取。
	rec = doJSON(router, http.MethodPost, "/api/methods/"+rejected.ID+"/feedback", `{"useful":false,"comment":"信号太频繁"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("feedback status=%d body=%s", rec.Code, rec.Body.String())
	}
	m, err := registry.Get(t.Context(), rejected.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Annotations) != 1 || !strings.HasPrefix(m.Annotations[0].Text, "feedback:useful=false") {
		t.Fatalf("feedback annotation missing: %+v", m.Annotations)
	}
	if m.Annotations[0].Actor != "user-feedback" || !strings.Contains(m.Annotations[0].Text, "信号太频繁") {
		t.Fatalf("feedback annotation wrong: %+v", m.Annotations[0])
	}

	// audit 轨迹里有 annotate 事件。
	rec = doJSON(router, http.MethodGet, "/api/methods/"+rejected.ID+"/audit", "")
	if !strings.Contains(rec.Body.String(), `"action":"annotate"`) {
		t.Fatalf("feedback audit event missing: %s", rec.Body.String())
	}
}

func TestMethodForwardHealthEndpoint(t *testing.T) {
	_, router, registry, forward, _ := newMarketTestServer(t)
	compiled := compileTestMethod(t, "前向健康方法")
	// 前向健康只评估 verified/observing/degraded 方法：给一个可通过门槛的证据包。
	bundle := &validation.EvidenceBundle{
		JobHash: "job-hash-forward", MethodHash: compiled.ContentHash, SnapshotID: "snap-1",
		Confidence: validation.ConfidenceModerate, Passable: true,
		ConfidenceReason: "soft_blocker:test",
		OosStats:         validation.PerformanceStats{TotalTrades: 20, SharpeRatio: 0.8, MaxDrawdown: 0.1},
	}
	bundle.ResultHash = bundle.ComputeResultHash()
	registered, err := registry.Register(t.Context(), methodregistry.Registration{
		FamilyID: "family-forward", VariantID: "v1", Market: "A", Method: compiled,
		Evidence: methodregistry.ValidationEvidence{Bundle: bundle},
	})
	if err != nil {
		t.Fatal(err)
	}
	versionID := registered.Versions[0].ID
	if _, err := forward.NewForwardRun(versionID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 100000, trading.DefaultTradingConstraints(), trading.DefaultCostModel()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		date := time.Date(2026, 1, 2+i, 0, 0, 0, 0, time.UTC)
		if err := forward.AppendSignal(ledger.SignalEntry{
			ID: versionID + "-sig-" + string(rune('a'+i)), RunID: "fr-" + versionID + "-20260101",
			ParadigmVersionID: versionID, StockCode: "600000", Direction: "buy",
			SignalDate: date, Price: 10,
			DataSnapshot: ledger.DataSnapshot{DataHash: "test-data-hash", CapturedAt: date},
			Execution: &ledger.ExecutionRecord{Status: "filled", ExecPrice: 10, ExecQty: 100, PnL: 80, ExecutedAt: date},
		}); err != nil {
			t.Fatal(err)
		}
	}
	rec := doJSON(router, http.MethodGet, "/api/methods/forward-health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("forward-health status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"method_id":"` + registered.ID + `"`, `"forward_samples":3`, `"executed_count":3`} {
		if !strings.Contains(body, want) {
			t.Fatalf("forward-health missing %s in %s", want, body)
		}
	}
}

func TestSelectionRunCreateRequiresMethodIDsAndRunsFiltered(t *testing.T) {
	_, router, registry, _, snapshots := newMarketTestServer(t)
	registered := registerVerifiedMethod(t, registry)

	// 缺 method_ids → 400。
	rec := doJSON(router, http.MethodPost, "/api/selections/run", `{"method_ids":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty method_ids should 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	// 未提供 market_snapshot_id 且无冻结快照 → 409。
	rec = doJSON(router, http.MethodPost, "/api/selections/run", `{"method_ids":["method-ghost"]}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("no frozen snapshot should 409, got %d body=%s", rec.Code, rec.Body.String())
	}

	// 冻结真实快照后：幽灵方法被排除，真实方法触发选股。
	if err := seedFrozenMarket(t, snapshots); err != nil {
		t.Fatal(err)
	}
	rec = doJSON(router, http.MethodPost, "/api/selections/run", `{"method_ids":["`+registered.ID+`","method-ghost"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("selection run status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"requested_method_ids"`) {
		t.Fatalf("run must record requested ids: %s", body)
	}
	if !strings.Contains(body, `"method_not_found"`) {
		t.Fatalf("ghost id must be excluded with method_not_found: %s", body)
	}
}
