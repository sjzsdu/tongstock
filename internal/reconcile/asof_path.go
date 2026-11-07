package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/sjzsdu/tongstock/internal/asof"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/selection"
)

// asOfFeatureEngine materializes the K-line derived features from the as-of
// view itself instead of re-reading the production feature table.
//
// Features with no point-in-time source (market cap, turnover, shareholder
// counts) cannot be derived from bars at all; their fallback values come from
// the stored feature snapshot. Both paths therefore read the same non-PIT
// inputs by construction, and a difference can only come from the K-line facts
// this layer is responsible for.
type asOfFeatureEngine struct {
	ctx      context.Context
	view     *asof.View
	fallback map[string]map[string]float64
	lookback int
}

func (e *asOfFeatureEngine) Compute(_ string, codes []string, features []marketsnapshot.FeatureSpec) (map[string]map[string]float64, error) {
	if e == nil || e.view == nil {
		return nil, fmt.Errorf("as-of feature engine requires an open view")
	}
	ctx := e.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	lookback := e.lookback
	if lookback <= 0 {
		lookback = 260
	}
	out := make(map[string]map[string]float64, len(codes))
	for _, code := range codes {
		perCode := map[string]float64{}
		bars, err := e.view.Bars(ctx, code, asof.DayKType, lookback)
		if err != nil {
			return nil, err
		}
		if len(bars) > 0 {
			converted := make([]methods.Bar, 0, len(bars))
			for _, b := range bars {
				converted = append(converted, methods.Bar{
					Date: b.Date, Open: b.Open, High: b.High, Low: b.Low,
					Close: b.Close, Volume: b.Volume, Amount: b.Amount,
				})
			}
			for _, f := range features {
				if v, ok := methods.ComputeBuiltinIndicator(f.Name, converted); ok {
					perCode[f.Name] = v
				}
			}
		}
		for _, f := range features {
			if _, ok := perCode[f.Name]; ok {
				continue
			}
			if v, ok := e.fallback[code][f.Name]; ok {
				perCode[f.Name] = v
			}
		}
		out[code] = perCode
	}
	return out, nil
}

// AsOfInputs are the in-memory artifacts the as-of path feeds into the
// selection engine. They are never saved.
type AsOfInputs struct {
	Market   *marketsnapshot.MarketSnapshot
	Feature  *marketsnapshot.FeatureSnapshot
	Coverage asof.Coverage
	Dropped  []string
}

