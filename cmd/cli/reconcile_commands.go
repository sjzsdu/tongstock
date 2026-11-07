package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/reconcile"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
)

// exitCodeError carries a process exit code out of RunE. Differing results are
// a finding (exit 1), not a usage error; a broken run is exit 2.
type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string { return e.msg }

var (
	reconcileDate string
	reconcileJSON bool
)

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "对账诊断命令（旁路，不写入正式结果）",
}

var reconcileAsOfCmd = &cobra.Command{
	Use:   "asof",
	Short: "同一交易日对比旧快照路径与 as-of 读取路径",
	Long: "在同一交易日分别跑「冻结快照 + 特征快照」与「kline_latest 的 date <= T 时点视图」，" +
		"比对候选代码集合、每只票的 action、score（容差 1e-9）与 content hash。" +
		"不写入 daily_selection_run / position_decision_run，生产读取路径不受影响。\n\n" +
		"退出码：0 一致（或无快照可对账），1 有差异，2 运行失败。",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	// main() prints the message once; let the exit code carry "differences
	// found" so scripts can branch on it without parsing text.
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		report, err := runReconcileAsOf(cmd.Context(), reconcileDate)
		if err != nil {
			return &exitCodeError{code: 2, msg: err.Error()}
		}
		if reconcileJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}
		} else {
			printReconcileReport(report)
		}
		if report.NoSnapshot || report.Consistent {
			return nil
		}
		return &exitCodeError{code: 1, msg: "对账发现差异"}
	},
}

func runReconcileAsOf(ctx context.Context, date string) (reconcile.Report, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return reconcile.Report{}, err
	}
	defer store.Close()

	snapshots, err := marketsnapshotrepo.New(store)
	if err != nil {
		return reconcile.Report{}, err
	}
	methodsRepo, err := methodregistryrepo.New(store)
	if err != nil {
		return reconcile.Report{}, err
	}
	runs, err := selectionrepo.New(store)
	if err != nil {
		return reconcile.Report{}, err
	}
	return reconcile.Run(ctx, reconcile.Deps{
		Store:      store,
		Snapshots:  snapshots,
		Methods:    methodsRepo,
		Universe:   marketsnapshotrepo.NewSQLiteUniverseProvider(store),
		StoredRuns: runs,
	}, date)
}

func printReconcileReport(report reconcile.Report) {
	if report.NoSnapshot {
		fmt.Printf("%s\n", report.Reason)
		return
	}
	status := "一致"
	if !report.Consistent {
		status = "有差异"
	}
	fmt.Printf("对账日 %s：%s\n", report.TradeDate, status)
	fmt.Printf("  快照路径   %s (snapshot=%s, feature=%s)\n", report.SnapshotPath, report.SnapshotID, report.FeatureSnapshotID)
	fmt.Printf("  as-of 路径 %s (data_version=%d)\n", report.AsOfPath, report.AsOfDataVersion)
	fmt.Printf("  as-of 覆盖 %d/%d (%.4f)，剔除无当日 bar 的票 %d 只\n",
		report.AsOfCoverage.Ready, report.AsOfCoverage.Total, report.AsOfCoverage.Ratio, len(report.DroppedCodes))
	if report.InputsDigest != "" {
		fmt.Printf("  inputs_digest %s\n", report.InputsDigest)
	}
	fmt.Printf("  content hash 一致=%t\n", report.ContentHashMatch)
	fmt.Printf("  候选：快照 %d 只，as-of %d 只\n", len(report.SnapshotCandidateCodes), len(report.AsOfCandidateCodes))
	diffs := report.DiffSummary()
	if len(diffs) == 0 {
		fmt.Println("  四项比对全部一致")
	} else {
		fmt.Println("  差异：")
		for _, d := range diffs {
			fmt.Printf("    - %s\n", d)
		}
	}
	for _, note := range report.Notes {
		fmt.Printf("  注：%s\n", note)
	}
}

func init() {
	reconcileAsOfCmd.Flags().StringVar(&reconcileDate, "date", "", "交易日 YYYY-MM-DD")
	reconcileAsOfCmd.Flags().BoolVar(&reconcileJSON, "json", false, "以 JSON 输出报告")
	_ = reconcileAsOfCmd.MarkFlagRequired("date")
	reconcileCmd.AddCommand(reconcileAsOfCmd)
	rootCmd.AddCommand(reconcileCmd)
}
