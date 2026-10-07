package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodautomation"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
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

func registerRejectedMethodWithReason(t *testing.T, registry *methodregistry.Registry, reason string) *methodregistry.Method {
	t.Helper()
	compiled := compileTestMethod(t, "指定原因的拒绝方法")
	bundle := &validation.EvidenceBundle{
		JobHash: "job-hash-" + reason, MethodHash: compiled.ContentHash, SnapshotID: "snap-1",
		Confidence: validation.ConfidenceWeak, Passable: false,
		ConfidenceReason: reason,
		OosStats:         validation.PerformanceStats{TotalTrades: 20, SharpeRatio: 0.2, MaxDrawdown: 0.1},
	}
	bundle.ResultHash = bundle.ComputeResultHash()
	registered, err := registry.Register(t.Context(), methodregistry.Registration{
		FamilyID: "family-" + reason, VariantID: "v1", Market: "A", Method: compiled,
		Evidence: methodregistry.ValidationEvidence{Bundle: bundle},
	})
	if err != nil {
		t.Fatal(err)
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
			Execution:    &ledger.ExecutionRecord{Status: "filled", ExecPrice: 10, ExecQty: 100, PnL: 80, ExecutedAt: date},
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

func TestMethodResearchStatusReportsIdleWithoutOrchestrator(t *testing.T) {
	_, router, _, _, _ := newMarketTestServer(t)
	rec := doJSON(router, http.MethodGet, "/api/methods/research/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status endpoint should always be 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"running":false`) {
		t.Fatalf("expected running:false, got %s", rec.Body.String())
	}
}

// 从未完成过任何批次时 last 端点返回 200 空结构（而不是 404）——
// 「没有记录」与「正在运行」是两种状态，前端需要区分。
func TestMethodResearchLastReturnsEmptyStructureWithoutCompletedBatch(t *testing.T) {
	_, router, _, _, _ := newMarketTestServer(t)
	rec := doJSON(router, http.MethodGet, "/api/methods/research/last", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("last should be 200 even without completed batch, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"no_completed_batch"`) {
		t.Fatalf("last should mark no_completed_batch: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"running":false`) {
		t.Fatalf("last should report running:false without orchestrator: %s", rec.Body.String())
	}
}

// 拒绝原因统计必须把带随机 hash 的原因码归一为可读类别。
func TestMethodRejectStatsNormalizesReasonsWithoutHashes(t *testing.T) {
	_, router, registry, _, _ := newMarketTestServer(t)
	first := registerRejectedMethodWithReason(t, registry, "hard_blocker:ss-trades-0797f2ae6b8fa808c7d66b8fb1036d66")
	second := registerRejectedMethodWithReason(t, registry, "hard_blocker:ss-trades-c45bda4362fe70b225ae862e7e8cfdd6")

	rec := doJSON(router, http.MethodGet, "/api/methods/reject-stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reject-stats status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "0797f2ae") || strings.Contains(body, "c45bda43") {
		t.Fatalf("reject stats must not leak random hash suffixes: %s", body)
	}
	if !strings.Contains(body, `"reason":"hard_blocker:ss-trades"`) || !strings.Contains(body, `"count":2`) {
		t.Fatalf("same-category reasons must merge into one count: %s", body)
	}
	_ = first
	_ = second
}

// ---------------------------------------------------------------------------
// 自动研究 run 端点：异步启动（一轮可能跑几十分钟，HTTP 不能同步等）
// ---------------------------------------------------------------------------

type stubResearchDiscoverer struct {
	block   chan struct{}
	release sync.Once
}

func (d *stubResearchDiscoverer) Release() { d.release.Do(func() { close(d.block) }) }

func (d *stubResearchDiscoverer) Run(ctx context.Context, _ discovery.Request) (*discovery.Result, error) {
	select {
	case <-d.block:
		return &discovery.Result{ResearchID: "res-async"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type stubResearchSnapshots struct{ snap *paradigm.DatasetSnapshot }

func (s *stubResearchSnapshots) List(int, int) ([]*paradigm.DatasetSnapshot, error) {
	return []*paradigm.DatasetSnapshot{s.snap}, nil
}

func (s *stubResearchSnapshots) GetByID(id string) (*paradigm.DatasetSnapshot, error) {
	if id == s.snap.ID {
		return s.snap, nil
	}
	return nil, fmt.Errorf("snapshot %s not found", id)
}

func (s *stubResearchSnapshots) VerifyContent(string) error { return nil }

type stubResearchUniverse struct{}

func (stubResearchUniverse) ResolveUniverse(context.Context, string, int, int) ([]string, int, error) {
	return []string{"600000"}, 0, nil
}

type stubResearchBars struct{}

func (stubResearchBars) LoadBars(context.Context, string, string, string, string) ([]validation.BacktestBar, error) {
	return nil, errors.New("unused in run-endpoint tests")
}

type stubResearchBenchmark struct{}

func (stubResearchBenchmark) LoadDailyReturns(context.Context, string, string, string, string) (map[string]float64, error) {
	return nil, errors.New("unused in run-endpoint tests")
}

func newResearchTestServer(t *testing.T, d methodautomation.Discoverer) (*Server, *gin.Engine) {
	t.Helper()
	srv, router, registry, _, _ := newMarketTestServer(t)
	orch, err := methodautomation.New(methodautomation.Deps{
		Registry:   registry,
		Snapshots:  &stubResearchSnapshots{snap: &paradigm.DatasetSnapshot{ID: "auto-snap"}},
		Universe:   stubResearchUniverse{},
		Bars:       stubResearchBars{},
		Benchmark:  stubResearchBenchmark{},
		Discoverer: d,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetMethodAutomation(orch)
	return srv, router
}

func TestMethodResearchRunStartsBatchInBackground(t *testing.T) {
	d := &stubResearchDiscoverer{block: make(chan struct{})}
	defer d.Release() // 保证测试退出前释放阻塞的 goroutine
	srv, router := newResearchTestServer(t, d)

	// run 立即返回 200 started，不等待 discovery 完成（显式 snapshot_id 走快路径）。
	rec := doJSON(router, http.MethodPost, "/api/methods/research/run", `{"snapshot_id":"auto-snap"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run should start asynchronously with 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"started":true`) {
		t.Fatalf("run response missing started:true: %s", rec.Body.String())
	}

	// 批次很快进入 running 状态（轮询端点可见）。
	deadline := time.Now().Add(2 * time.Second)
	for {
		rec = doJSON(router, http.MethodGet, "/api/methods/research/status", "")
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), `"running":true`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status never reported running: %s", rec.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 运行中状态必须携带实时进度阶段（前端进度卡片的数据源）。
	rec = doJSON(router, http.MethodGet, "/api/methods/research/status", "")
	if !strings.Contains(rec.Body.String(), `"phase":"discovery"`) {
		t.Fatalf("status should surface discovery phase while batch runs: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"universe_size":1`) {
		t.Fatalf("status progress should carry universe size: %s", rec.Body.String())
	}

	// 已在运行时再次触发 → 409 busy（前端转跟踪模式的依据）。
	rec = doJSON(router, http.MethodPost, "/api/methods/research/run", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second run should 409, got %d body=%s", rec.Code, rec.Body.String())
	}

	d.Release()
	srv.WaitForBackgroundTasks()

	// 批次结束后：status 恢复 idle 且带 last_finished_at；last 端点返回批次结果。
	rec = doJSON(router, http.MethodGet, "/api/methods/research/status", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"running":true`) {
		t.Fatalf("status should be idle after batch, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"last_finished_at"`) {
		t.Fatalf("status missing last_finished_at: %s", rec.Body.String())
	}
	rec = doJSON(router, http.MethodGet, "/api/methods/research/last", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"snapshot_id":"auto-snap"`) {
		t.Fatalf("last should return batch result, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMethodResearchRunRecordsFailureInStatus(t *testing.T) {
	srv, router := newResearchTestServer(t, &stubResearchDiscoverer{block: make(chan struct{})})

	// ghost snapshot 让 Run 在 resolveSnapshot 阶段同步失败
	// （超时取消走同一条 recordResearchResult 路径）。
	rec := doJSON(router, http.MethodPost, "/api/methods/research/run", `{"snapshot_id":"ghost-snap"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run should start asynchronously even when batch will fail, got %d", rec.Code)
	}
	srv.WaitForBackgroundTasks()

	rec = doJSON(router, http.MethodGet, "/api/methods/research/status", "")
	if !strings.Contains(rec.Body.String(), `"last_error":"load frozen snapshot ghost-snap`) {
		t.Fatalf("status should surface last_error after failure: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"running":true`) {
		t.Fatalf("status should be idle after failure: %s", rec.Body.String())
	}
}
