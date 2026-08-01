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

const migrationsTable = "schema_migrations"

func appliedMigrationCount(db *storage.Storage) (int, error) {
	row := db.DB().QueryRow(`SELECT COUNT(*) FROM ` + migrationsTable)
	var n int
	err := row.Scan(&n)
	return n, err
}
