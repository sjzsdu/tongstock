package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
)

var marketCmd = &cobra.Command{
	Use:   "market",
	Short: "每日市场快照 (MarketSnapshot) + 特征快照 (FeatureSnapshot)",
	Long: `封装 tongstock-ai.7: 全市场真实数据就绪与每日特征快照。
下游 (选股 / AI 挖掘 / 回测) 只能引用 status=ready 且 frozen=1 的 snapshot_id。
Fail closed: 如果 coverage / gap 未达阈值，扫描阶段不使用该快照。`,
}

var (
	mkAdj       = "forward"
	mkUniverse  = "universe_usable"
	mkThreshold = 0.99
	mkMaxGap    = 50
	mkCodes     string
)

var marketBuildCmd = &cobra.Command{
	Use:   "build <YYYY-MM-DD>",
	Short: "构建指定交易日的 MarketSnapshot（可选 FeatureSnapshot）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		date := args[0]
		db, repo, builder, err := wireMarket()
		if err != nil {
			return err
		}
		_ = db
		builder.CoverageThreshold = mkThreshold
		builder.MaxGappedCodes = mkMaxGap
		builder.PriceAdjustment = mkAdj

		univ := pickUniverse(mkUniverse)
		if mkCodes != "" {
			list := strings.FieldsFunc(mkCodes, func(r rune) bool { return r == ',' || r == ' ' })
			univ.RequiredCodes = append(univ.RequiredCodes, list...)
			univ.MinIpoDays = 0
			univ.ExcludeST = false
			univ.ExcludeSuspended = false
			univ.ExcludeDelisted = false
		}
		s, err := builder.Build(date, univ)
		if err != nil {
			return fmt.Errorf("build snapshot: %w", err)
		}
		// 先保存 (未冻结，可重复覆盖)
		if err := repo.SaveMarketSnapshot(s); err != nil {
			return err
		}
		// 只有 ready 才默认冻结
		if s.Status == marketsnapshot.StatusReady {
			if err := repo.FreezeMarketSnapshot(s.ID); err != nil {
				return err
			}
			s.Frozen = true
		}
		printMarket(s)
		return nil
	},
}

var marketFreezeCmd = &cobra.Command{
	Use:   "freeze <id>",
	Short: "冻结一个 MarketSnapshot（之后任何修改都会失败）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, repo, _, err := wireMarket()
		if err != nil {
			return err
		}
		if err := repo.FreezeMarketSnapshot(args[0]); err != nil {
			return err
		}
		s, err := repo.LoadMarketSnapshot(args[0], false)
		if err != nil {
			return err
		}
		printMarket(s)
		return nil
	},
}

var marketShowCmd = &cobra.Command{
	Use:   "show ([date] [universe] | <id>)",
	Short: "查看 snapshot；传 <id> 或 (date, universe)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, repo, _, err := wireMarket()
		if err != nil {
			return err
		}
		var s *marketsnapshot.MarketSnapshot
		if len(args) == 1 {
			s, err = repo.LoadMarketSnapshot(args[0], true)
		} else {
			s, err = repo.FindMarketSnapshot(args[0], args[1], mkAdj)
		}
		if err != nil {
			return err
		}
		printMarket(s)
		return nil
	},
}

var marketListCmd = &cobra.Command{
	Use:   "list [status]",
	Short: "列所有 MarketSnapshot（按 status 可选过滤）",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, repo, _, err := wireMarket()
		if err != nil {
			return err
		}
		status := ""
		if len(args) == 1 {
			status = args[0]
		}
		list, err := repo.ListMarketSnapshots("", "", status)
		if err != nil {
			return err
		}
		for _, s := range list {
			fmt.Printf("%s date=%s univ=%s status=%s frozen=%v coverage=%.1f%% ready=%d/%d\n",
				s.ID, s.SnapshotDate, s.Universe.Name, s.Status, s.Frozen,
				s.CoveragePct*100, s.ReadyKlineCodes, s.ExpectedKlineCodes)
		}
		return nil
	},
}

var marketFeaturesCmd = &cobra.Command{
	Use:   "features <snapshot_id>",
	Short: "在一个已 ready 的 MarketSnapshot 上构建 FeatureSnapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, repo, b, err := wireMarket()
		if err != nil {
			return err
		}
		ms, err := repo.LoadMarketSnapshot(args[0], true)
		if err != nil {
			return err
		}
		if ms.Status != marketsnapshot.StatusReady {
			return fmt.Errorf("market snapshot %s status=%s != ready", ms.ID, ms.Status)
		}
		fe := marketsnapshotrepo.NewSQLiteFeatureEngine(db)
		fs, err := b.BuildFeatureSnapshot(ms, nil, fe)
		if err != nil {
			return fmt.Errorf("build feature: %w", err)
		}
		if err := repo.SaveFeatureSnapshot(fs); err != nil {
			return err
		}
		fmt.Printf("FeatureSnapshot id=%s rows=%d features=%d codes=%d hash=%s\n",
			fs.ID, fs.RowsWritten, fs.FeatureTotal, len(fs.Values), fs.ContentHash[:12])
		return nil
	},
}

