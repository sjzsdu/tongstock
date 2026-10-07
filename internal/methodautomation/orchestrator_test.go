package methodautomation_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/methodautomation"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

type fakeDiscoverer struct {
	mu      sync.Mutex
	results []*discovery.Result
	calls   int
	block   chan struct{}
}

func (f *fakeDiscoverer) Run(ctx context.Context, req discovery.Request) (*discovery.Result, error) {
	f.mu.Lock()
	if f.block != nil {
		f.mu.Unlock()
		<-f.block
		f.mu.Lock()
	}
	if f.calls >= len(f.results) {
		f.mu.Unlock()
		return nil, errors.New("no more fake discovery results")
	}
	res := f.results[f.calls]
	f.calls++
	f.mu.Unlock()
	return res, nil
}

type fakeBars struct{}

func (fakeBars) LoadBars(_ context.Context, _, code, start, end string) ([]validation.BacktestBar, error) {
	all := syntheticBars(code)
	out := make([]validation.BacktestBar, 0, len(all))
	for _, b := range all {
		if (start == "" || b.Date >= start) && (end == "" || b.Date <= end) {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no bars in range")
	}
	return out, nil
}

func syntheticBars(code string) []validation.BacktestBar {
	board := backtest.BoardForCode(code)
	bars := make([]validation.BacktestBar, 0, 260)
	date := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
	var prevClose float64
	for i := 0; i < 260; i++ {
		for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			date = date.AddDate(0, 0, 1)
		}
		close := 10 + 0.02*float64(i) + 0.4*sin(float64(i)*0.35)
		bar := validation.BacktestBar{
			Code: code, Date: date.Format("2006-01-02"),
			Open: close - 0.02, High: close + 0.05, Low: close - 0.05, Close: close,
			Volume: 1_000_000, Amount: 10_000_000, Board: board, PreClose: prevClose,
		}
		bars = append(bars, bar)
		prevClose = close
		date = date.AddDate(0, 0, 1)
	}
	return bars
}

func sin(x float64) float64 {
	// 简单确定性周期近似，避免浮点平台差异担忧。
	x = x - float64(int(x/(6.283185307)))*6.283185307
	term, sum := x, x
	for n := 3; n < 15; n += 2 {
		term *= -x * x / float64(n*(n-1))
		sum += term
	}
	return sum
}

type fakeBenchmark struct{}

