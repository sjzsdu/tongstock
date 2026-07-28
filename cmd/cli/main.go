package main

import (
	"context"
	"fmt"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"time"
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
	mode := stockdata.RequireFresh
	switch strings.ToLower(strings.TrimSpace(dataConsistency)) {
	case "allow_stale":
		mode = stockdata.AllowStale
	case "cache_only":
		mode = stockdata.CacheOnly
	}
	return stockdata.DataRequest{Spec: spec, Mode: mode, ForceRefresh: forceRefresh}
}

// dialService creates a connected Service wrapper around a Client.
func dialService() (*tdx.Service, error) {
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
	client, err := tdx.DialHosts(cfg.TDX.Hosts)
	if err != nil {
		return nil, nil, err
	}
	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	tdxService, err := tdx.NewOwnedService(client, store)
	if err != nil {
		_ = store.Close()
		_ = client.Close()
		return nil, nil, err
	}
	repository, err := stockdata.NewSQLiteRepository(store)
	if err != nil {
		_ = tdxService.Close()
		return nil, nil, err
	}
	provider, err := stockdata.NewTDXProvider(tdxService)
	if err != nil {
		_ = tdxService.Close()
		return nil, nil, err
	}
	calendar, err := stockdata.NewSQLiteTradingCalendar(store)
	if err != nil {
		_ = tdxService.Close()
		return nil, nil, err
	}
	service, err := stockdata.NewServiceWithContext(
		ctx,
		repository, provider,
		stockdata.NewMarketFreshnessPolicy(calendar, time.Local),
		stockdata.SystemClock{},
	)
	if err != nil {
		_ = tdxService.Close()
		return nil, nil, err
	}
	cleanup := func() {
		_ = tdxService.Close()
	}
	return service, cleanup, nil
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
}
