package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/factorlab"
	"github.com/sjzsdu/tongstock/internal/methodautomation"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
)

// ---------------------------------------------------------------------------
// fakes：与 internal/factorlab 测试同形，但放在 server 包内（test-only 不能跨包复用）
// ---------------------------------------------------------------------------

type factorSnapStore struct{ snaps []*paradigm.DatasetSnapshot }

func (f factorSnapStore) List(limit, offset int) ([]*paradigm.DatasetSnapshot, error) {
	if offset >= len(f.snaps) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.snaps) {
		end = len(f.snaps)
	}
	return f.snaps[offset:end], nil
}
func (f factorSnapStore) GetByID(id string) (*paradigm.DatasetSnapshot, error) {
	for _, s := range f.snaps {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, errors.New("snapshot not found")
}
func (factorSnapStore) VerifyContent(string) error { return nil }

type factorUniverseStub struct{ codes []string }

func (f factorUniverseStub) ResolveUniverse(context.Context, string, int, int) ([]string, int, error) {
	return f.codes, 0, nil
}

type factorBarsStub struct {
	byCode map[string][]validation.BacktestBar
}

func (p factorBarsStub) LoadBars(_ context.Context, _, code, _, _ string) ([]validation.BacktestBar, error) {
	if bars, ok := p.byCode[code]; ok {
		return bars, nil
	}
	return nil, errors.New("no bars")
}

// factorTestBars 生成相位错开的「趋势-反转」横截面（与 internal/factorlab
// 测试里的 crossSectionalBars 同构）：每只股票经历 20 天匀速上涨 + 20 天
// 匀速下跌，相位按代码错开 7 天。这样每个截面日的因子值与前向收益都带
// 真实离散度（RankIC 可产出），且因子方向由数据决定而非硬编码。
func factorTestBars(codes []string, days int) map[string][]validation.BacktestBar {
	byCode := make(map[string][]validation.BacktestBar, len(codes))
	date := func(i int) string {
		return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
	}
	const period = 40
	for ci, code := range codes {
		price := 10.0
		for i := 0; i < days; i++ {
			phase := (i + 7*ci) % period
			ret := -0.005
			if phase < period/2 {
				ret = 0.005
			}
			price *= 1 + ret
			byCode[code] = append(byCode[code], validation.BacktestBar{
				Code: code, Date: date(i),
				Open: price, High: price * 1.01, Low: price * 0.99, Close: price,
				Volume: 1_000_000, Amount: 10_000_000,
			})
		}
	}
	return byCode
}

func newFactorTestServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	srv, router, _, _, _ := newMarketTestServer(t)
	installFactorLab(t, srv)
	return srv, router
}