func (fakeBenchmark) LoadDailyReturns(_ context.Context, _, code, start, end string) (map[string]float64, error) {
	bars := syntheticBars(code)
	out := map[string]float64{}
	for i := 1; i < len(bars); i++ {
		if (start == "" || bars[i].Date >= start) && (end == "" || bars[i].Date <= end) && bars[i-1].Close > 0 {
			out[bars[i].Date] = (bars[i].Close - bars[i-1].Close) / bars[i-1].Close
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no benchmark returns")
	}
	return out, nil
}

type fakeSnapshots struct{ snaps []*paradigm.DatasetSnapshot }

func (f fakeSnapshots) List(int, int) ([]*paradigm.DatasetSnapshot, error) { return f.snaps, nil }
func (f fakeSnapshots) GetByID(id string) (*paradigm.DatasetSnapshot, error) {
	for _, s := range f.snaps {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, errors.New("not found")
}
func (fakeSnapshots) VerifyContent(string) error { return nil }

// pagedSnapshots 按 created_at 倒序真实分页（snaps[0] 最新），用于模拟线上
// 「快照表被单票快照灌满」的分布，验证 resolveSnapshot 会分页扫到多股票快照。
type pagedSnapshots struct{ snaps []*paradigm.DatasetSnapshot }

func (f pagedSnapshots) List(limit, offset int) ([]*paradigm.DatasetSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset >= len(f.snaps) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.snaps) {
		end = len(f.snaps)
	}
	return f.snaps[offset:end], nil
}
func (f pagedSnapshots) GetByID(id string) (*paradigm.DatasetSnapshot, error) {
	for _, s := range f.snaps {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, errors.New("not found")
}
func (pagedSnapshots) VerifyContent(string) error { return nil }

type fakeUniverse struct{ codes []string }

func (f fakeUniverse) ResolveUniverse(context.Context, string, int, int) ([]string, int, error) {
	return f.codes, 0, nil
}

type fakeEvidence struct {
	mu      sync.Mutex
	bundles []*validation.EvidenceBundle
}

func (f *fakeEvidence) Save(_ context.Context, bundle *validation.EvidenceBundle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bundles = append(f.bundles, bundle)
	return nil
}

type fakeTraces struct{ saved int }

func (f *fakeTraces) Save(context.Context, *discovery.Result) error { f.saved++; return nil }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type harness struct {
	orch     *methodautomation.Orchestrator
	registry *methodregistry.Registry
	evidence *fakeEvidence
	traces   *fakeTraces
}

func newHarness(t *testing.T, discoverer methodautomation.Discoverer) *harness {
	t.Helper()
	return newHarnessWithSnapshots(t, discoverer, fakeSnapshots{snaps: []*paradigm.DatasetSnapshot{{ID: "snap-1", Universe: []string{"600000", "600001", "600002", "600003", "600004", "600005"}}}})
}

func newHarnessWithSnapshots(t *testing.T, discoverer methodautomation.Discoverer, snaps methodautomation.SnapshotStore) *harness {
	t.Helper()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "registry.db")})
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
	evidence := &fakeEvidence{}
	h := &harness{registry: registry, evidence: evidence, traces: &fakeTraces{}}
	orch, err := methodautomation.New(methodautomation.Deps{
		Registry:  registry,
		Snapshots: snaps,
		Universe:  fakeUniverse{codes: []string{"600000", "600001", "600002", "600003", "600004", "600005"}},
		Bars:      fakeBars{}, Benchmark: fakeBenchmark{},
		Evidence: evidence, Discoverer: discoverer, Traces: h.traces,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.orch = orch
	return h
}

func compileBreakout(t *testing.T, name string) *methods.CompiledMethod {
	t.Helper()
	pct := 0.1
	compiled, diags, err := methods.Compile(&methods.Candidate{
		Name: name, Universe: "researched_stocks",
		Entry: map[string]any{"type": "compare",
			"left":  map[string]any{"type": "indicator", "indicator": "close"},
			"right": map[string]any{"type": "indicator", "indicator": "ma20"},
			"op":    "gt"},
		HoldingMaxDays: 5, PositionMode: "pct_equity", PositionPct: &pct,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !compiled.IsExecutable() {
		t.Fatalf("compiled method not executable: %v", diags)
	}
	return compiled
}

func discoveryResult(t *testing.T, researchID string, hold int, compiled *methods.CompiledMethod) *discovery.Result {
	t.Helper()
	res := &discovery.Result{
		ResearchID: researchID, SnapshotID: "snap-1", GeneratorVersion: discovery.GeneratorVersion,
		GeneratedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), HoldDays: hold,
		DiscoveryTrials: 9, Conclusion: "ranked_hypotheses",
		Boundaries: []discovery.CodeBoundary{
			{Code: "600000", ReservedStartDate: "2023-06-01", LastDate: "2023-12-29"},
			{Code: "600001", ReservedStartDate: "2023-06-01", LastDate: "2023-12-29"},
		},
		Rejected: []discovery.RejectedCandidate{{TemplateID: "weak_template", Reason: "observations 3 < required 12", Observations: 3}},
	}
	res.Candidates = append(res.Candidates, discovery.CandidateEvidence{
		TemplateID: "close_above_ma20", Method: compiled, Observations: 60,
		Rationale: "检验趋势持续", TStatistic: 2.0,
	})
	res.ResultHash = res.ComputeHash()
	return res
}

func candidateOutcome(t *testing.T, result *methodautomation.BatchResult) *methodautomation.CandidateOutcome {
	t.Helper()
	for i := range result.Outcomes {
		if result.Outcomes[i].TemplateID == "close_above_ma20" {
			return &result.Outcomes[i]
		}
	}
	t.Fatalf("missing candidate outcome: %+v", result.Outcomes)
	return nil
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestOrchestratorFullPipelineRegistersByEvidence(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	h := newHarness(t, &fakeDiscoverer{results: []*discovery.Result{discoveryResult(t, "research-a", 5, compiled)}})

	result, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	if result.SnapshotID != "snap-1" || result.UniverseSize != 6 {
		t.Fatalf("unexpected batch meta: %+v", result)
	}
	if result.TrialsThisBatch != 9 || result.TrialsCumulative != 9 {
		t.Fatalf("discovery trials not accounted: %+v", result)
	}
	if len(result.Batches) != 1 || result.Batches[0].DiscoveryTrials != 9 {
		t.Fatalf("unexpected batches: %+v", result.Batches)
	}
	// 1 个候选结局 + 1 个 discovery 阶段拒绝的模板。
	if len(result.Outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %+v", result.Outcomes)
	}
	out := candidateOutcome(t, result)
	if out.MethodID == "" {
		t.Fatalf("candidate outcome not registered: %+v", out)
	}
	if out.Status != "verified" && out.Status != "rejected" {
		t.Fatalf("status must come from the machine policy, got %q", out.Status)
	}
	if len(h.evidence.bundles) != 1 {
		t.Fatalf("expected 1 persisted evidence bundle, got %d", len(h.evidence.bundles))
	}
	bundle := h.evidence.bundles[0]
	if bundle.JobHash == "" || bundle.ResultHash == "" {
		t.Fatal("evidence bundle missing immutable hashes")
	}
	if out.Status == "rejected" && bundle.ConfidenceReason == "" {
		t.Fatal("rejected outcome must carry a machine-readable confidence reason")
	}
	if result.Outcomes[1].Stage != "discovery" || result.Outcomes[1].Reason == "" {
		t.Fatalf("discovery-level reject not recorded: %+v", result.Outcomes[1])
	}
	if result.Registered != 1 {
		t.Fatalf("registered counter wrong: %+v", result)
	}
	if h.traces.saved != 1 {
		t.Fatalf("research trace not persisted: saved=%d", h.traces.saved)
	}
	// 方法库中确实登记，且状态与结局一致。
	cards, err := h.registry.Cards(context.Background(), methodregistry.Query{FamilyID: "auto-close_above_ma20"})
	if err != nil || len(cards) != 1 {
		t.Fatalf("registered method missing: %v", err)
	}
	if string(cards[0].Status) != out.Status {
		t.Fatalf("registry status %q != outcome status %q", cards[0].Status, out.Status)
	}
}

func TestOrchestratorIsIdempotentForSameMethodHash(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	h := newHarness(t, &fakeDiscoverer{results: []*discovery.Result{
		discoveryResult(t, "research-a", 5, compiled),
		discoveryResult(t, "research-a", 5, compiled), // 相同方法哈希再跑一遍
	}})

	if _, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}}); err != nil {
		t.Fatal(err)
	}
	result, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	out := candidateOutcome(t, result)
	// 只有第一轮证据 passable 时才允许跳过；被拒的方法必须重新验证。
	cards, err := h.registry.Cards(context.Background(), methodregistry.Query{FamilyID: "auto-close_above_ma20"})
	if err != nil || len(cards) == 0 {
		t.Fatalf("cards unavailable: %v", err)
	}
	skipAllowed := false
	for _, card := range cards {
		if card.VariantID == compiled.ContentHash && card.Evidence != nil && card.Evidence.Passable {
			skipAllowed = true
		}
	}
	if skipAllowed {
		if out.Status != "skipped_registered" {
			t.Fatalf("passable evidence should be skipped, got %+v", out)
		}
		return
	}
	if out.Status == "skipped_registered" {
		t.Fatalf("non-passable evidence must not block re-validation, got %+v", out)
	}
}

