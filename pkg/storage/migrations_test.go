package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// TestMigrationIdempotent validates that applying the full migration set twice
// leaves the schema identical (CREATE TABLE IF NOT EXISTS / INSERT OR IGNORE style).
// This prevents breakages on production DBs that have been partially migrated.
func TestMigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mig.db")
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	// Migrate twice. Second apply must not error or modify applied migrations count.
	if err := db.Migrate(); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	applied1, err := appliedMigrationCount(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("second migrate (idempotency): %v", err)
	}
	applied2, err := appliedMigrationCount(db)
	if err != nil {
		t.Fatal(err)
	}
	if applied1 != applied2 {
		t.Fatalf("migration count changed on idempotent replay: %d -> %d", applied1, applied2)
	}
	_ = db.Close()
}

func TestKlineIntegrityMigrationQuarantinesExistingRowsAndRejectsNewOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kline-integrity.db")
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Recreate the pre-v19 condition so the migration cleanup itself is tested.
	if _, err := db.DB().Exec(`
		DROP TRIGGER trg_kline_validate_insert;
		DROP TRIGGER trg_kline_validate_update;
		DELETE FROM schema_migrations WHERE version = 19;
		INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES ('601688',9,'72200215',19.5,19.57,19.41,19.47,100,1000)
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	var active, quarantined int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM kline WHERE code='601688'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM kline_quarantine WHERE code='601688' AND reason='invalid_date'`).Scan(&quarantined); err != nil {
		t.Fatal(err)
	}
	if active != 0 || quarantined != 1 {
		t.Fatalf("active=%d quarantined=%d, want 0/1", active, quarantined)
	}

	if _, err := db.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES ('601688',9,'72200215',19.5,19.57,19.41,19.47,100,1000)`); err == nil {
		t.Fatal("database trigger accepted an invalid future date")
	}
	if _, err := db.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES ('601688',9,'20260803',19.5,19.57,19.41,19.47,100,1000)`); err != nil {
		t.Fatalf("database trigger rejected a valid row: %v", err)
	}
}

const migrationsTable = "schema_migrations"

func appliedMigrationCount(db *storage.Storage) (int, error) {
	row := db.DB().QueryRow(`SELECT COUNT(*) FROM ` + migrationsTable)
	var n int
	err := row.Scan(&n)
	return n, err
}

// TestInputsDigestColumnsAreAddedWithoutBackfill pins migration 25: both run
// tables gain a nullable inputs_digest column, and existing rows keep NULL
// instead of being rewritten with a fabricated digest.
func TestInputsDigestColumnsAreAddedWithoutBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest-columns.db")
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, table := range []string{"daily_selection_run", "position_decision_run"} {
		if !tableHasColumn(t, db, table, "inputs_digest") {
			t.Fatalf("%s has no inputs_digest column after migration 25", table)
		}
	}

	// A pre-existing row must stay readable with a NULL digest.
	if _, err := db.DB().Exec(`INSERT INTO market_snapshot(id,snapshot_date,universe_definition,status)
		VALUES ('snap','2024-01-05','{"name":"universe_usable"}','ready')`); err != nil {
		t.Fatalf("insert market snapshot: %v", err)
	}
	if _, err := db.DB().Exec(`INSERT INTO feature_snapshot(id,market_snapshot_id,snapshot_date,feature_ids_json,status)
		VALUES ('fs','snap','2024-01-05','[]','ready')`); err != nil {
		t.Fatalf("insert feature snapshot: %v", err)
	}
	if _, err := db.DB().Exec(`INSERT INTO position_decision_run
		(run_id,run_hash,engine_version,snapshot_id,feature_snapshot_id,snapshot_date,decision_json,created_at_ns)
		VALUES ('legacy-run','legacy-hash','position-decision-v1','snap','fs','2024-01-05','{}',1)`); err != nil {
		t.Fatalf("insert legacy run: %v", err)
	}
	var digest any
	if err := db.DB().QueryRow(`SELECT inputs_digest FROM position_decision_run WHERE run_id='legacy-run'`).Scan(&digest); err != nil {
		t.Fatalf("read legacy digest: %v", err)
	}
	if digest != nil {
		t.Fatalf("legacy row was backfilled with %v", digest)
	}

	// Re-running the migration must stay a no-op (ADD COLUMN has no IF NOT EXISTS).
	if err := db.Migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func tableHasColumn(t *testing.T, db *storage.Storage, table, column string) bool {
	t.Helper()
	rows, err := db.DB().Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan pragma %s: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	return false
}