func installFactorLab(t *testing.T, srv *Server) {
	t.Helper()
	codes := []string{"600000", "600001", "600002", "600003", "600004", "600005"}
	svc, err := factorlab.New(methodautomation.Deps{
		Snapshots: factorSnapStore{snaps: []*paradigm.DatasetSnapshot{
			{ID: "factor-snap", Universe: codes, DateRange: paradigm.DateRange{Start: "2024-01-01", End: "2024-02-09"}},
		}},
		Universe: factorUniverseStub{codes: codes},
		Bars:     factorBarsStub{byCode: factorTestBars(codes, 40)},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetFactorLab(svc)
	srv.SetFactorPicks(newSQLitePickStore(t, srv))
}

// newSQLitePickStore 用夹具自己的存储建真实的因子通道仓库。
func newSQLitePickStore(t *testing.T, srv *Server) factorlab.PickStore {
	t.Helper()
	db := srv.storage.DB()
	store, err := factorlab.NewSQLitePickStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// awaitFactorRun 轮询 last 端点直到批次完成（异步启动的测试等待器）。
func awaitFactorRun(t *testing.T, router *gin.Engine) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		rec := doJSON(router, http.MethodGet, "/api/factors/research/last", "")
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), `"running":false`) && strings.Contains(rec.Body.String(), `"result":{`) {
			return rec.Body.String()
		}
		if time.Now().After(deadline) {
			t.Fatalf("factor run did not finish in time; last=%s", rec.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func containsJSONCode(body, needle string) bool {
	return strings.Contains(body, needle)
}

// 泄露 aid：直接查存储里是否已有因子通道产出（避免只依赖 HTTP 状态机时序）。
func pickRowCount(t *testing.T, srv *Server) int {
	t.Helper()
	var n int
	if err := srv.storage.DB().QueryRow(`SELECT COUNT(*) FROM factor_pick_run WHERE as_of='2024-02-09'`).Scan(&n); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0
		}
		t.Fatalf("count factor pick rows: %v", err)
	}
	return n
}

func TestFactorResearchRunUnavailableWithoutService(t *testing.T) {
	_, router, _, _, _ := newMarketTestServer(t)

	rec := doJSON(router, http.MethodPost, "/api/factors/research/run", `{"snapshot_id":"factor-snap"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("run without factorlab should 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !containsJSONCode(rec.Body.String(), "factor_research_unavailable") {
		t.Fatalf("503 should carry factor_research_unavailable code: %s", rec.Body.String())
	}
	rec = doJSON(router, http.MethodGet, "/api/factors/research/last", "")
	if rec.Code != http.StatusOK || !containsJSONCode(rec.Body.String(), "no_completed_run") {
		t.Fatalf("last without factorlab should stay 200 no_completed_run, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(router, http.MethodGet, "/api/factors/picks/last", "")
	if rec.Code != http.StatusOK || !containsJSONCode(rec.Body.String(), "no_pick_run") {
		t.Fatalf("picks without store should stay 200 no_pick_run, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFactorResearchRunProducesResultsAndLastFollows(t *testing.T) {
	_, router := newFactorTestServer(t)

	// 先 last：尚无完成结果 → 200 no_completed_run（前端据此渲染空态）。
	rec := doJSON(router, http.MethodGet, "/api/factors/research/last", "")
	if rec.Code != http.StatusOK || !containsJSONCode(rec.Body.String(), "no_completed_run") {
		t.Fatalf("initial last should be no_completed_run, got %d body=%s", rec.Code, rec.Body.String())
	}

	// run 异步启动：立即返回 started 包封，不带结果（全市场快照一轮分钟级）。
	rec = doJSON(router, http.MethodPost, "/api/factors/research/run", `{"snapshot_id":"factor-snap"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run should start asynchronously with 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !containsJSONCode(rec.Body.String(), `"started":true`) {
		t.Fatalf("run response missing started:true: %s", rec.Body.String())
	}

	// 轮询 last 等批次完成：结果嵌在 result 字段里，评估真实数据方向。
	body := awaitFactorRun(t, router)
	for _, want := range []string{`"engine_version":"factorlab-v1"`, `"snapshot_id":"factor-snap"`, `"factors":`, `"snapshot_date_end":"2024-02-09"`} {
		if !containsJSONCode(body, want) {
			t.Fatalf("last result missing %s: %s", want, body)
		}
	}
}

func TestFactorResearchRunHonorsTopKOption(t *testing.T) {
	_, router := newFactorTestServer(t)

	rec := doJSON(router, http.MethodPost, "/api/factors/research/run", `{"snapshot_id":"factor-snap","top_k":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run with top_k should 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := awaitFactorRun(t, router)
	var parsed struct {
		Result struct {
			TopPicks []json.RawMessage `json:"top_picks"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Result.TopPicks) != 2 {
		t.Fatalf("top_k=2 should yield exactly 2 picks, got %d: %s", len(parsed.Result.TopPicks), body)
	}
}

func TestFactorResearchRunBadBodyIs400(t *testing.T) {
	_, router := newFactorTestServer(t)

	rec := doJSON(router, http.MethodPost, "/api/factors/research/run", `{invalid`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body should 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !containsJSONCode(rec.Body.String(), "invalid_request") {
		t.Fatalf("400 should carry invalid_request code: %s", rec.Body.String())
	}
}

func TestFactorResearchRunWritesSelectionRunWhenRequested(t *testing.T) {
	srv, router := newFactorTestServer(t)

	// 先无产出：picks 端点 no_pick_run。
	rec := doJSON(router, http.MethodGet, "/api/factors/picks/last", "")
	if rec.Code != http.StatusOK || !containsJSONCode(rec.Body.String(), "no_pick_run") {
		t.Fatalf("picks should start no_pick_run, got %d body=%s", rec.Code, rec.Body.String())
	}

	// 要求落库的 run：完成后 picks 端点产出 TopN（截面日 2024-02-09）。
	before := pickRowCount(t, srv)
	if before != 0 {
		t.Fatalf("no pick rows expected before first run, got %d", before)
	}
	rec = doJSON(router, http.MethodPost, "/api/factors/research/run", `{"snapshot_id":"factor-snap","write_to_selection":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run should start with 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	awaitFactorRun(t, router)
	rec = doJSON(router, http.MethodGet, "/api/factors/picks/last", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("picks should 200 after run, got %d body=%s", rec.Code, rec.Body.String())
	}
	picksBody := rec.Body.String()
	for _, want := range []string{`"run_id":"pick-2024-02-09"`, `"as_of":"2024-02-09"`, `"picks":[`, `"factors_snapshot":[`, `"stale_days":`} {
		if !containsJSONCode(picksBody, want) {
			t.Fatalf("picks payload missing %s: %s", want, picksBody)
		}
	}
	if n := pickRowCount(t, srv); n != 1 {
		t.Fatalf("exactly one pick row expected, got %d", n)
	}

	// 同截面日重跑（显式 snapshot）：幂等更新同一行，不新增。
	rec = doJSON(router, http.MethodPost, "/api/factors/research/run", `{"snapshot_id":"factor-snap","write_to_selection":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rerun should 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	awaitFactorRun(t, router)
	if n := pickRowCount(t, srv); n != 1 {
		t.Fatalf("same-snapshot rerun must stay idempotent, got %d rows", n)
	}
}