// 历史版本被旧门槛（如已降级的 bl-underperform 硬拒绝）误杀成 rejected 后，
// 同哈希候选不得被幂等闸门永久挡在门外：必须重新验证并追加版本，
// 由 policy.Initial 按当前标准重算状态。而已有 passable 证据的方法保持跳过，
// 避免重复验证浪费预算。
func TestOrchestratorRevalidatesRejectedMethodInsteadOfSkipping(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	h := newHarness(t, &fakeDiscoverer{results: []*discovery.Result{
		discoveryResult(t, "research-a", 5, compiled),
		discoveryResult(t, "research-b", 5, compiled), // 同哈希第二轮
	}})

	ctx := context.Background()
	first, err := h.orch.Run(ctx, methodautomation.Request{HoldDays: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	firstOut := candidateOutcome(t, first)
	if firstOut.MethodID == "" {
		t.Fatalf("first run must register the candidate, got %+v", firstOut)
	}
	cards, err := h.registry.Cards(ctx, methodregistry.Query{FamilyID: "auto-close_above_ma20"})
	if err != nil || len(cards) == 0 {
		t.Fatalf("cards unavailable: %v", err)
	}
	var card *methodregistry.Card
	for i := range cards {
		if cards[i].VariantID == compiled.ContentHash {
			card = &cards[i]
		}
	}
	if card == nil {
		t.Fatalf("registered card missing for hash %s", firstOut.MethodHash)
	}

	second, err := h.orch.Run(ctx, methodautomation.Request{HoldDays: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	secondOut := candidateOutcome(t, second)
	if card.Evidence != nil && card.Evidence.Passable {
		if secondOut.Status != "skipped_registered" {
			t.Fatalf("passable evidence must keep the idempotent skip, got %+v", secondOut)
		}
		return
	}
	// 旧证据不可用：重新走完整验证，Register 追加版本并重算状态。
	if secondOut.Status == "skipped_registered" {
		t.Fatalf("rejected/non-passable evidence must not block re-validation, got %+v", secondOut)
	}
	if secondOut.MethodID == "" || secondOut.MethodID != card.ID {
		t.Fatalf("re-validation must register into the same method, got %+v", secondOut)
	}
	reloaded, err := h.registry.Get(ctx, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CurrentVersion != 2 {
		t.Fatalf("expected version 2 after re-validation, got %d", reloaded.CurrentVersion)
	}
	if string(reloaded.Status) != secondOut.Status {
		t.Fatalf("registry status %q must match outcome %q", reloaded.Status, secondOut.Status)
	}
	events, err := h.registry.Audit(ctx, card.ID)
	if err != nil || len(events) < 2 {
		t.Fatalf("audit trail must record both registrations: %v", err)
	}
	if events[0].Action != "register" || events[1].Action != "register" {
		t.Fatalf("expected two register events, got %+v", events)
	}
}

func TestOrchestratorAccumulatesDiscoveryTrialsAcrossBatches(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	other := compileBreakout(t, "AI发现候选: 收盘站上20日均线 v2")
	h := newHarness(t, &fakeDiscoverer{results: []*discovery.Result{
		discoveryResult(t, "research-h5", 5, compiled),
		discoveryResult(t, "research-h20", 20, other),
	}})

	result, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5, 20}})
	if err != nil {
		t.Fatal(err)
	}
	if result.TrialsThisBatch != 18 || result.TrialsCumulative != 18 {
		t.Fatalf("trials must accumulate across hold-day batches: %+v", result)
	}
	if len(h.evidence.bundles) != 2 {
		t.Fatalf("expected 2 validation runs, got %d", len(h.evidence.bundles))
	}
	// 第二个批次的验证任务必须带上累计后的试验次数。
	firstTrials, secondTrials := h.evidence.bundles[0].DiscoveryTrials, h.evidence.bundles[1].DiscoveryTrials
	if firstTrials != 9 || secondTrials != 18 {
		t.Fatalf("validation jobs did not receive cumulative trials: %d then %d", firstTrials, secondTrials)
	}
}

func TestOrchestratorHonorsNegativeUserFeedback(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	other := compileBreakout(t, "AI发现候选: 收盘站上20日均线 v2")
	h := newHarness(t, &fakeDiscoverer{results: []*discovery.Result{
		discoveryResult(t, "research-a", 5, compiled),
		discoveryResult(t, "research-b", 5, other), // 同族不同哈希
	}})

	if _, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}}); err != nil {
		t.Fatal(err)
	}
	cards, err := h.registry.Cards(context.Background(), methodregistry.Query{FamilyID: "auto-close_above_ma20", Limit: 10})
	if err != nil || len(cards) == 0 {
		t.Fatalf("cards unavailable: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := h.registry.Annotate(context.Background(), cards[0].ID, "feedback:useful=false; 信号太频繁", "user-feedback"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	if err != nil {
		t.Fatal(err)
	}
	out := candidateOutcome(t, result)
	if out.Status != "skipped_feedback" {
		t.Fatalf("negative feedback should skip the family, got %+v", out)
	}
	if !strings.Contains(out.Reason, "user feedback") {
		t.Fatalf("skip reason should mention user feedback: %+v", out)
	}
}

