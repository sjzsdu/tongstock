package methodautomation_test

import (
	"context"
	"errors"
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
		Snapshots: fakeSnapshots{snaps: []*paradigm.DatasetSnapshot{{ID: "snap-1", Universe: []string{"600000", "600001", "600002", "600003", "600004", "600005"}}}},
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
	if out.Status != "skipped_registered" {
		t.Fatalf("same method hash should be skipped, got %+v", out)
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
