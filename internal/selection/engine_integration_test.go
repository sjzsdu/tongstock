package selection_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestSelectionPersistsDeterministicTraceableBuy(t *testing.T) {
	ctx := context.Background()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "selection.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	snapshots, _ := marketsnapshotrepo.New(store)
	methodsRepo, _ := methodregistryrepo.New(store)
	runs, _ := selectionrepo.New(store)

	market := &marketsnapshot.MarketSnapshot{ID: "real-snapshot", SnapshotDate: "2024-01-02", Universe: marketsnapshot.UniverseDefinition{Name: "universe_usable"}, Market: "CN-A", PriceAdjustment: "forward", ExpectedKlineCodes: 1, ReadyKlineCodes: 1, CoveragePct: 1, Status: marketsnapshot.StatusReady, Frozen: false, UniverseMembers: []marketsnapshot.UniverseMember{{Code: "000001", Selected: true, Status: "normal"}}, Codes: []marketsnapshot.CodeStatus{{Code: "000001", UniverseMember: true, KlineLastDate: "2024-01-02", KlineRowCount: 260}}}
	market.UniverseHash = marketsnapshot.ComputeUniverseHash(market.UniverseMembers)
	market.ContentHash, _ = marketsnapshot.ComputeContentHash(market)
	if err := snapshots.SaveMarketSnapshot(market); err != nil {
		t.Fatal(err)
	}
	if err := snapshots.FreezeMarketSnapshot(market.ID); err != nil {
		t.Fatal(err)
	}
	feature := &marketsnapshot.FeatureSnapshot{ID: "real-features", MarketSnapshotID: market.ID, SnapshotDate: market.SnapshotDate, FeatureIDs: []string{"close", "ma20", "amount"}, FeatureTotal: 3, RowsWritten: 3, LeakChecked: true, PriceAdjustment: "forward", Status: marketsnapshot.StatusReady, AsOfNs: time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local).UnixNano(), Values: map[string]map[string]float64{"000001": {"close": 10.5, "ma20": 10, "amount": 100000000}}}
	feature.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(feature)
	if err := snapshots.SaveFeatureSnapshot(feature); err != nil {
		t.Fatal(err)
	}
	pct, stop, take := .08, -.05, .1
	compiled, _, err := methods.Compile(&methods.Candidate{Name: "收盘站上20日均线", Universe: "universe_usable", Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"}, HoldingMaxDays: 10, StopLossPct: &stop, TakeProfitPct: &take, PositionMode: "pct_equity", PositionPct: &pct})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	method := &methodregistry.Method{ID: "method-real", FamilyID: "family-trend", VariantID: "v1", Name: compiled.Name, Status: methodregistry.StatusVerified, Market: "A", Universe: "universe_usable", HoldingMaxDays: 10, CurrentVersion: 1, Versions: []methodregistry.MethodVersion{{ID: "method-real-v1", Version: 1, MethodHash: compiled.ContentHash, Method: compiled, Evidence: &methodregistry.EvidenceSummary{ResultHash: "evidence-real", SnapshotID: "validation-real", Confidence: "strong", Passable: true, OOSTrades: 120, OOSMaxDrawdown: -.1}, CreatedAt: now}}, CreatedAt: now, UpdatedAt: now}
	if err := methodsRepo.Save(ctx, method, methodregistry.AuditEvent{ID: "audit-real", MethodID: method.ID, From: methodregistry.StatusCandidate, To: methodregistry.StatusVerified, Action: "policy", Automatic: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	engine, err := selection.NewEngine(snapshots, methodsRepo, runs)
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.Run(ctx, selection.Request{MarketSnapshotID: market.ID, FeatureSnapshotID: feature.ID})
	if err != nil {
		t.Fatal(err)
	}
	if first.BuyCount != 1 || len(first.Candidates) != 1 || first.Candidates[0].Action != selection.ActionBuy {
		t.Fatalf("unexpected run: %+v", first)
	}
	if first.Candidates[0].SnapshotID != market.ID || first.Candidates[0].Triggers[0].MethodVersionID != "method-real-v1" {
		t.Fatal("candidate lost snapshot/method traceability")
	}
	second, err := engine.Run(ctx, selection.Request{MarketSnapshotID: market.ID, FeatureSnapshotID: feature.ID})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("same immutable inputs did not replay idempotently")
	}
}
