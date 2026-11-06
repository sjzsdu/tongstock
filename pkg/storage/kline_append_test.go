package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "append.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func readLatest(t *testing.T, db *storage.Storage, bar storage.KlineBar) storage.KlineBar {
	t.Helper()
	got := bar
	err := db.DB().QueryRow(`SELECT open, high, low, close, volume, amount FROM kline_latest
		WHERE code = ? AND ktype = ? AND date = ?`, bar.Code, bar.KType, bar.Date).
		Scan(&got.Open, &got.High, &got.Low, &got.Close, &got.Volume, &got.Amount)
	if err != nil {
		t.Fatalf("read kline_latest: %v", err)
	}
	return got
}

func TestKlineAppendOnlyRevisionFlow(t *testing.T) {
	db := newTestStorage(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)

	bar := storage.KlineBar{Code: "600000", KType: 9, Date: "20240102",
		Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1000, Amount: 10500}

	appendBar := func(b storage.KlineBar, reason string) storage.KlineAppendResult {
		t.Helper()
		tx, err := db.DB().BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := storage.AppendKline(ctx, tx, b, reason, now)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("append: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		return res
	}

	if res := appendBar(bar, "initial sync"); !res.Inserted || res.Version != 1 {
		t.Fatalf("first write = %+v, want inserted at version 1", res)
	}
	if v, err := db.KlineDataVersion(ctx); err != nil || v != 1 {
		t.Fatalf("data_version = %d (%v), want 1", v, err)
	}

	// Re-sending identical values must not create history.
	if res := appendBar(bar, "repeat sync"); res.Inserted || res.Revised || res.Version != 1 {
		t.Fatalf("identical rewrite = %+v, want no-op at version 1", res)
	}
	var revisions int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM kline_revision`).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 0 {
		t.Fatalf("revision rows = %d after identical rewrite, want 0", revisions)
	}

	// A correction appends a revision and bumps the table version.
	corrected := bar
	corrected.Close = 12.0
	corrected.High = 12.5
	corrected.Amount = 12000
	if res := appendBar(corrected, "adjustment factor recompute"); !res.Revised || res.Version != 2 {
		t.Fatalf("correction = %+v, want revision at version 2", res)
	}
	if v, err := db.KlineDataVersion(ctx); err != nil || v != 2 {
		t.Fatalf("data_version after correction = %d (%v), want 2", v, err)
	}

	// Main table keeps the first observed value...
	var mainClose float64
	if err := db.DB().QueryRow(`SELECT close FROM kline WHERE code = '600000' AND ktype = 9 AND date = '20240102'`).Scan(&mainClose); err != nil {
		t.Fatal(err)
	}
	if mainClose != bar.Close {
		t.Fatalf("main table close = %v, want untouched %v", mainClose, bar.Close)
	}
	// ...while reads see the latest version.
	if got := readLatest(t, db, bar); got.Close != corrected.Close {
		t.Fatalf("kline_latest close = %v, want %v", got.Close, corrected.Close)
	}

	revisionRows, err := storage.KlineRevisions(ctx, db.DB(), bar.Code, bar.KType, bar.Date)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisionRows) != 1 || revisionRows[0].Version != 2 ||
		revisionRows[0].Reason != "adjustment factor recompute" {
		t.Fatalf("revisions = %+v, want one v2 row with reason", revisionRows)
	}

	// Direct UPDATE is rejected by the append-only guard.
	if _, err := db.DB().Exec(`UPDATE kline SET close = 99 WHERE code = '600000'`); err == nil {
		t.Fatal("UPDATE on kline must be rejected")
	}
}

func TestKlineLatestFallsBackToMainTableForLegacyRows(t *testing.T) {
	db := newTestStorage(t)
	if _, err := db.DB().Exec(`INSERT INTO kline
		(code, ktype, date, open, high, low, close, volume, amount)
		VALUES ('000001', 9, '20240103', 10, 11, 9, 10.5, 100, 1050)`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	got := readLatest(t, db, storage.KlineBar{Code: "000001", KType: 9, Date: "20240103"})
	if got.Close != 10.5 || got.High != 11 || got.Volume != 100 {
		t.Fatalf("legacy row through view = %+v, want main-table values", got)
	}
	if v, err := db.KlineDataVersion(context.Background()); err != nil || v != 1 {
		t.Fatalf("data_version for legacy data = %d (%v), want 1", v, err)
	}
}
