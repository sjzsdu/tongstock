package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "tongstock",
	Short: "通达信股票数据查询工具",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch strings.ToLower(strings.TrimSpace(dataConsistency)) {
		case "require_fresh", "allow_stale", "cache_only":
		default:
			return fmt.Errorf("无效的 --consistency: %s", dataConsistency)
		}
		if forceRefresh && dataConsistency == "cache_only" {
			return fmt.Errorf("--refresh 不能与 --consistency=cache_only 同时使用")
		}
		_, err := config.Load()
		return err
	},
}

var (
	dataConsistency string
	forceRefresh    bool
)

func init() {
	rootCmd.PersistentFlags().StringVar(
		&dataConsistency, "consistency", "require_fresh",
		"数据一致性: require_fresh, allow_stale, cache_only",
	)
	rootCmd.PersistentFlags().BoolVar(&forceRefresh, "refresh", false, "强制从 TDX 刷新后再从数据库读取")
}

func cliDataRequest(spec stockdata.DataSpec) stockdata.DataRequest {
	return stockdata.DataRequest{Spec: spec, Mode: cliConsistencyMode(), ForceRefresh: forceRefresh}
}

func cliConsistencyMode() stockdata.ConsistencyMode {
	switch strings.ToLower(strings.TrimSpace(dataConsistency)) {
	case "allow_stale":
		return stockdata.AllowStale
	case "cache_only":
		return stockdata.CacheOnly
	default:
		return stockdata.RequireFresh
	}
}

// dialService creates a connected Service wrapper around a Client.
func dialService() (*tdx.Service, error) {
	if cliConsistencyMode() == stockdata.CacheOnly {
		return nil, fmt.Errorf("当前命令仅支持 TDX 上游，cache_only 禁止网络连接；请改用本地 kline/indicator/screen，或显式执行 sync/refresh")
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	client, err := tdx.DialHosts(cfg.TDX.Hosts)
	if err != nil {
		return nil, err
	}
	s, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return tdx.NewOwnedService(client, s)
}

func dialStockData(ctx context.Context) (*stockdata.Service, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	return buildStockDataService(ctx, cfg, cliConsistencyMode(), func(hosts []string, store *storage.Storage) (*tdx.Service, error) {
		client, err := tdx.DialHosts(hosts)
		if err != nil {
			return nil, err
		}
		service, err := tdx.NewOwnedService(client, store)
		if err != nil {
			_ = client.Close()
			return nil, err
		}
		return service, nil
	})
}

type tdxServiceFactory func([]string, *storage.Storage) (*tdx.Service, error)

func buildStockDataService(
	ctx context.Context,
	cfg *config.Config,
	mode stockdata.ConsistencyMode,
	createTDXService tdxServiceFactory,
) (*stockdata.Service, func(), error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("配置不能为空")
	}
	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, nil, err
	}
	repository, err := stockdata.NewSQLiteRepository(store)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	calendar, err := stockdata.NewSQLiteTradingCalendar(store)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}

	var provider stockdata.Provider
	cleanup := func() { _ = store.Close() }
	if mode == stockdata.CacheOnly {
		provider = stockdata.NewOfflineProvider()
	} else {
		if createTDXService == nil {
			_ = store.Close()
			return nil, nil, fmt.Errorf("TDX service factory 不能为空")
		}
		tdxService, err := createTDXService(cfg.TDX.Hosts, store)
		if err != nil {
			_ = store.Close()
			return nil, nil, err
		}
		if tdxService == nil {
			_ = store.Close()
			return nil, nil, fmt.Errorf("TDX service factory 返回 nil")
		}
		tdxProvider, err := stockdata.NewTDXProvider(tdxService)
		if err != nil {
			_ = tdxService.Close()
			return nil, nil, err
		}
		provider = tdxProvider
		cleanup = func() { _ = tdxService.Close() }
	}
	service, err := stockdata.NewServiceWithContext(
		ctx,
		repository, provider,
		stockdata.NewMarketFreshnessPolicy(calendar, time.Local),
		stockdata.SystemClock{},
	)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return service, cleanup, nil
}

func cliDataError(err error, spec stockdata.DataSpec) error {
	if err == nil {
		return nil
	}
	if cliConsistencyMode() == stockdata.CacheOnly && stockdata.CodeOf(err) == stockdata.ErrCacheMiss {
		return fmt.Errorf("%w；本地缓存缺少 %s 数据，请显式执行同步（如 tongstock sync daily --codes %s --consistency=require_fresh），或使用 --consistency=require_fresh --refresh 重试",
			err, spec.Code, spec.Code)
	}
	return err
}

func init() {
	rootCmd.AddCommand(quoteCmd)
	rootCmd.AddCommand(codesCmd)
	rootCmd.AddCommand(klineCmd)
	rootCmd.AddCommand(minuteCmd)
	rootCmd.AddCommand(tradeCmd)
	rootCmd.AddCommand(xdxrCmd)
	rootCmd.AddCommand(financeCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(companyCmd)
	rootCmd.AddCommand(companyContentCmd)
	rootCmd.AddCommand(blockCmd)
	rootCmd.AddCommand(countCmd)
	rootCmd.AddCommand(auctionCmd)
	rootCmd.AddCommand(indicatorCmd)
	rootCmd.AddCommand(screenCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(qualityCmd)
}