var marketReportCmd = &cobra.Command{
	Use:   "report <id>",
	Short: "导出 snapshot 的 JSON 报告（含所有代码水位 + feature 统计）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, repo, _, err := wireMarket()
		if err != nil {
			return err
		}
		ms, err := repo.LoadMarketSnapshot(args[0], true)
		if err != nil {
			return err
		}
		fss, err := repo.ListFeatureSnapshots(ms.ID)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"market_snapshot":    ms,
			"feature_snapshots":  fss,
			"ready_for_scan":     ms.IsReady(),
			"fail_closed_reason": failClosedReason(ms),
		})
	},
}

func init() {
	marketCmd.AddCommand(marketBuildCmd, marketFreezeCmd, marketShowCmd, marketListCmd, marketFeaturesCmd, marketReportCmd)
	marketBuildCmd.Flags().StringVar(&mkAdj, "adj", "forward", "价格口径: raw/forward/backward")
	marketBuildCmd.Flags().StringVar(&mkUniverse, "universe", "universe_usable", "股票池: universe_usable / universe_csi800 / universe_all_a")
	marketBuildCmd.Flags().Float64Var(&mkThreshold, "coverage", 0.99, "ready 覆盖阈值 (0,1]")
	marketBuildCmd.Flags().IntVar(&mkMaxGap, "max-gapped", 50, "最多允许多少只股票有 gap_days > 0")
	marketBuildCmd.Flags().StringVar(&mkCodes, "codes", "", "指定代码列表，用逗号分隔；会覆盖 universe 过滤")
	marketShowCmd.Flags().StringVar(&mkAdj, "adj", "forward", "价格口径")
	rootCmd.AddCommand(marketCmd)
}

func wireMarket() (*storage.Storage, marketsnapshot.Repository, *marketsnapshot.Builder, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	db, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := db.Migrate(); err != nil {
		return nil, nil, nil, err
	}
	repo, err := marketsnapshotrepo.New(db)
	if err != nil {
		return nil, nil, nil, err
	}
	up := marketsnapshotrepo.NewSQLiteUniverseProvider(db)
	wp := marketsnapshotrepo.NewSQLiteWatermarkProvider(db)
	cal := marketsnapshotrepo.NewSQLiteTradingCalendar(db)
	b := marketsnapshot.NewBuilder(up, wp, cal)
	return db, repo, b, nil
}

func pickUniverse(name string) marketsnapshot.UniverseDefinition {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "universe_csi800":
		return marketsnapshot.DefaultUniverseCSI800()
	case "universe_all_a":
		return marketsnapshot.DefaultUniverseAllA()
	default:
		return marketsnapshot.DefaultUniverseUsable()
	}
}

func printMarket(s *marketsnapshot.MarketSnapshot) {
	fmt.Printf("MarketSnapshot id=%s\n", s.ID)
	fmt.Printf("  date        = %s\n", s.SnapshotDate)
	fmt.Printf("  universe    = %s (%s)\n", s.Universe.Name, s.Universe.Description)
	fmt.Printf("  price_adj   = %s\n", s.PriceAdjustment)
	fmt.Printf("  coverage    = %.2f%% (%d / %d kline ready)\n",
		s.CoveragePct*100, s.ReadyKlineCodes, s.ExpectedKlineCodes)
	fmt.Printf("  quote/fin/xdxr = %d / %d / %d ready\n", s.ReadyQuoteCodes, s.ReadyFinanceCodes, s.ReadyXdxrCodes)
	fmt.Printf("  status      = %s  frozen=%v  ready_for_scan=%v\n", s.Status, s.Frozen, s.IsReady())
	fmt.Printf("  reason      = %s\n", s.ReadinessReason)
	fmt.Printf("  universe_hash = %s\n", s.UniverseHash)
	fmt.Printf("  content_hash  = %s\n", s.ContentHash)
	if len(s.Codes) > 0 {
		var gaps []marketsnapshot.CodeStatus
		for _, c := range s.Codes {
			if c.GapDays > 0 {
				gaps = append(gaps, c)
			}
		}
		fmt.Printf("  gap_codes   = %d (展示前 10)\n", len(gaps))
		for i := 0; i < len(gaps) && i < 10; i++ {
			fmt.Printf("    %s last=%s gap=%d status=%s err=%s\n",
				gaps[i].Code, gaps[i].KlineLastDate, gaps[i].GapDays, gaps[i].SecurityStatus, gaps[i].LastError)
		}
	}
}

func failClosedReason(s *marketsnapshot.MarketSnapshot) string {
	if s.IsReady() {
		return ""
	}
	if !s.Frozen {
		return "snapshot 尚未冻结，需先 `tongstock market freeze " + s.ID + "`"
	}
	return "status=" + s.Status + "; readiness_reason=" + s.ReadinessReason
}
