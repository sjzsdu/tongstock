package reconcile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/asof"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/runinput"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// Deps are the ports a reconcile run needs. Every field is a read-only view of
// production state; the run never saves snapshots or runs.
type Deps struct {
	Store      *storage.Storage
	Snapshots  *marketsnapshotrepo.SQLiteRepository
	Methods    selection.MethodRepository
	Universe   marketsnapshot.UniverseProvider
	StoredRuns selection.Repository
}

// Run executes both read paths for tradeDate (YYYY-MM-DD) and reports their
// differences. A nil error with Report.NoSnapshot means the day has no ready
// frozen snapshot to reconcile against, which is not a failure.
func Run(ctx context.Context, deps Deps, tradeDate string) (Report, error) {
	if deps.Store == nil || deps.Store.DB() == nil {
		return Report{}, errors.New("reconcile requires storage")
	}
	if deps.Snapshots == nil {
		return Report{}, errors.New("reconcile requires the snapshot repository")
	}
	if deps.Methods == nil || deps.Universe == nil {
		return Report{}, errors.New("reconcile requires a method repository and universe provider")
	}
	date, err := normalizeDate(tradeDate)
	if err != nil {
		return Report{}, err
	}

	market, feature, reason, err := loadFrozenSnapshot(deps.Snapshots, date)
	if err != nil {
		return Report{}, err
	}
	if market == nil {
		return Report{
			TradeDate:  date,
			NoSnapshot: true,
			Reason:     reason,
			Consistent: true,
			Notes:      []string{reason},
		}, nil
	}

	// Old path: the frozen snapshot pair, recomputed in memory so both paths
	// run the current engine and only their input provenance differs.
	oldRun, err := runOnSnapshots(ctx, deps.Methods, &memorySnapshots{
		market:   market,
		features: map[string]*marketsnapshot.FeatureSnapshot{feature.ID: feature},
	}, market.ID, feature.ID)
	if err != nil {
		return Report{}, fmt.Errorf("recompute snapshot path: %w", err)
	}

	// New path: the same question answered from the bounded K-line stream.
	view, err := asof.Open(ctx, deps.Store.DB(), date)
	if err != nil {
		return Report{}, fmt.Errorf("open as-of view for %s: %w", date, err)
	}
	asOf, err := BuildAsOf(ctx, view, market, deps.Universe, feature.FeatureIDs, feature.Values)
	if err != nil {
		return Report{}, err
	}
	newRun, err := runOnSnapshots(ctx, deps.Methods, &memorySnapshots{
		market:   asOf.Market,
		features: map[string]*marketsnapshot.FeatureSnapshot{asOf.Feature.ID: asOf.Feature},
	}, asOf.Market.ID, asOf.Feature.ID)
	if err != nil {
		return Report{}, fmt.Errorf("run as-of path: %w", err)
	}

	report := Compare(oldRun, newRun, feature.ContentHash, asOf.Feature.ContentHash)
	report.TradeDate = date
	report.SnapshotID = market.ID
	report.FeatureSnapshotID = feature.ID
	report.SnapshotPath = "frozen snapshot pair, recomputed in memory (no writes)"
	report.AsOfPath = "as-of view over kline_latest bounded by date <= " + date
	report.AsOfDataVersion = view.DataVersion
	report.AsOfCoverage = asOf.Coverage
	report.DroppedCodes = asOf.Dropped

	digest, err := digestFor(ctx, deps.Store, date, feature.FeatureIDs)
	if err != nil {
		return Report{}, err
	}
	report.InputsDigest = digest

	if deps.StoredRuns != nil {
		if note := storedRunNote(ctx, deps.StoredRuns, date, market.ID, feature.ID); note != "" {
			report.Notes = append(report.Notes, note)
		}
	}
	return report, nil
}

