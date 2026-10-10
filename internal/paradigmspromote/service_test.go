package paradigmspromote_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/paradigmspromote"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// ---------------------------------------------------------------------------
// fakes（与 methodautomation/orchestrator_test.go 同风格）
// ---------------------------------------------------------------------------

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
	bars := make([]validation.BacktestBar, 0, 300)
	date := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
	var prevClose float64
	for i := 0; i < 300; i++ {
		for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			date = date.AddDate(0, 0, 1)
		}
		close := 10 + 0.02*float64(i) + 0.4*sinSeries(float64(i)*0.35)
		bars = append(bars, validation.BacktestBar{
			Code: code, Date: date.Format("2006-01-02"),
			Open: close - 0.02, High: close + 0.05, Low: close - 0.05, Close: close,
			Volume: 1_000_000, Amount: 10_000_000, Board: board, PreClose: prevClose,
		})
		prevClose = close
		date = date.AddDate(0, 0, 1)
	}
	return bars
}

func sinSeries(x float64) float64 {
	x = x - float64(int(x/6.283185307))*6.283185307
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

var testCodes = []string{"600000", "600001", "600002", "600003", "600004", "600005"}

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

type fakeUniverse struct{}

func (fakeUniverse) ResolveUniverse(context.Context, string, int, int) ([]string, int, error) {
	return testCodes, 0, nil
}

type fakeEvidence struct {
	bundles []*validation.EvidenceBundle
}

func (f *fakeEvidence) Save(_ context.Context, bundle *validation.EvidenceBundle) error {
	f.bundles = append(f.bundles, bundle)
	return nil
}

// ---------------------------------------------------------------------------
// harness
// ---------------------------------------------------------------------------

type harness struct {
	svc      *paradigmspromote.Service
	registry *methodregistry.Registry
	store    *paradigms.Store
	evidence *fakeEvidence
}

func newHarness(t *testing.T, trials func() int64) *harness {
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
	paradigmStore, err := paradigms.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	evidence := &fakeEvidence{}
	svc, err := paradigmspromote.New(paradigmspromote.Deps{
		Paradigms: paradigmStore,
		Registry:  registry,
		Snapshots: fakeSnapshots{snaps: []*paradigm.DatasetSnapshot{{
			ID: "snap-1", Universe: testCodes,
			DateRange: paradigm.DateRange{Start: "2023-01-02", End: "2023-12-31"},
		}}},
		Universe: fakeUniverse{},
		Bars:     fakeBars{}, Benchmark: fakeBenchmark{},
		Evidence: evidence, Trials: trials,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{svc: svc, registry: registry, store: paradigmStore, evidence: evidence}
}

func saveParadigm(t *testing.T, h *harness, p *paradigms.Paradigm) {
	t.Helper()
	if err := h.store.Save(p); err != nil {
		t.Fatal(err)
	}
}

func buyParadigm(id string, conds ...paradigms.Condition) *paradigms.Paradigm {
	return &paradigms.Paradigm{
		ID:       id,
		Name:     "均线多头测试范式",
		Side:     "buy",
		BuyConds: conds,
		Expectation: paradigms.Expectation{
			HoldingPeriod: "5-10天",
			Confidence:    0,
		},
		Rationale: "收盘价站上 MA20 时入场",
	}
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestPromote_BlockedOnCrossCondition(t *testing.T) {
	h := newHarness(t, nil)
	p := buyParadigm("p-cross", paradigms.Condition{Indicator: "close", Operator: "cross_above", Value: "MA20"})
	saveParadigm(t, h, p)

	out, err := h.svc.Promote(context.Background(), p.ID, paradigmspromote.Options{})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if out.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", out.Status)
	}
	if len(out.Blockers) == 0 {
		t.Fatal("blockers must be reported")
	}
	// 状态不写：ReviewStatus 保持原样，但 blockers 必须落进证据卡供前端展示。
	updated, err := h.store.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ReviewStatus != "" {
		t.Errorf("review status = %q, want unchanged", updated.ReviewStatus)
	}
	if updated.Evidence == nil || updated.Evidence.Eligible || len(updated.Evidence.MustFix) == 0 {
		t.Errorf("promotion blockers must be persisted on paradigm, got %+v", updated.Evidence)
	}
	// 方法库不得产生方法。
	cards, err := h.registry.Cards(context.Background(), methodregistry.Query{FamilyID: "paradigm-" + p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 0 {
		t.Fatalf("no method should be registered for blocked paradigm, got %d", len(cards))
	}
}

func TestPromote_SellSideBlocked(t *testing.T) {
	h := newHarness(t, nil)
	p := buyParadigm("p-sell", paradigms.Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	p.Side = "sell"
	saveParadigm(t, h, p)

	out, err := h.svc.Promote(context.Background(), p.ID, paradigmspromote.Options{})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if out.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", out.Status)
	}
	if !contains(out.Blockers, "buy") {
		t.Errorf("blockers should mention side restriction, got %v", out.Blockers)
	}
}

func TestPromote_EndToEndValidationAndWriteBack(t *testing.T) {
	h := newHarness(t, nil)
	p := buyParadigm("p-e2e",
		paradigms.Condition{Indicator: "close", Operator: "gt", Value: "MA20"},
		paradigms.Condition{Indicator: "rsi14", Operator: "between", Value: "10-80"},
	)
	p.SellConds = paradigms.SellConditions{
		TakeProfit: []paradigms.Condition{{Indicator: "close", Operator: "gt", Value: "10%"}},
	}
	saveParadigm(t, h, p)

	out, err := h.svc.Promote(context.Background(), p.ID, paradigmspromote.Options{SnapshotID: "snap-1"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if out.Status != "promoted" && out.Status != "rejected" {
		t.Fatalf("status = %q, want promoted or rejected (machine verdict)", out.Status)
	}
	if out.MethodID == "" {
		t.Fatal("method id must be written back on any registered outcome")
	}
	if len(h.evidence.bundles) != 1 {
		t.Fatalf("evidence bundles saved = %d, want 1", len(h.evidence.bundles))
	}

	updated, err := h.store.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := h.registry.Get(context.Background(), out.MethodID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status == "promoted" {
		if m.Status != methodregistry.StatusVerified {
			t.Fatalf("promoted outcome requires verified method, got %s", m.Status)
		}
		if updated.ReviewStatus != paradigms.StatePromoted {
			t.Errorf("paradigm review status = %q, want promoted", updated.ReviewStatus)
		}
		if updated.MethodID != out.MethodID {
			t.Errorf("paradigm method id = %q, want %q", updated.MethodID, out.MethodID)
		}
		if updated.Evidence == nil || !updated.Evidence.Eligible {
			t.Errorf("paradigm evidence should be eligible after promotion, got %+v", updated.Evidence)
		}
	} else {
		if m.Status == methodregistry.StatusVerified {
			t.Fatalf("rejected outcome must not yield a verified method")
		}
		if updated.ReviewStatus != paradigms.StateRejected {
			t.Errorf("paradigm review status = %q, want rejected", updated.ReviewStatus)
		}
		if updated.ReviewNote == "" {
			t.Error("rejected paradigm must carry a human-readable reason")
		}
	}
	if len(updated.Transitions) != 1 {
		t.Fatalf("transitions = %d, want 1 audit record", len(updated.Transitions))
	}
	wantAction := "promote"
	if out.Status == "rejected" {
		wantAction = "reject"
	}
	if updated.Transitions[0].Action != wantAction {
		t.Errorf("transition action = %q, want %q", updated.Transitions[0].Action, wantAction)
	}
	if updated.Transitions[0].From != "" || updated.Transitions[0].To != updated.ReviewStatus {
		t.Errorf("transition %q->%q does not match review status", updated.Transitions[0].From, updated.Transitions[0].To)
	}
}

func TestPromote_IdempotentWhenAlreadyRegisteredPassable(t *testing.T) {
	h := newHarness(t, nil)
	p := buyParadigm("p-idem", paradigms.Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	saveParadigm(t, h, p)

	first, err := h.svc.Promote(context.Background(), p.ID, paradigmspromote.Options{})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if first.Status != "promoted" {
		t.Skipf("first run did not pass the gate (%s); idempotency only applies to passable evidence", first.Status)
	}
	bundles := len(h.evidence.bundles)
	second, err := h.svc.Promote(context.Background(), p.ID, paradigmspromote.Options{})
	if err != nil {
		t.Fatalf("second promote: %v", err)
	}
	if second.Status != "promoted" {
		t.Fatalf("second status = %q, want promoted", second.Status)
	}
	if len(h.evidence.bundles) != bundles {
		t.Errorf("passable evidence must skip revalidation: bundles %d -> %d", bundles, len(h.evidence.bundles))
	}
}

func TestPromote_TrialsWiredIntoMultipleTestingCorrection(t *testing.T) {
	h := newHarness(t, func() int64 { return 42 })
	p := buyParadigm("p-trials", paradigms.Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	saveParadigm(t, h, p)

	out, err := h.svc.Promote(context.Background(), p.ID, paradigmspromote.Options{})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if out.Status != "promoted" && out.Status != "rejected" {
		t.Fatalf("status = %q, want a machine verdict", out.Status)
	}
	if len(h.evidence.bundles) != 1 {
		t.Fatalf("bundles = %d, want 1", len(h.evidence.bundles))
	}
	if h.evidence.bundles[0].DiscoveryTrials != 42 {
		t.Errorf("discovery trials = %d, want 42 (global counter wired)", h.evidence.bundles[0].DiscoveryTrials)
	}
}

func TestPromote_AutoSnapshotResolution(t *testing.T) {
	h := newHarness(t, nil)
	p := buyParadigm("p-auto", paradigms.Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	saveParadigm(t, h, p)

	out, err := h.svc.Promote(context.Background(), p.ID, paradigmspromote.Options{})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if out.SnapshotID != "snap-1" {
		t.Errorf("auto snapshot = %q, want snap-1", out.SnapshotID)
	}
}

func TestPromote_UnknownParadigmFails(t *testing.T) {
	h := newHarness(t, nil)
	if _, err := h.svc.Promote(context.Background(), "missing", paradigmspromote.Options{}); err == nil {
		t.Fatal("expected error for unknown paradigm")
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if len(v) >= len(want) && (v == want || len(v) > 0 && stringContains(v, want)) {
			return true
		}
	}
	return false
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