func TestOrchestratorRejectsConcurrentRuns(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	block := make(chan struct{})
	discoverer := &fakeDiscoverer{results: []*discovery.Result{discoveryResult(t, "research-a", 5, compiled)}, block: block}
	h := newHarness(t, discoverer)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	}()
	// 等 discoverer 进入阻塞态后再触发并发。
	time.Sleep(50 * time.Millisecond)
	if _, err := h.orch.Run(context.Background(), methodautomation.Request{}); !errors.Is(err, methodautomation.ErrBusy) {
		t.Fatalf("concurrent run should be rejected with ErrBusy, got %v", err)
	}
	close(block)
	<-done
}

// 运行状态必须对外可见：启动时调度器会立刻跑一轮，HTTP 层要用 Running()
// 展示「研究进行中」，否则用户点「启动研究」只会撞上单飞互斥的 ErrBusy。
func TestOrchestratorExposesRunningState(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	block := make(chan struct{})
	discoverer := &fakeDiscoverer{results: []*discovery.Result{discoveryResult(t, "research-a", 5, compiled)}, block: block}
	h := newHarness(t, discoverer)

	if running, _ := h.orch.Running(); running {
		t.Fatal("no batch should be running before Run")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	}()
	time.Sleep(50 * time.Millisecond)
	running, since := h.orch.Running()
	if !running {
		t.Fatal("Running should be true while the batch is in flight")
	}
	if since.IsZero() {
		t.Fatal("runningSince should be recorded when a batch starts")
	}
	close(block)
	<-done
	if running, _ := h.orch.Running(); running {
		t.Fatal("Running should be false after the batch completes")
	}
}

