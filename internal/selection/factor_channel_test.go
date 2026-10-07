package selection_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/factorlab"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// factorPickStub 是可变桩：测试中途换名单/报错，观察通道行为。
type factorPickStub struct {
	pick *factorlab.PickRun
	err  error
}

func (s *factorPickStub) GetLatestPickRun(context.Context) (*factorlab.PickRun, error) {
	return s.pick, s.err
}

// newFactorChannelEngine 建一套真实仓库 + 一个已晋级的形态方法 + 因子桩，
// 返回可直接 Run 的引擎与 market/feature ID。
func newFactorChannelEngine(t *testing.T) (*selection.Engine, *factorPickStub, string, string) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "factor-channel.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	snapshots, _ := marketsnapshotrepo.New(store)
	methodsRepo, _ := methodregistryrepo.New(store)
	runs, _ := selectionrepo.New(store)

	market := &marketsnapshot.MarketSnapshot{ID: "fc-snapshot", SnapshotDate: "2024-01-02", Universe: marketsnapshot.UniverseDefinition{Name: "universe_usable"}, Market: "CN-A", PriceAdjustment: "forward", ExpectedKlineCodes: 1, ReadyKlineCodes: 1, CoveragePct: 1, Status: marketsnapshot.StatusReady, Frozen: false, UniverseMembers: []marketsnapshot.UniverseMember{{Code: "000001", Selected: true, Status: "normal"}}, Codes: []marketsnapshot.CodeStatus{{Code: "000001", UniverseMember: true, KlineLastDate: "2024-01-02", KlineRowCount: 260}}}
	market.UniverseHash = marketsnapshot.ComputeUniverseHash(market.UniverseMembers)
	market.ContentHash, _ = marketsnapshot.ComputeContentHash(market)
	if err := snapshots.SaveMarketSnapshot(market); err != nil {
		t.Fatal(err)
	}
	if err := snapshots.FreezeMarketSnapshot(market.ID); err != nil {
		t.Fatal(err)
	}
	feature := &marketsnapshot.FeatureSnapshot{ID: "fc-features", MarketSnapshotID: market.ID, SnapshotDate: market.SnapshotDate, FeatureIDs: []string{"close", "ma20", "amount"}, FeatureTotal: 3, RowsWritten: 3, LeakChecked: true, PriceAdjustment: "forward", Status: marketsnapshot.StatusReady, AsOfNs: time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local).UnixNano(), Values: map[string]map[string]float64{"000001": {"close": 10.5, "ma20": 10, "amount": 100000000}}}
	feature.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(feature)
	if err := snapshots.SaveFeatureSnapshot(feature); err != nil {
		t.Fatal(err)
	}
	stop, take := -.05, .1
	pct := .08
	compiled, _, err := methods.Compile(&methods.Candidate{Name: "收盘站上20日均线", Universe: "universe_usable", Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"}, HoldingMaxDays: 10, StopLossPct: &stop, TakeProfitPct: &take, PositionMode: "pct_equity", PositionPct: &pct})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	method := &methodregistry.Method{ID: "method-fc", FamilyID: "family-fc", VariantID: "v1", Name: compiled.Name, Status: methodregistry.StatusVerified, Market: "A", Universe: "universe_usable", HoldingMaxDays: 10, CurrentVersion: 1, Versions: []methodregistry.MethodVersion{{ID: "method-fc-v1", Version: 1, MethodHash: compiled.ContentHash, Method: compiled, Evidence: &methodregistry.EvidenceSummary{ResultHash: "ev-fc", SnapshotID: "validation-fc", Confidence: "strong", Passable: true, OOSTrades: 120, OOSMaxDrawdown: -.1}, CreatedAt: now}}, CreatedAt: now, UpdatedAt: now}
	if err := methodsRepo.Save(ctx, method, methodregistry.AuditEvent{ID: "audit-fc", MethodID: method.ID, From: methodregistry.StatusCandidate, To: methodregistry.StatusVerified, Action: "policy", Automatic: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	engine, err := selection.NewEngine(snapshots, methodsRepo, runs)
	if err != nil {
		t.Fatal(err)
	}
	stub := &factorPickStub{}
	engine.SetFactorPicks(stub)
	return engine, stub, market.ID, feature.ID
}

func freshFactorPick(updatedAt time.Time) *factorlab.PickRun {
	return &factorlab.PickRun{
		RunID: "pick-2024-02-09", SnapshotID: "factor-snap", SnapshotDateEnd: "2024-02-09",
		AsOf: "2024-02-09", StaleDays: 5, UpdatedAt: updatedAt.UnixMilli(),
		Note: "6 个因子显著",
		Picks: []factorlab.TopPick{
			{Code: "600001", Score: 0.8, Contributions: map[string]float64{"momentum_5d": 0.5, "volatility_20d": 0.3}},
			{Code: "600002", Score: 0.6, Contributions: map[string]float64{"momentum_5d": 0.6}},
		},
	}
}

func TestFactorChannelAppendsFreshPicksAsWatch(t *testing.T) {
	engine, stub, marketID, featureID := newFactorChannelEngine(t)
	stub.pick = freshFactorPick(time.Now())

	run, err := engine.Run(context.Background(), selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID})
	if err != nil {
		t.Fatal(err)
	}
	var factorCands []selection.Candidate
	for _, c := range run.Candidates {
		if c.Triggers[0].MethodID == selection.FactorMethodID {
			factorCands = append(factorCands, c)
		}
	}
	if len(factorCands) != 2 {
		t.Fatalf("fresh picks should become 2 factor candidates, got %d: %+v", len(factorCands), run.Candidates)
	}
	for _, c := range factorCands {
		if c.Action != selection.ActionWatch {
			t.Fatalf("factor candidates must be watch-only, got %s", c.Action)
		}
		if c.Exit.Complete {
			t.Fatal("factor candidates must not carry a complete exit plan")
		}
		facts := c.Triggers[0].Facts
		if len(facts) < 4 { // score + run_id + as_of + >=1 contribution
			t.Fatalf("factor facts missing decomposition: %+v", facts)
		}
		if facts[1].Path != "factor.run_id" || facts[1].Detail != "pick-2024-02-09" {
			t.Fatalf("run_id fact missing: %+v", facts)
		}
		if facts[2].Path != "factor.as_of" || facts[2].Detail != "2024-02-09（距今 5 天）" {
			t.Fatalf("as_of fact missing: %+v", facts)
		}
		if c.Explanation == "" {
			t.Fatal("factor candidate must explain why")
		}
	}
	// 形态方法候选（close>ma20 触发 buy）与因子候选并存。
	if run.BuyCount != 1 {
		t.Fatalf("method candidate should still be buy, got %d", run.BuyCount)
	}
}

func TestFactorChannelStalePickBecomesExclusion(t *testing.T) {
	engine, stub, marketID, featureID := newFactorChannelEngine(t)
	pick := freshFactorPick(time.Now())
	pick.StaleDays = 166 // 实测场景：4-24 快照距今 5 个月
	stub.pick = pick

	run, err := engine.Run(context.Background(), selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range run.Candidates {
		if c.Triggers[0].MethodID == selection.FactorMethodID {
			t.Fatalf("stale picks must not become candidates: %+v", c)
		}
	}
	found := false
	for _, ex := range run.Exclusions {
		if ex.MethodID == selection.FactorMethodID && ex.ReasonCode == "factor_pick_stale" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale pick must be an honest exclusion, got %+v", run.Exclusions)
	}
}

func TestFactorChannelUnavailableIsExclusionNotFailure(t *testing.T) {
	engine, stub, marketID, featureID := newFactorChannelEngine(t)
	stub.err = errors.New("db locked")

	run, err := engine.Run(context.Background(), selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID})
	if err != nil {
		t.Fatalf("factor source error must not fail the whole run: %v", err)
	}
	found := false
	for _, ex := range run.Exclusions {
		if ex.MethodID == selection.FactorMethodID && ex.ReasonCode == "factor_pick_unavailable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unavailable factor source must be recorded, got %+v", run.Exclusions)
	}
}

func TestFactorChannelRespectsExplicitMethodSelection(t *testing.T) {
	engine, stub, marketID, featureID := newFactorChannelEngine(t)
	stub.pick = freshFactorPick(time.Now())

	run, err := engine.Run(context.Background(), selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID, MethodIDs: []string{"method-fc"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range run.Candidates {
		if c.Triggers[0].MethodID == selection.FactorMethodID {
			t.Fatal("explicit selection without factor-combo must not include factor candidates")
		}
	}
	// 点名 factor-combo 则启用（与其他方法并列）。
	run, err = engine.Run(context.Background(), selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID, MethodIDs: []string{"method-fc", selection.FactorMethodID}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range run.Candidates {
		if c.Triggers[0].MethodID == selection.FactorMethodID {
			found = true
		}
	}
	if !found {
		t.Fatal("explicit factor-combo request should include factor candidates")
	}
}

func TestFactorChannelPickIdentityJoinsRunHash(t *testing.T) {
	engine, stub, marketID, featureID := newFactorChannelEngine(t)
	ctx := context.Background()
	stub.pick = freshFactorPick(time.Date(2024, 3, 1, 8, 0, 0, 0, time.UTC))

	first, err := engine.Run(ctx, selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID})
	if err != nil {
		t.Fatal(err)
	}
	// 同一名单重跑：幂等重放同一 run。
	replay, err := engine.Run(ctx, selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != first.ID {
		t.Fatal("same pick run must replay idempotently")
	}
	// 名单更新（UpdatedAt 变化）：必须产出新 run，不能重放旧缓存。
	stub.pick = freshFactorPick(time.Date(2024, 3, 2, 8, 0, 0, 0, time.UTC))
	second, err := engine.Run(ctx, selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("updated pick run must produce a new selection run")
	}
}
