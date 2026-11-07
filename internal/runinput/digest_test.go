package runinput_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/runinput"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func newDB(t *testing.T) (*storage.Storage, *sql.DB) {
	t.Helper()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "digest.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, store.DB()
}

func insertBar(t *testing.T, db *sql.DB, code, date string, close float64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES (?,?,?,?,?,?,?,?,?)`, code, 9, date, close, close+1, close-1, close, 1000, 1000*close); err != nil {
		t.Fatalf("insert bar %s %s: %v", code, date, err)
	}
}

func features() []marketsnapshot.FeatureSpec {
	return []marketsnapshot.FeatureSpec{
		{Name: "close", Window: 1},
		{Name: "ma5", Window: 5},
		// Duplicate names must not change the digest: the manifest is a set.
		{Name: "ma5", Window: 5},
	}
}

func TestDigestIsStableForIdenticalInputs(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()
	insertBar(t, db, "600000", "20240104", 10)
	insertBar(t, db, "600000", "20240105", 11)
	insertBar(t, db, "600001", "20240105", 20)

	first, err := runinput.Build(ctx, db, "2024-01-05", features(), "selection-v4")
	if err != nil {
		t.Fatalf("build #1: %v", err)
	}
	second, err := runinput.Build(ctx, db, "2024-01-05", features(), "selection-v4")
	if err != nil {
		t.Fatalf("build #2: %v", err)
	}
	d1, err := runinput.Digest(first)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := runinput.Digest(second)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("same inputs produced different digests:\n%s\n%s", d1, d2)
	}
	if len(d1) != 64 {
		t.Fatalf("digest %q is not a sha256 hex string", d1)
	}
	if len(first.Features) != 2 {
		t.Fatalf("feature manifest kept duplicates: %+v", first.Features)
	}
}

func TestDigestChangesAfterKlineRevision(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()
	insertBar(t, db, "600000", "20240104", 10)
	insertBar(t, db, "600000", "20240105", 11)

	before, err := runinput.Build(ctx, db, "2024-01-05", features(), "selection-v4")
	if err != nil {
		t.Fatalf("build before: %v", err)
	}
	digestBefore, err := runinput.Digest(before)
	if err != nil {
		t.Fatal(err)
	}

	// A correction to a bar the trade date already saw. Only data_version moves,
	// which is exactly the signal a snapshot id cannot carry.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := storage.AppendKline(ctx, tx, storage.KlineBar{
		Code: "600000", KType: 9, Date: "20240105",
		Open: 11, High: 12, Low: 10, Close: 11.5, Volume: 2000, Amount: 23000,
	}, "digest sensitivity test", time.Now().UTC())
	if err != nil {
		t.Fatalf("append revision: %v", err)
	}
	if !res.Revised {
		t.Fatal("expected a revision to be appended")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	after, err := runinput.Build(ctx, db, "2024-01-05", features(), "selection-v4")
	if err != nil {
		t.Fatalf("build after: %v", err)
	}
	digestAfter, err := runinput.Digest(after)
	if err != nil {
		t.Fatal(err)
	}
	if digestBefore == digestAfter {
		t.Fatal("digest did not change after a kline revision")
	}
	if after.DataVersion <= before.DataVersion {
		t.Fatalf("data version did not bump: %d -> %d", before.DataVersion, after.DataVersion)
	}
}

// TestDigestIgnoresFactsAfterTradeDate proves the digest is a point-in-time
// identity: writing a bar after T must not change T's digest.
func TestDigestIgnoresFactsAfterTradeDate(t *testing.T) {
	_, db := newDB(t)
	ctx := context.Background()
	insertBar(t, db, "600000", "20240104", 10)
	insertBar(t, db, "600000", "20240105", 11)

	before, err := runinput.Digest(mustBuild(t, ctx, db, "2024-01-05"))
	if err != nil {
		t.Fatal(err)
	}
	insertBar(t, db, "600000", "20240108", 99)
	insertBar(t, db, "600009", "20240108", 5)
	after, err := runinput.Digest(mustBuild(t, ctx, db, "2024-01-05"))
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("digest changed after a bar that postdates the trade date")
	}
	// ... while the newer trade date does see them.
	newer, err := runinput.Digest(mustBuild(t, ctx, db, "2024-01-08"))
	if err != nil {
		t.Fatal(err)
	}
	if newer == before {
		t.Fatal("digest for a later trade date equals the earlier one")
	}
}

func mustBuild(t *testing.T, ctx context.Context, db *sql.DB, date string) runinput.Inputs {
	t.Helper()
	in, err := runinput.Build(ctx, db, date, features(), "selection-v4")
	if err != nil {
		t.Fatalf("build %s: %v", date, err)
	}
	return in
}
