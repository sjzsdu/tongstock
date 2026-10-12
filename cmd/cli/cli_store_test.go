package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCLIConfig(t *testing.T, home, dsn string) {
	t.Helper()
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	contents := "database:\n  driver: sqlite3\n  dsn: " + dsn + "\n"
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCLIStoreMigratesConfiguredSQLite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dsn := filepath.Join(home, "configured.db")
	writeCLIConfig(t, home, dsn)

	store, err := openCLIStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var migrations int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if migrations == 0 {
		t.Fatal("openCLIStore returned a store without migrated schema")
	}
}

func TestOpenCLIStoreFallsBackToDefaultsWhenConfigUnreadable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "config.yaml"), 0755); err != nil {
		t.Fatal(err)
	}

	store, err := openCLIStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := os.Stat(filepath.Join(home, "cache", "tongstock.db")); err != nil {
		t.Fatalf("default database was not opened: %v", err)
	}
}

func TestOpenCLIStoreRejectsUnsupportedDriver(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("database:\n  driver: postgres\n  dsn: invalid\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := openCLIStore()
	if err == nil || !strings.Contains(err.Error(), "仅支持 sqlite3") {
		t.Fatalf("openCLIStore error = %v, want unsupported-driver error", err)
	}
}

func TestCLIWiringSmoke(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeCLIConfig(t, home, filepath.Join(home, "wiring.db"))

	selectionStore, snapshots, selectionEngine, runs, err := wireSelection()
	if err != nil {
		t.Fatalf("wireSelection: %v", err)
	}
	if snapshots == nil || selectionEngine == nil || runs == nil {
		t.Fatal("wireSelection returned incomplete dependencies")
	}
	selectionStore.Close()

	marketStore, marketRepo, builder, err := wireMarket()
	if err != nil {
		t.Fatalf("wireMarket: %v", err)
	}
	if marketRepo == nil || builder == nil {
		t.Fatal("wireMarket returned incomplete dependencies")
	}
	marketStore.Close()

	automationStore, marketSnapshots, orchestrator, err := wireAutomation()
	if err != nil {
		t.Fatalf("wireAutomation: %v", err)
	}
	if marketSnapshots == nil || orchestrator == nil {
		t.Fatal("wireAutomation returned incomplete dependencies")
	}
	automationStore.Close()
}
