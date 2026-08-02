package methodregistryrepo_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestRegistryPolicyPersistenceFamilyAuditAndRestart(t *testing.T) {
	ctx := context.Background()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "registry.db")})
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
	method := compiled(t, "单股方法", "single:600519")
	bundle := evidence(method, "600519", true, validation.ConfidenceModerate)
	first, err := registry.Register(ctx, methodregistry.Registration{FamilyID: "family-policy", VariantID: "variant-a", Market: "A", EntrySummary: "收盘价高于20日均线", ExitSummary: "收盘价跌破20日均线", Method: method, Evidence: methodregistry.ValidationEvidence{Bundle: bundle}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != methodregistry.StatusVerified || first.Versions[0].Evidence.ResultHash != bundle.ResultHash {
		t.Fatalf("method not machine verified: %+v", first)
	}
	badMethod := compiled(t, "失败变体", "single:600519")
	badEvidence := evidence(badMethod, "600519", false, validation.ConfidenceRejected)
	failed, err := registry.Register(ctx, methodregistry.Registration{FamilyID: "family-policy", VariantID: "variant-b", Market: "A", Method: badMethod, Evidence: methodregistry.ValidationEvidence{Bundle: badEvidence}})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != methodregistry.StatusRejected {
		t.Fatalf("failed variant status=%s", failed.Status)
	}
	cards, err := registry.Cards(ctx, methodregistry.Query{FamilyID: "family-policy"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("family variants=%d", len(cards))
	}
	if _, err := registry.ManualTransition(ctx, first.ID, methodregistry.StatusVerified, "human says good", "tester"); err == nil {
		t.Fatal("human forged verified status")
	}
	health := methodregistry.HealthState{Score: 48, ForwardSamples: 30, Drift: true, EvidenceHash: "forward-health-hash", AsOf: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)}
	degraded, err := registry.ApplyHealth(ctx, first.ID, health)
	if err != nil {
		t.Fatal(err)
	}
	if degraded.Status != methodregistry.StatusDegraded {
		t.Fatalf("health status=%s", degraded.Status)
	}
	restarted, err := methodregistry.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Get(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != methodregistry.StatusDegraded || loaded.Health == nil {
		t.Fatalf("restart lost state: %+v", loaded)
	}
	audit, err := restarted.Audit(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 {
		t.Fatalf("audit events=%d", len(audit))
	}
}

func compiled(t *testing.T, name, universe string) *methods.CompiledMethod {
	t.Helper()
	pct := 0.1
	c := &methods.Candidate{Name: name, Universe: universe, Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"}, Exit: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "lt"}, PositionMode: "pct_equity", PositionPct: &pct, HoldingMaxDays: 10}
	m, _, err := methods.Compile(c)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsExecutable() {
		t.Fatalf("compiled method not executable: %+v", m.Diagnostics)
	}
	return m
}
func evidence(m *methods.CompiledMethod, code string, pass bool, confidence validation.ConfidenceLevel) *validation.EvidenceBundle {
	b := &validation.EvidenceBundle{JobHash: "validation-job", MethodHash: m.ContentHash, MethodName: m.Name, SnapshotID: "immutable-snapshot", StockCode: code, GeneratedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), OosStats: validation.PerformanceStats{TotalTrades: 32}, Confidence: confidence, Passable: pass}
	b.ResultHash = b.ComputeResultHash()
	return b
}
