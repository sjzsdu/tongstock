package asof_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/asof"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func viewNow() time.Time { return time.Now().UTC() }

func newDB(t *testing.T) (*storage.Storage, *sql.DB) {
	t.Helper()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "asof.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, store.DB()
}

func insertBar(t *testing.T, db *sql.DB, code, date string, close float64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES (?,?,?,?,?,?,?,?,?)`, code, asof.DayKType, date, close, close+1, close-1, close, 1000, 1000*close); err != nil {
		t.Fatalf("insert bar %s %s: %v", code, date, err)
	}
}

// TestViewHidesBarsAfterTradeDate is the correctness proof of this layer: data
// written after the trade date must be invisible to both Bars and Universe.
func TestViewHidesBarsAfterTradeDate(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()

	// Two codes with a complete series up to the trade date, plus one bar each
	// after it. The post-date closes are wildly different so any leak shows up
	// as a value change, not just an extra row.
	for _, code := range []string{"600000", "600001"} {
		insertBar(t, db, code, "20240102", 10)
		insertBar(t, db, code, "20240103", 11)
		insertBar(t, db, code, "20240104", 12)
		insertBar(t, db, code, "20240105", 13)
		insertBar(t, db, code, "20240108", 99) // after T
	}
	// A code that only exists after the trade date must not enter the universe.
	insertBar(t, db, "600099", "20240108", 50)

	view, err := asof.Open(ctx, db, "2024-01-05")
	if err != nil {
		t.Fatalf("open as-of view: %v", err)
	}
	if view.TradeDate != "2024-01-05" {
		t.Fatalf("TradeDate = %q", view.TradeDate)
	}

	bars, err := view.Bars(ctx, "600000", asof.DayKType, 10)
	if err != nil {
		t.Fatalf("bars: %v", err)
	}
	if len(bars) != 4 {
		t.Fatalf("got %d bars, want 4 (post-T bar leaked)", len(bars))
	}
	for i, want := range []string{"20240102", "20240103", "20240104", "20240105"} {
		if bars[i].Date != want {
			t.Fatalf("bars[%d].Date = %q, want %q (ascending order)", i, bars[i].Date, want)
		}
	}
	if bars[len(bars)-1].Close != 13 {
		t.Fatalf("last bar Close = %v, want 13 (post-T revision leaked)", bars[len(bars)-1].Close)
	}

	universe, err := view.Universe(ctx)
	if err != nil {
		t.Fatalf("universe: %v", err)
	}
	if len(universe) != 2 || universe[0] != "600000" || universe[1] != "600001" {
		t.Fatalf("universe = %v, want [600000 600001]", universe)
	}
	for _, code := range universe {
		if code == "600099" {
			t.Fatal("universe contains a code that only has post-T data")
		}
	}

	cov, err := view.Coverage(ctx)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if cov.Ready != 2 || cov.Total != 2 || cov.Ratio != 1 {
		t.Fatalf("coverage = %+v, want 2/2 (post-T-only code must not be counted)", cov)
	}
}

func TestViewBarsRespectsLookback(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()
	for _, d := range []string{"20240102", "20240103", "20240104"} {
		insertBar(t, db, "600000", d, 10)
	}
	view, err := asof.Open(ctx, db, "2024-01-04")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	bars, err := view.Bars(ctx, "600000", asof.DayKType, 2)
	if err != nil {
		t.Fatalf("bars: %v", err)
	}
	if len(bars) != 2 || bars[0].Date != "20240103" || bars[1].Date != "20240104" {
		t.Fatalf("lookback=2 -> %v, want the two most recent bars in ascending order", bars)
	}
}

// TestOpenFailsClosedOnLowCoverage pins the fail-closed contract: one stale
// code out of two is 50% coverage, which must error instead of returning a
// view missing that code.
func TestOpenFailsClosedOnLowCoverage(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()
	insertBar(t, db, "600000", "20240105", 10)
	insertBar(t, db, "600001", "20240104", 10) // stale: no bar on T

	view, err := asof.Open(ctx, db, "2024-01-05")
	if err == nil {
		t.Fatalf("Open returned a view despite 50%% coverage: %+v", view)
	}
	if view != nil {
		t.Fatalf("Open returned a non-nil view together with an error: %+v", view)
	}
}

func TestOpenRejectsEmptyAndInvalidDates(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()
	if _, err := asof.Open(ctx, db, "2024-01-05"); err == nil {
		t.Fatal("Open succeeded on an empty database")
	}
	if _, err := asof.Open(ctx, db, "20240105"); err == nil {
		t.Fatal("Open accepted a non-ISO date")
	}
}

// TestBarsSeeRevisionsButNotNewerWrites documents the interaction with the
// append-only revision log: a correction for a bar at or before T is visible
// (that is the whole point of kline_latest), a bar after T is not.
func TestBarsSeeRevisionsButNotNewerWrites(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()
	insertBar(t, db, "600000", "20240104", 10)
	insertBar(t, db, "600000", "20240105", 11)
	initialVersion, err := storage.DataVersion(ctx, db, storage.KlineDataVersionKey)
	if err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := storage.AppendKline(ctx, tx, storage.KlineBar{
		Code: "600000", KType: asof.DayKType, Date: "20240105",
		Open: 11, High: 12, Low: 10, Close: 11.5, Volume: 2000, Amount: 23000,
	}, "as-of test correction", viewNow())
	if err != nil {
		t.Fatalf("append revision: %v", err)
	}
	if !res.Revised {
		t.Fatal("expected an appended revision")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	before, err := asof.Open(ctx, db, "2024-01-04")
	if err != nil {
		t.Fatalf("open T-1: %v", err)
	}
	bars, err := before.Bars(ctx, "600000", asof.DayKType, 10)
	if err != nil {
		t.Fatalf("bars T-1: %v", err)
	}
	if len(bars) != 1 || bars[0].Close != 10 {
		t.Fatalf("T-1 bars = %+v, want only the 20240104 bar", bars)
	}

	after, err := asof.Open(ctx, db, "2024-01-05")
	if err != nil {
		t.Fatalf("open T: %v", err)
	}
	bars, err = after.Bars(ctx, "600000", asof.DayKType, 10)
	if err != nil {
		t.Fatalf("bars T: %v", err)
	}
	if len(bars) != 2 || bars[1].Close != 11.5 {
		t.Fatalf("T bars = %+v, want the revised close 11.5", bars)
	}
	if after.DataVersion <= initialVersion {
		t.Fatalf("data version did not bump: %d -> %d", initialVersion, after.DataVersion)
	}
}
