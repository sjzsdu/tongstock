package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

func TestBuildStockDataServiceCacheOnlyNeverCreatesTDXService(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache-only.db")
	seed, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.DB().Exec(`INSERT INTO kline
		(code, ktype, date, open, high, low, close, volume, amount)
		VALUES ('600000', 9, '20260730', 10, 10.2, 9.8, 10.1, 100000, 1010000)`); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Database.DSN = path
	cfg.TDX.Hosts = []string{"127.0.0.1:1"}
	factoryCalls := 0
	service, cleanup, err := buildStockDataService(
		context.Background(), cfg, stockdata.CacheOnly,
		func([]string, *storage.Storage) (*tdx.Service, error) {
			factoryCalls++
			return nil, errors.New("network factory must not be called")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if factoryCalls != 0 {
		t.Fatalf("TDX service factory calls = %d, want 0", factoryCalls)
	}
	result, err := service.Query(context.Background(), stockdata.DataRequest{
		Mode: stockdata.CacheOnly,
		Spec: stockdata.DataSpec{
			Type: stockdata.DataKline, Market: "sh", Code: "600000",
			Granularity: "day", KType: 9,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Klines) != 1 || result.Metadata.SyncStatus != "cache" {
		t.Fatalf("cache-only result = %+v", result)
	}
}

func TestCacheOnlyMissHasExplicitRecoveryHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache-only.db")
	cfg := config.DefaultConfig()
	cfg.Database.DSN = path
	service, cleanup, err := buildStockDataService(
		context.Background(), cfg, stockdata.CacheOnly, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	spec := stockdata.DataSpec{
		Type: stockdata.DataKline, Market: "sh", Code: "600999",
		Granularity: "day", KType: 9,
	}
	_, queryErr := service.Query(context.Background(), stockdata.DataRequest{
		Mode: stockdata.CacheOnly, Spec: spec,
	})
	if stockdata.CodeOf(queryErr) != stockdata.ErrCacheMiss {
		t.Fatalf("query error = %v, want cache miss", queryErr)
	}
	previous := dataConsistency
	dataConsistency = "cache_only"
	defer func() { dataConsistency = previous }()
	message := cliDataError(queryErr, spec).Error()
	for _, expected := range []string{"本地缓存缺少", "sync daily", "require_fresh", "--refresh"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("recovery hint %q does not contain %q", message, expected)
		}
	}
}

func TestCacheOnlyMissingRequestedRangeIsCacheMiss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache-only.db")
	seed, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.DB().Exec(`INSERT INTO kline
		(code, ktype, date, open, high, low, close, volume, amount)
		VALUES ('600000', 9, '20250102', 10, 10.2, 9.8, 10.1, 100000, 1010000)`); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Database.DSN = path
	service, cleanup, err := buildStockDataService(context.Background(), cfg, stockdata.CacheOnly, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	_, err = service.Query(context.Background(), stockdata.DataRequest{
		Mode: stockdata.CacheOnly,
		Spec: stockdata.DataSpec{
			Type: stockdata.DataKline, Market: "sh", Code: "600000",
			Granularity: "day", KType: 9,
			Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local),
			End:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.Local),
		},
	})
	if stockdata.CodeOf(err) != stockdata.ErrCacheMiss {
		t.Fatalf("missing requested range error = %v, want cache miss", err)
	}
}

func TestLegacyTDXCommandFailsBeforeDialInCacheOnly(t *testing.T) {
	previous := dataConsistency
	dataConsistency = "cache_only"
	defer func() { dataConsistency = previous }()
	_, err := dialService()
	if err == nil || !strings.Contains(err.Error(), "cache_only 禁止网络连接") {
		t.Fatalf("dialService error = %v", err)
	}
}