// loadFrozenSnapshot finds the ready frozen snapshot for date plus the feature
// snapshot that a selection run would have used. A nil market with a reason
// means there is nothing to reconcile.
func loadFrozenSnapshot(snapshots *marketsnapshotrepo.SQLiteRepository, date string) (*marketsnapshot.MarketSnapshot, *marketsnapshot.FeatureSnapshot, string, error) {
	items, err := snapshots.ListMarketSnapshots(date, date, marketsnapshot.StatusReady)
	if err != nil {
		return nil, nil, "", err
	}
	marketID := ""
	for _, item := range items {
		if item.SnapshotDate == date && item.Frozen {
			marketID = item.ID
			break
		}
	}
	if marketID == "" {
		return nil, nil, fmt.Sprintf("无快照可对账：%s 没有 frozen + ready 的 market snapshot", date), nil
	}
	market, err := snapshots.LoadMarketSnapshot(marketID, true)
	if err != nil {
		return nil, nil, "", err
	}
	features, err := snapshots.ListFeatureSnapshots(marketID)
	if err != nil {
		return nil, nil, "", err
	}
	// Prefer the widest ready, leak-checked feature snapshot: that is what the
	// selection engine would pick for a fresh run.
	var chosen *marketsnapshot.FeatureSnapshot
	for _, f := range features {
		if f.Status != marketsnapshot.StatusReady || !f.LeakChecked {
			continue
		}
		if chosen == nil || len(f.FeatureIDs) > len(chosen.FeatureIDs) {
			chosen = f
		}
	}
	if chosen == nil {
		return nil, nil, fmt.Sprintf("无快照可对账：%s 的快照 %s 没有 ready 的 feature snapshot", date, marketID), nil
	}
	loaded, err := snapshots.LoadFeatureSnapshot(chosen.ID, true)
	if err != nil {
		return nil, nil, "", err
	}
	return market, loaded, "", nil
}

// runOnSnapshots runs the selection engine against in-memory snapshots with a
// run repository that discards every write.
func runOnSnapshots(ctx context.Context, methods selection.MethodRepository, snapshots selection.SnapshotRepository, marketID, featureID string) (*selection.Run, error) {
	engine, err := selection.NewEngine(snapshots, methods, discardRuns{})
	if err != nil {
		return nil, err
	}
	return engine.Run(ctx, selection.Request{MarketSnapshotID: marketID, FeatureSnapshotID: featureID})
}

// digestFor fingerprints the inputs of the trade date. It is the same digest
// for both paths: it describes the facts, not the storage layout.
func digestFor(ctx context.Context, db *storage.Storage, date string, featureIDs []string) (string, error) {
	specs := make([]marketsnapshot.FeatureSpec, 0, len(featureIDs))
	for _, id := range featureIDs {
		specs = append(specs, marketsnapshot.FeatureSpec{Name: id})
	}
	in, err := runinput.Build(ctx, db.DB(), date, specs, selection.EngineVersion)
	if err != nil {
		return "", fmt.Errorf("collect run inputs: %w", err)
	}
	digest, err := runinput.Digest(in)
	if err != nil {
		return "", err
	}
	return digest, nil
}

// storedRunNote reports whether a production selection run already exists for
// this exact snapshot pair, without using it as either path.
func storedRunNote(ctx context.Context, runs selection.Repository, date, marketID, featureID string) string {
	items, err := runs.List(ctx, date, "", 100)
	if err != nil {
		return "读取已落库选股运行失败: " + err.Error()
	}
	for _, run := range items {
		if run.SnapshotID != marketID || run.FeatureSnapshotID != featureID {
			continue
		}
		note := fmt.Sprintf("已落库选股运行 %s (engine_version=%s) 存在，本次未使用它以免引擎版本差异混入对账", run.ID, run.EngineVersion)
		if run.EngineVersion != selection.EngineVersion {
			note += "；其引擎版本与当前不同，旧记录不能代表当前快照路径"
		}
		return note
	}
	return ""
}

func normalizeDate(value string) (string, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("--date 必须是 YYYY-MM-DD: %w", err)
	}
	return parsed.Format("2006-01-02"), nil
}
