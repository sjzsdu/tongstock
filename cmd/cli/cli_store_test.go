package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/config"
)

func setCLIConfig(t *testing.T, cfgLoader func() (*config.Config, error), defaultConfig func() *config.Config) {
	t.Helper()
	previousLoader, previousDefault := cliConfigLoader, cliDefaultConfig
	cliConfigLoader, cliDefaultConfig = cfgLoader, defaultConfig
	t.Cleanup(func() { cliConfigLoader, cliDefaultConfig = previousLoader, previousDefault })
}

func TestOpenCLIStoreMigratesConfiguredSQLite(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "configured.db")
	setCLIConfig(t, func() (*config.Config, error) {
		cfg := config.DefaultConfig()
		cfg.Database.DSN = dsn
		return cfg, nil
	}, config.DefaultConfig)

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
	dsn := filepath.Join(t.TempDir(), "fallback.db")
	setCLIConfig(t, func() (*config.Config, error) {
		return nil, errors.New("unreadable config")
	}, func() *config.Config {
		cfg := config.DefaultConfig()
		cfg.Database.DSN = dsn
		return cfg
	})

	store, err := openCLIStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
}

func TestOpenCLIStoreRejectsUnsupportedDriver(t *testing.T) {
	setCLIConfig(t, func() (*config.Config, error) {
		cfg := config.DefaultConfig()
		cfg.Database.Driver = "postgres"
		cfg.Database.DSN = "invalid"
		return cfg, nil
	}, config.DefaultConfig)

	_, err := openCLIStore()
	if err == nil || !strings.Contains(err.Error(), "仅支持 sqlite3") {
		t.Fatalf("openCLIStore error = %v, want unsupported-driver error", err)
	}
}

func TestCLIWiringSmoke(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "wiring.db")
	setCLIConfig(t, func() (*config.Config, error) {
		cfg := config.DefaultConfig()
		cfg.Database.DSN = dsn
		return cfg, nil
	}, config.DefaultConfig)

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
