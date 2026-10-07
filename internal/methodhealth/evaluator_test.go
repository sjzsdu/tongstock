package methodhealth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

type fakeLedger struct{ signals []ledger.SignalEntry }

func (f fakeLedger) ListByParadigm(string) []ledger.SignalEntry { return f.signals }

func newRegistryWithMethod(t *testing.T) (*methodregistry.Registry, string) {
	t.Helper()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "health.db")})
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
	pct := 0.1
	compiled, _, err := methods.Compile(&methods.Candidate{
		Name: "健康度测试方法", Universe: "universe_all",
		Entry: map[string]any{"type": "compare",
			"left":  map[string]any{"type": "indicator", "indicator": "close"},
			"right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"},
		HoldingMaxDays: 8, PositionMode: "pct_equity", PositionPct: &pct,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	m := &methodregistry.Method{
		ID: "method-health", FamilyID: "family-health", VariantID: "v1", Name: compiled.Name,
		Status: methodregistry.StatusVerified, Market: "A", Universe: "universe_all",
		HoldingMaxDays: 8, CurrentVersion: 1,
		Versions:  []methodregistry.MethodVersion{{ID: "method-health-v1", Version: 1, MethodHash: compiled.ContentHash, Method: compiled, CreatedAt: now}},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Save(context.Background(), m, methodregistry.AuditEvent{ID: "audit-health", MethodID: m.ID, To: methodregistry.StatusVerified, Action: "policy", Automatic: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return registry, "method-health-v1"
}

func signal(versionID string, i int, status string, pnl float64) ledger.SignalEntry {
	date := time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
	entry := ledger.SignalEntry{
		ID: versionID + "-sig-" + string(rune('a'+i)), RunID: "fr-" + versionID + "-20260101",
		ParadigmVersionID: versionID, StockCode: "600000", Direction: "buy",
		SignalDate: date, Price: 10,
	}
	if status != "" {
		entry.Execution = &ledger.ExecutionRecord{Status: status, ExecPrice: 10, ExecQty: 100, PnL: pnl, ExecutedAt: date}
	}
	return entry
}

func TestEvaluatorHealthyForwardEvidencePromotesToObserving(t *testing.T) {
	registry, versionID := newRegistryWithMethod(t)
	signals := make([]ledger.SignalEntry, 0, 20)
	for i := 0; i < 20; i++ {
		signals = append(signals, signal(versionID, i, "filled", 100)) // 10% 每笔
	}
	evaluator, err := New(registry, fakeLedger{signals: signals}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := evaluator.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 method health item, got %d", len(items))
	}
	item := items[0]
	if item.ForwardSamples != 20 || item.ExecutedCount != 20 {
		t.Fatalf("samples not aggregated: %+v", item)
	}
	if item.HitRate == nil || *item.HitRate != 1.0 {
		t.Fatalf("hit rate must come from real executions: %+v", item.HitRate)
	}
	if item.Status != string(methodregistry.StatusObserving) {
		t.Fatalf("20 healthy forward samples should promote to observing, got %q (score=%.1f)", item.Status, item.Score)
	}
}

func TestEvaluatorConsecutiveSevereLossesRetireMethod(t *testing.T) {
	registry, versionID := newRegistryWithMethod(t)
	signals := []ledger.SignalEntry{
		signal(versionID, 0, "filled", 200),
		signal(versionID, 1, "filled", -150), // -15%
		signal(versionID, 2, "filled", -150),
		signal(versionID, 3, "filled", -150),
	}
	evaluator, err := New(registry, fakeLedger{signals: signals}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := evaluator.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if items[0].ConsecutiveSevere != 3 {
		t.Fatalf("expected 3 consecutive severe losses, got %d", items[0].ConsecutiveSevere)
	}
	if items[0].Status != string(methodregistry.StatusRetired) {
		t.Fatalf("policy should retire the method, got %q", items[0].Status)
	}
}

func TestEvaluatorFlagsExecutionDeviationWithoutPolicyWrite(t *testing.T) {
	registry, versionID := newRegistryWithMethod(t)
	signals := []ledger.SignalEntry{
		signal(versionID, 0, "rejected", 0),
		signal(versionID, 1, "rejected", 0),
		signal(versionID, 2, "rejected", 0),
		signal(versionID, 3, "rejected", 0),
		signal(versionID, 4, "rejected", 0),
		signal(versionID, 5, "filled", 50),
		signal(versionID, 6, "filled", 60),
		signal(versionID, 7, "filled", 70),
	}
	// applyPolicy=false：只读评估，不触发状态转移。
	evaluator, err := New(registry, fakeLedger{signals: signals}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := evaluator.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]
	if !item.ExecutionDeviation {
		t.Fatalf("5/8 rejected executions must flag deviation: %+v", item)
	}
	if item.Status != string(methodregistry.StatusVerified) {
		t.Fatalf("read-only evaluation must not change status, got %q", item.Status)
	}
	if item.HitRate == nil || *item.HitRate != 1.0 {
		t.Fatalf("hit rate computed from executed fills only: %+v", item.HitRate)
	}
}

func TestEvaluatorWithoutForwardEvidenceKeepsNilMetrics(t *testing.T) {
	registry, _ := newRegistryWithMethod(t)
	evaluator, err := New(registry, fakeLedger{}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := evaluator.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ForwardSamples != 0 || item.HitRate != nil || item.AvgReturn != nil {
		t.Fatalf("missing forward evidence must stay nil, got %+v", item)
	}
	if item.Status != string(methodregistry.StatusVerified) {
		t.Fatalf("no forward evidence must keep status, got %q", item.Status)
	}
}
