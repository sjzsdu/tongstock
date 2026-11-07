package reconcile_test

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
	"github.com/sjzsdu/tongstock/internal/reconcile"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

const testCode = "000001"

// fixture builds one frozen snapshot pair over a synthetic 5-day series so the
// as-of path recomputes exactly what the stored feature snapshot claims.
type fixture struct {
	store     *storage.Storage
	snapshots *marketsnapshotrepo.SQLiteRepository
	methods   *methodregistryrepo.SQLiteRepository
	runs      *selectionrepo.SQLiteRepository
	market    *marketsnapshot.MarketSnapshot
	feature   *marketsnapshot.FeatureSnapshot
}

func newFixture(t *testing.T, featureValues func(bars []methods.Bar) map[string]float64) *fixture {
	t.Helper()
	ctx := context.Background()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "reconcile.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bars := []methods.Bar{
		{Date: "20231229", Open: 10.0, High: 10.2, Low: 9.8, Close: 10.0, Volume: 90, Amount: 900},
		{Date: "20240102", Open: 10.0, High: 10.4, Low: 9.9, Close: 10.0, Volume: 100, Amount: 1000},
		{Date: "20240103", Open: 10.1, High: 10.5, Low: 10.0, Close: 10.2, Volume: 110, Amount: 1122},
		{Date: "20240104", Open: 10.3, High: 10.7, Low: 10.1, Close: 10.6, Volume: 120, Amount: 1272},
		{Date: "20240105", Open: 10.5, High: 10.9, Low: 10.4, Close: 10.8, Volume: 130, Amount: 1404},
	}
	for _, b := range bars {
		if _, err := store.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
			VALUES (?,?,?,?,?,?,?,?,?)`, testCode, 9, b.Date, b.Open, b.High, b.Low, b.Close, b.Volume, b.Amount); err != nil {
			t.Fatalf("insert bar: %v", err)
		}
	}
	if _, err := store.DB().Exec(`INSERT INTO stockinfo(code,name,exchange) VALUES (?,?,?)`,
		testCode, "平安银行", "SZ"); err != nil {
		t.Fatalf("insert stockinfo: %v", err)
	}

	snapshots, err := marketsnapshotrepo.New(store)
	if err != nil {
		t.Fatal(err)
	}
	methodRepo, err := methodregistryrepo.New(store)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := selectionrepo.New(store)
	if err != nil {
		t.Fatal(err)
	}

	market := &marketsnapshot.MarketSnapshot{
		ID: "snap-20240105", SnapshotDate: "2024-01-05",
		Universe:           marketsnapshot.UniverseDefinition{Name: "universe_usable"},
		Market:             "CN-A",
		PriceAdjustment:    "forward",
		ExpectedKlineCodes: 1, ReadyKlineCodes: 1, CoveragePct: 1,
		Status:          marketsnapshot.StatusReady,
		UniverseMembers: []marketsnapshot.UniverseMember{{Code: testCode, Selected: true, Status: "normal"}},
		Codes:           []marketsnapshot.CodeStatus{{Code: testCode, UniverseMember: true, KlineLastDate: "2024-01-05", KlineRowCount: 4}},
	}
	market.UniverseHash = marketsnapshot.ComputeUniverseHash(market.UniverseMembers)
	market.ContentHash, _ = marketsnapshot.ComputeContentHash(market)
	if err := snapshots.SaveMarketSnapshot(market); err != nil {
		t.Fatal(err)
	}
	if err := snapshots.FreezeMarketSnapshot(market.ID); err != nil {
		t.Fatal(err)
	}

	ids := []string{"amount", "close", "ma5"}
	values := map[string]map[string]float64{testCode: featureValues(bars)}
	feature := &marketsnapshot.FeatureSnapshot{
		ID: "fs-20240105", MarketSnapshotID: market.ID, SnapshotDate: "2024-01-05",
		FeatureIDs: ids, FeatureTotal: len(ids), RowsWritten: len(ids),
		LeakChecked: true, PriceAdjustment: "forward", Status: marketsnapshot.StatusReady,
		AsOfNs: time.Now().UTC().UnixNano(), Values: values,
	}
	feature.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(feature)
	if err := snapshots.SaveFeatureSnapshot(feature); err != nil {
		t.Fatal(err)
	}

	pct, stop, take := .08, -.05, .1
	compiled, _, err := methods.Compile(&methods.Candidate{
		Name: "收盘站上5日均线", Universe: "universe_usable",
		Entry: map[string]any{"type": "compare",
			"left":  map[string]any{"type": "indicator", "indicator": "close"},
			"right": map[string]any{"type": "indicator", "indicator": "ma5"},
			"op":    "gt"},
		HoldingMaxDays: 10, StopLossPct: &stop, TakeProfitPct: &take,
		PositionMode: "pct_equity", PositionPct: &pct,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	method := &methodregistry.Method{
		ID: "method-asof", FamilyID: "family-trend", VariantID: "v1", Name: compiled.Name,
		Status: methodregistry.StatusVerified, Market: "A", Universe: "universe_usable",
		HoldingMaxDays: 10, CurrentVersion: 1,
		Versions: []methodregistry.MethodVersion{{
			ID: "method-asof-v1", Version: 1, MethodHash: compiled.ContentHash, Method: compiled,
			Evidence: &methodregistry.EvidenceSummary{
				ResultHash: "evidence-asof", SnapshotID: "validation-asof",
				Confidence: "strong", Passable: true, OOSTrades: 120, OOSMaxDrawdown: -.1,
			},
			CreatedAt: now,
		}},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := methodRepo.Save(ctx, method, methodregistry.AuditEvent{
		ID: "audit-asof", MethodID: method.ID, From: methodregistry.StatusCandidate,
		To: methodregistry.StatusVerified, Action: "policy", Automatic: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	return &fixture{store: store, snapshots: snapshots, methods: methodRepo, runs: runs, market: market, feature: feature}
}

func (f *fixture) deps() reconcile.Deps {
	return reconcile.Deps{
		Store:      f.store,
		Snapshots:  f.snapshots,
		Methods:    f.methods,
		Universe:   marketsnapshotrepo.NewSQLiteUniverseProvider(f.store),
		StoredRuns: f.runs,
	}
}

func builtinValues(names []string) func([]methods.Bar) map[string]float64 {
	return func(bars []methods.Bar) map[string]float64 {
		out := map[string]float64{}
		for _, name := range names {
			if v, ok := methods.ComputeBuiltinIndicator(name, bars); ok {
				out[name] = v
			}
		}
		return out
	}
}

// TestReconcileAgreesWhenTheBoundedStreamMatchesTheSnapshot is the happy-path
// proof: with no data drift, both paths produce the same candidate, action,
// score and content hash, and neither run table is written.
func TestReconcileAgreesWhenTheBoundedStreamMatchesTheSnapshot(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, builtinValues([]string{"amount", "close", "ma5"}))

	report, err := reconcile.Run(ctx, f.deps(), "2024-01-05")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.NoSnapshot {
		t.Fatalf("unexpected no-snapshot report: %s", report.Reason)
	}
	if !report.ContentHashMatch {
		t.Fatalf("content hash mismatch: snapshot=%s as-of=%s", report.SnapshotContentHash, report.AsOfContentHash)
	}
	if !report.Consistent {
		t.Fatalf("reconcile found differences: %v", report.DiffSummary())
	}
	if len(report.SnapshotCandidateCodes) != 1 || len(report.AsOfCandidateCodes) != 1 {
		t.Fatalf("candidate sets = %v / %v, want one code each",
			report.SnapshotCandidateCodes, report.AsOfCandidateCodes)
	}
	if len(report.InputsDigest) != 64 {
		t.Fatalf("inputs digest = %q, want sha256 hex", report.InputsDigest)
	}
	if report.AsOfDataVersion < 1 {
		t.Fatalf("as-of data version = %d", report.AsOfDataVersion)
	}

	var selectionRuns, positionRuns int
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM daily_selection_run`).Scan(&selectionRuns); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM position_decision_run`).Scan(&positionRuns); err != nil {
		t.Fatal(err)
	}
	if selectionRuns != 0 || positionRuns != 0 {
		t.Fatalf("reconcile wrote official results: selection=%d position=%d", selectionRuns, positionRuns)
	}
}

// TestReconcileReportsDriftWhenFeatureValuesDiffer flips one stored feature
// value so the paths disagree; the report must name the code and the content
// hash mismatch instead of claiming consistency.
func TestReconcileReportsDriftWhenFeatureValuesDiffer(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, func(bars []methods.Bar) map[string]float64 {
		values := builtinValues([]string{"amount", "close", "ma5"})(bars)
		values["close"] = 12.34 // what the snapshot recorded, not what the stream says
		return values
	})

	report, err := reconcile.Run(ctx, f.deps(), "2024-01-05")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Consistent {
		t.Fatal("reconcile claimed consistency while the feature values differ")
	}
	if report.ContentHashMatch {
		t.Fatal("content hash matched although the feature values differ")
	}
	if len(report.DiffSummary()) == 0 {
		t.Fatal("differences were detected but not described")
	}
}

// TestReconcileReportsNoSnapshotWithoutInventingOne covers the documented exit
// path: a day with no frozen snapshot must be reported, not fabricated.
func TestReconcileReportsNoSnapshotWithoutInventingOne(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, builtinValues([]string{"amount", "close", "ma5"}))

	report, err := reconcile.Run(ctx, f.deps(), "2024-01-08")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !report.NoSnapshot {
		t.Fatalf("report for a snapshot-less day claims a comparison: %+v", report)
	}
	if report.Reason == "" {
		t.Fatal("no-snapshot report has no reason")
	}
	if !report.Consistent {
		t.Fatal("a snapshot-less day must not be reported as inconsistent")
	}
}