// 线上踩过的分布：范式分析/单股研究产生的大量单票快照把早期的多股票快照
// 挤出「最近 20 条」窗口，resolveSnapshot 只扫前 20 条就误报没有可用快照。
// 必须分页扫到多股票快照为止（这里把它放在第 61 位，跨越 50 条的分页边界）。
func TestOrchestratorFindsMultiCodeSnapshotBeyondSingleCodeFlood(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	discoverer := &fakeDiscoverer{results: []*discovery.Result{discoveryResult(t, "research-a", 5, compiled)}}

	snaps := make([]*paradigm.DatasetSnapshot, 0, 61)
	for i := 0; i < 60; i++ {
		snaps = append(snaps, &paradigm.DatasetSnapshot{ID: fmt.Sprintf("single-%02d", i), Universe: []string{"600000"}})
	}
	snaps = append(snaps, &paradigm.DatasetSnapshot{ID: "multi-old", Universe: []string{"600000", "600001", "600002", "600003", "600004", "600005"}})

	h := newHarnessWithSnapshots(t, discoverer, pagedSnapshots{snaps: snaps})
	res, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	if err != nil {
		t.Fatalf("pipeline should run against the multi-code snapshot beyond the flood, got %v", err)
	}
	if res.SnapshotID != "multi-old" {
		t.Fatalf("expected snapshot multi-old to be selected, got %q", res.SnapshotID)
	}
	if res.Registered == 0 {
		t.Fatalf("expected at least one registered outcome, got %+v", res.Outcomes)
	}
}

// 全部是单票快照时保持 fail-closed，但错误要给出扫描诊断（扫了多少、多少单票）。
// 运行中的批次必须能被观察到实时进度：阶段从 preparing 推进到 discovery，
// 携带持有期与股票池大小；批次结束后进度不可读（空闲状态没有"当前进度"）。
func TestOrchestratorExposesRealtimeProgress(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	block := make(chan struct{})
	discoverer := &fakeDiscoverer{results: []*discovery.Result{discoveryResult(t, "research-a", 5, compiled)}, block: block}
	h := newHarness(t, discoverer)

	if _, ok := h.orch.RunningProgress(); ok {
		t.Fatal("progress must be unreadable while idle")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	}()
	time.Sleep(50 * time.Millisecond)

	progress, ok := h.orch.RunningProgress()
	if !ok {
		t.Fatal("progress must be readable while the batch is in flight")
	}
	if progress.Phase != methodautomation.PhaseDiscovery {
		t.Fatalf("expected discovery phase while discoverer is blocked, got %s", progress.Phase)
	}
	if progress.HoldDays != 5 || progress.UniverseSize != 6 {
		t.Fatalf("progress should carry hold/universe, got %+v", progress)
	}
	close(block)
	<-done

	if _, ok := h.orch.RunningProgress(); ok {
		t.Fatal("progress must be unreadable after the batch finishes")
	}
}

// 全部是单票快照时保持 fail-closed，但错误要给出扫描诊断（扫了多少、多少单票）。
func TestOrchestratorFailsClosedWithoutMultiCodeSnapshot(t *testing.T) {
	compiled := compileBreakout(t, "AI发现候选: 收盘站上20日均线")
	discoverer := &fakeDiscoverer{results: []*discovery.Result{discoveryResult(t, "research-a", 5, compiled)}}

	snaps := make([]*paradigm.DatasetSnapshot, 0, 25)
	for i := 0; i < 25; i++ {
		snaps = append(snaps, &paradigm.DatasetSnapshot{ID: fmt.Sprintf("single-%02d", i), Universe: []string{"600000"}})
	}

	h := newHarnessWithSnapshots(t, discoverer, pagedSnapshots{snaps: snaps})
	_, err := h.orch.Run(context.Background(), methodautomation.Request{HoldDays: []int{5}})
	if err == nil {
		t.Fatal("expected fail-closed error without any multi-code snapshot")
	}
	if !strings.Contains(err.Error(), "no usable frozen dataset snapshot") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "scanned 25 snapshots") || !strings.Contains(err.Error(), "25 with universe < 5") {
		t.Fatalf("error should carry scan diagnostics, got: %v", err)
	}
}