// BuildAsOf materializes a market + feature snapshot pair from the as-of view.
// The universe definition comes from the frozen snapshot so both paths answer
// the same question; the data comes exclusively from the bounded view.
func BuildAsOf(
	ctx context.Context,
	view *asof.View,
	template *marketsnapshot.MarketSnapshot,
	universe marketsnapshot.UniverseProvider,
	featureIDs []string,
	fallback map[string]map[string]float64,
) (*AsOfInputs, error) {
	if view == nil || template == nil {
		return nil, fmt.Errorf("as-of build requires a view and a template snapshot")
	}
	asOfCodes, err := view.Universe(ctx)
	if err != nil {
		return nil, err
	}
	inAsOf := make(map[string]bool, len(asOfCodes))
	for _, c := range asOfCodes {
		inAsOf[c] = true
	}

	members, err := universe.BuildUniverse(view.TradeDate, template.Universe)
	if err != nil {
		return nil, fmt.Errorf("rebuild universe as of %s: %w", view.TradeDate, err)
	}

	dropped := []string{}
	selected := make([]marketsnapshot.UniverseMember, 0, len(members))
	for i := range members {
		m := members[i]
		if !m.Selected {
			continue
		}
		if !inAsOf[m.Code] {
			// The frozen snapshot selected this code but the bounded stream has
			// no bar for it on the trade date: the as-of path cannot use it.
			members[i].Selected = false
			members[i].ExcludeReasons = append(members[i].ExcludeReasons, "no_bar_as_of")
			dropped = append(dropped, m.Code)
			continue
		}
		selected = append(selected, m)
	}
	sort.Strings(dropped)
	if len(selected) == 0 {
		return nil, fmt.Errorf("as-of universe on %s is empty", view.TradeDate)
	}

	codeStates := make([]marketsnapshot.CodeStatus, 0, len(selected))
	for _, m := range selected {
		codeStates = append(codeStates, marketsnapshot.CodeStatus{
			Code:           m.Code,
			UniverseMember: true,
			SecurityStatus: m.Status,
			IpoDate:        m.IpoDate,
			DelistDate:     m.DelistDate,
			KlineLastDate:  view.TradeDate,
			QuoteReady:     true,
			FinanceReady:   true,
			XdxrReady:      true,
		})
	}
	market := &marketsnapshot.MarketSnapshot{
		SnapshotDate:       view.TradeDate,
		Universe:           template.Universe,
		Market:             template.Market,
		PriceAdjustment:    template.PriceAdjustment,
		ExpectedKlineCodes: len(selected),
		ReadyKlineCodes:    len(selected),
		ReadyQuoteCodes:    len(selected),
		ReadyFinanceCodes:  len(selected),
		ReadyXdxrCodes:     len(selected),
		CoveragePct:        1,
		Status:             marketsnapshot.StatusReady,
		ReadinessReason:    "as-of view (bounded kline_latest)",
		UniverseHash:       marketsnapshot.ComputeUniverseHash(members),
		Frozen:             true,
		UniverseMembers:    selected,
		Codes:              codeStates,
	}
	market.ID = derivedID("asof-ms-", market.SnapshotDate, market.Universe.Name, market.PriceAdjustment, market.UniverseHash)
	contentHash, err := marketsnapshot.ComputeContentHash(market)
	if err != nil {
		return nil, err
	}
	market.ContentHash = contentHash

	specs := make([]marketsnapshot.FeatureSpec, 0, len(featureIDs))
	codes := make([]string, 0, len(selected))
	for _, m := range selected {
		codes = append(codes, m.Code)
	}
	for _, id := range featureIDs {
		specs = append(specs, marketsnapshot.FeatureSpec{Name: id})
	}
	engine := &asOfFeatureEngine{ctx: ctx, view: view, fallback: fallback, lookback: 260}
	values, err := engine.Compute(view.TradeDate, codes, specs)
	if err != nil {
		return nil, fmt.Errorf("compute as-of features: %w", err)
	}

	featureIDsCopy := append([]string(nil), featureIDs...)
	sort.Strings(featureIDsCopy)
	rows := 0
	for _, perCode := range values {
		rows += len(perCode)
	}
	feature := &marketsnapshot.FeatureSnapshot{
		MarketSnapshotID: market.ID,
		SnapshotDate:     view.TradeDate,
		FeatureIDs:       featureIDsCopy,
		FeatureTotal:     len(featureIDsCopy),
		RowsWritten:      rows,
		LeakChecked:      true,
		PriceAdjustment:  template.PriceAdjustment,
		Status:           marketsnapshot.StatusReady,
		Values:           values,
	}
	feature.ContentHash, err = marketsnapshot.ComputeFeatureContentHash(feature)
	if err != nil {
		return nil, err
	}
	feature.ID = derivedID("asof-fs-", market.ID, view.TradeDate, feature.ContentHash)

	cov, err := view.Coverage(ctx)
	if err != nil {
		return nil, err
	}
	return &AsOfInputs{Market: market, Feature: feature, Coverage: cov, Dropped: dropped}, nil
}

// derivedID builds a stable id from its parts. It mirrors the snapshot
// builder's id derivation so ids stay recognisable, with an as-of prefix so a
// derived artifact can never be mistaken for a stored snapshot.
func derivedID(prefix string, parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return prefix + hex.EncodeToString(h[:])[:20]
}

// memorySnapshots serves in-memory snapshots to the selection engine. Nothing
// is persisted: a reconcile run must not add rows to the run tables.
type memorySnapshots struct {
	market   *marketsnapshot.MarketSnapshot
	features map[string]*marketsnapshot.FeatureSnapshot
}

func (m *memorySnapshots) LoadMarketSnapshot(id string, _ bool) (*marketsnapshot.MarketSnapshot, error) {
	if m.market == nil || m.market.ID != id {
		return nil, fmt.Errorf("market snapshot %s not found in reconcile memory", id)
	}
	return m.market, nil
}

func (m *memorySnapshots) LoadFeatureSnapshot(id string, _ bool) (*marketsnapshot.FeatureSnapshot, error) {
	if f, ok := m.features[id]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("feature snapshot %s not found in reconcile memory", id)
}

func (m *memorySnapshots) ListFeatureSnapshots(marketSnapshotID string) ([]*marketsnapshot.FeatureSnapshot, error) {
	out := []*marketsnapshot.FeatureSnapshot{}
	for _, f := range m.features {
		if f.MarketSnapshotID == marketSnapshotID {
			out = append(out, f)
		}
	}
	return out, nil
}

// discardRuns is a selection.Repository that never writes and never hits a
// cached run, so both paths always recompute and the stored run tables stay
// untouched.
type discardRuns struct{}

func (discardRuns) Save(context.Context, *selection.Run) error { return nil }

func (discardRuns) Get(context.Context, string, string) (*selection.Run, error) {
	return nil, selection.ErrNotFound
}

func (discardRuns) List(context.Context, string, string, int) ([]*selection.Run, error) {
	return nil, nil
}
