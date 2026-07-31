package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacySQLitePreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	fixture, err := os.ReadFile(filepath.Join("testdata", "legacy-v0.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := New(Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatalf("New() migration error = %v", err)
	}
	defer store.Close()

	var historyName string
	if err := store.DB().QueryRow(`SELECT name FROM history_stocks WHERE code = '600000'`).Scan(&historyName); err != nil {
		t.Fatalf("legacy history row missing after migration: %v", err)
	}
	var group, note string
	var updatedAt int64
	if err := store.DB().QueryRow(`SELECT "group", note, updated_at FROM watchlist WHERE code = '000001'`).
		Scan(&group, &note, &updatedAt); err != nil {
		t.Fatalf("legacy watchlist row missing after migration: %v", err)
	}
	if group != "default" || note != "" || updatedAt != 0 {
		t.Fatalf("migrated defaults = (%q, %q, %d)", group, note, updatedAt)
	}
	version, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != 10 {
		t.Fatalf("schema version = %d, want 10", version)
	}
}

func TestSQLiteTransactionRollbackContract(t *testing.T) {
	store, err := New(Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "contract.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO quote_snapshot(code, payload, source_updated_at, updated_at) VALUES ('600000', '{}', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM quote_snapshot`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled back row count = %d", count)
	}
}

func TestUnsupportedDriverIsRejected(t *testing.T) {
	if _, err := New(Config{Driver: "mysql", DSN: "ignored"}); err == nil {
		t.Fatal("New(mysql) error = nil")
	}
}
