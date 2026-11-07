// Package asof is a read-only point-in-time view over the append-only K-line
// stream. It exists so a historical decision can be recomputed from the same
// facts the decision could have seen on its trade date, without trusting a
// snapshot's stored copy.
//
// Every query in this package is bounded by `date <= tradeDate`; see
// sql_boundary_test.go for the static gate that enforces it. The layer is
// fail-closed: Open refuses to hand out a view whose coverage is below
// MinCoverage instead of returning partial results.
//
// This package is currently only reachable through `tongstock reconcile asof`
// and its tests. Production reads still go through internal/selection and
// internal/positiondecision, which continue to use frozen snapshots.
package asof

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// DayKType is the K-line type used by daily bars throughout TongStock.
const DayKType uint8 = 9

// MinCoverage is the ready-code ratio below which Open fails. It matches the
// snapshot builder's DefaultReadyCoverageThreshold: one coverage definition,
// so an as-of view and a ready snapshot agree about what "complete" means.
const MinCoverage = 0.99

// defaultLookback caps Bars when the caller asks for an unbounded window. It
// matches the 260 daily bars the feature engine materializes from.
const defaultLookback = 260

// Coverage describes how complete the K-line stream is on TradeDate.
type Coverage struct {
	TradeDate string   `json:"trade_date"`
	Ready     int      `json:"ready_codes"`
	Total     int      `json:"total_codes"`
	Ratio     float64  `json:"coverage"`
	Missing   []string `json:"missing,omitempty"`
}

// View is a point-in-time read surface bound to one trade date. It never
// exposes data newer than TradeDate.
type View struct {
	TradeDate   string
	DataVersion int64

	db       *sql.DB
	universe []string
	coverage Coverage
}

// Open binds a read-only view to tradeDate (YYYY-MM-DD). It computes universe
// and coverage up front and fails when coverage is too low, so callers never
// observe a partially covered view.
func Open(ctx context.Context, db *sql.DB, tradeDate string) (*View, error) {
	if db == nil {
		return nil, errors.New("as-of view requires a database handle")
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(tradeDate))
	if err != nil {
		return nil, fmt.Errorf("as-of trade date must be YYYY-MM-DD: %w", err)
	}
	date := parsed.Format("2006-01-02")
	compact := strings.ReplaceAll(date, "-", "")

	version, err := storage.DataVersion(ctx, db, storage.KlineDataVersionKey)
	if err != nil {
		return nil, fmt.Errorf("read kline data_version: %w", err)
	}

	// One pass over the bounded stream yields both the ready set and the
	// per-code latest bar date, so universe/coverage can never disagree.
	rows, err := db.QueryContext(ctx, `SELECT code, MAX(date) AS last_date
		FROM kline_latest
		WHERE ktype = ? AND date <= ?
		GROUP BY code
		ORDER BY code`, DayKType, compact)
	if err != nil {
		return nil, fmt.Errorf("scan kline_latest up to %s: %w", date, err)
	}
	defer rows.Close()

	cov := Coverage{TradeDate: date}
	var ready []string
	for rows.Next() {
		var code, last string
		if err := rows.Scan(&code, &last); err != nil {
			return nil, err
		}
		cov.Total++
		if last == compact {
			cov.Ready++
			// Universe is "codes with a bar on the trade date": a code whose
			// latest bar predates TradeDate has no point-in-time data for the
			// decision and must not be treated as a member.
			ready = append(ready, code)
		} else {
			cov.Missing = append(cov.Missing, code)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if cov.Total == 0 {
		return nil, fmt.Errorf("no kline rows on or before %s: cannot open as-of view", date)
	}
	cov.Ratio = float64(cov.Ready) / float64(cov.Total)
	if cov.Ratio < MinCoverage {
		return nil, fmt.Errorf("as-of coverage %.4f (%d/%d codes) is below %.2f on %s; refusing to return partial results",
			cov.Ratio, cov.Ready, cov.Total, MinCoverage, date)
	}
	sort.Strings(ready)
	sort.Strings(cov.Missing)

	return &View{
		TradeDate:   date,
		DataVersion: version,
		db:          db,
		universe:    ready,
		coverage:    cov,
	}, nil
}

// Bars returns the last lookback bars at or before TradeDate, oldest first.
// A non-positive lookback falls back to the default 260-bar window.
func (v *View) Bars(ctx context.Context, code string, ktype uint8, lookback int) ([]storage.KlineBar, error) {
	if v == nil || v.db == nil {
		return nil, errors.New("as-of view is not open")
	}
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("as-of bars requires a code")
	}
	if lookback <= 0 {
		lookback = defaultLookback
	}
	compact := strings.ReplaceAll(v.TradeDate, "-", "")
	rows, err := v.db.QueryContext(ctx, `SELECT date, open, high, low, close, volume, amount
		FROM kline_latest
		WHERE code = ? AND ktype = ? AND date <= ?
		ORDER BY date DESC
		LIMIT ?`, code, ktype, compact, lookback)
	if err != nil {
		return nil, fmt.Errorf("query as-of bars for %s: %w", code, err)
	}
	defer rows.Close()
	var out []storage.KlineBar
	for rows.Next() {
		bar := storage.KlineBar{Code: code, KType: ktype}
		if err := rows.Scan(&bar.Date, &bar.Open, &bar.High, &bar.Low, &bar.Close, &bar.Volume, &bar.Amount); err != nil {
			return nil, err
		}
		out = append(out, bar)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Query is newest-first so LIMIT keeps the most recent window; callers want
	// ascending time order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// Universe returns the codes that have a bar exactly on TradeDate, sorted.
func (v *View) Universe(ctx context.Context) ([]string, error) {
	if v == nil || v.db == nil {
		return nil, errors.New("as-of view is not open")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]string(nil), v.universe...), nil
}

// Coverage reports how many codes have a bar on TradeDate versus how many have
// any history up to it, plus the stale-code list.
func (v *View) Coverage(ctx context.Context) (Coverage, error) {
	if v == nil || v.db == nil {
		return Coverage{}, errors.New("as-of view is not open")
	}
	if err := ctx.Err(); err != nil {
		return Coverage{}, err
	}
	cov := v.coverage
	cov.Missing = append([]string(nil), v.coverage.Missing...)
	return cov, nil
}
