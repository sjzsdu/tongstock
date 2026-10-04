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

func TestSelectionRunFiltersByRequestedMethodIDs(t *testing.T) {
	ctx := context.Background()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "selection-filter.db")})
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
	compiledA, _, err := methods.Compile(&methods.Candidate{Name: "方法A 收盘站上20日均线", Universe: "universe_usable", Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"}, HoldingMaxDays: 10, StopLossPct: &stop, TakeProfitPct: &take, PositionMode: "pct_equity", PositionPct: &pct})
	if err != nil {
		t.Fatal(err)
	}
	compiledB, _, err := methods.Compile(&methods.Candidate{Name: "方法B 收盘跌破20日均线", Universe: "universe_usable", Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "lt"}, HoldingMaxDays: 10, StopLossPct: &stop, TakeProfitPct: &take, PositionMode: "pct_equity", PositionPct: &pct})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	save := func(id, family string, compiled *methods.CompiledMethod) *methodregistry.Method {
		m := &methodregistry.Method{ID: id, FamilyID: family, VariantID: "v1", Name: compiled.Name, Status: methodregistry.StatusVerified, Market: "A", Universe: "universe_usable", HoldingMaxDays: 10, CurrentVersion: 1, Versions: []methodregistry.MethodVersion{{ID: id + "-v1", Version: 1, MethodHash: compiled.ContentHash, Method: compiled, Evidence: &methodregistry.EvidenceSummary{ResultHash: "evidence-" + id, SnapshotID: "validation-real", Confidence: "strong", Passable: true, OOSTrades: 120, OOSMaxDrawdown: -.1}, CreatedAt: now}}, CreatedAt: now, UpdatedAt: now}
		if err := methodsRepo.Save(ctx, m, methodregistry.AuditEvent{ID: "audit-" + id, MethodID: id, From: methodregistry.StatusCandidate, To: methodregistry.StatusVerified, Action: "policy", Automatic: true, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		return m
	}
	methodA := save("method-A", "family-a", compiledA)
	methodB := save("method-B", "family-b", compiledB)

	engine, err := selection.NewEngine(snapshots, methodsRepo, runs)
	if err != nil {
		t.Fatal(err)
	}

	// 1. 全量运行仍然照旧（不传 method_ids）。
	full, err := engine.Run(ctx, selection.Request{MarketSnapshotID: market.ID, FeatureSnapshotID: feature.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.RequestedMethodIDs) != 0 {
		t.Fatalf("full run must not record requested ids: %+v", full.RequestedMethodIDs)
	}

	// 2. 只跑方法 A：B 不出现在触发里。
	onlyA, err := engine.Run(ctx, selection.Request{MarketSnapshotID: market.ID, FeatureSnapshotID: feature.ID, MethodIDs: []string{methodA.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyA.RequestedMethodIDs) != 1 || onlyA.RequestedMethodIDs[0] != methodA.ID {
		t.Fatalf("requested ids not recorded: %+v", onlyA.RequestedMethodIDs)
	}
	for _, candidate := range onlyA.Candidates {
		for _, trigger := range candidate.Triggers {
			if trigger.MethodID == methodB.ID {
				t.Fatal("unrequested method triggered a candidate")
			}
			if trigger.MethodID != methodA.ID {
				t.Fatalf("unexpected method trigger: %+v", trigger)
			}
		}
	}
	if onlyA.ID == full.ID {
		t.Fatal("filtered run must not collide with the full run hash")
	}

	// 3. 未知 ID 记入排除，且不产生结果污染。
	unknownRun, err := engine.Run(ctx, selection.Request{MarketSnapshotID: market.ID, FeatureSnapshotID: feature.ID, MethodIDs: []string{"method-ghost", methodA.ID}})
	if err != nil {
		t.Fatal(err)
	}
	foundGhost := false
	for _, exclusion := range unknownRun.Exclusions {
		if exclusion.MethodID == "method-ghost" && exclusion.ReasonCode == "method_not_found" {
			foundGhost = true
		}
	}
	if !foundGhost {
		t.Fatalf("unknown requested id must be excluded: %+v", unknownRun.Exclusions)
	}
	if unknownRun.ID == onlyA.ID {
		t.Fatal("different requested sets must produce different runs")
	}

	// 4. 相同请求幂等重放。
	replay, err := engine.Run(ctx, selection.Request{MarketSnapshotID: market.ID, FeatureSnapshotID: feature.ID, MethodIDs: []string{methodA.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != onlyA.ID || !replay.CreatedAt.Equal(onlyA.CreatedAt) {
		t.Fatal("same requested methods did not replay idempotently")
	}
}
