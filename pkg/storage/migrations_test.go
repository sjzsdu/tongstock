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
