package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
)

var selectionFeatureID string

var selectionCmd = &cobra.Command{
	Use:   "select",
	Short: "在冻结真实数据上生成可审计的每日买入候选",
}

var selectionRunCmd = &cobra.Command{
	Use:   "run [market_snapshot_id]",
	Short: "运行全部可信方法；省略 snapshot 时使用最新 ready 快照",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, snapshots, engine, _, err := wireSelection()
		if err != nil {
			return err
		}
		defer store.Close()
		id := ""
		if len(args) > 0 {
			id = args[0]
		} else {
			items, listErr := snapshots.ListMarketSnapshots("", "", "ready")
			if listErr != nil {
				return listErr
			}
			for _, item := range items {
				if item.Frozen {
					id = item.ID
					break
				}
			}
			if id == "" {
				return fmt.Errorf("没有 frozen + ready 的 MarketSnapshot；先运行 tongstock market build/features")
			}
		}
		run, err := engine.Run(cmd.Context(), selection.Request{MarketSnapshotID: id, FeatureSnapshotID: selectionFeatureID})
		if err != nil {
			return err
		}
		return printSelection(run)
	},
}

var selectionShowCmd = &cobra.Command{
	Use: "show <run_id>", Short: "查看一次不可变选股运行", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, _, runs, err := wireSelection()
		if err != nil {
			return err
		}
		defer store.Close()
		run, err := runs.Get(cmd.Context(), args[0], "")
		if err != nil {
			return err
		}
		return printSelection(run)
	},
}

var selectionListCmd = &cobra.Command{
	Use: "list [date]", Short: "列出历史选股运行", Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, _, runs, err := wireSelection()
		if err != nil {
			return err
		}
		defer store.Close()
		date := ""
		if len(args) > 0 {
			date = args[0]
		}
		items, err := runs.List(context.Background(), date, "", 30)
		if err != nil {
			return err
		}
		for _, r := range items {
			fmt.Printf("%s date=%s candidates=%d buy=%d eligible=%d snapshot=%s\n", r.ID, r.SnapshotDate, r.CandidateCount, r.BuyCount, r.EligibleMethods, r.SnapshotID)
		}
		return nil
	},
}

func init() {
	selectionRunCmd.Flags().StringVar(&selectionFeatureID, "feature-snapshot", "", "指定 FeatureSnapshot，默认取绑定快照的最新 ready 版本")
	selectionCmd.AddCommand(selectionRunCmd, selectionShowCmd, selectionListCmd)
	rootCmd.AddCommand(selectionCmd)
}

func wireSelection() (*storage.Storage, *marketsnapshotrepo.SQLiteRepository, *selection.Engine, *selectionrepo.SQLiteRepository, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err = store.Migrate(); err != nil {
		store.Close()
		return nil, nil, nil, nil, err
	}
	snapshots, err := marketsnapshotrepo.New(store)
	if err != nil {
		store.Close()
		return nil, nil, nil, nil, err
	}
	methodsRepo, err := methodregistryrepo.New(store)
	if err != nil {
		store.Close()
		return nil, nil, nil, nil, err
	}
	runs, err := selectionrepo.New(store)
	if err != nil {
		store.Close()
		return nil, nil, nil, nil, err
	}
	engine, err := selection.NewEngine(snapshots, methodsRepo, runs)
	if err != nil {
		store.Close()
		return nil, nil, nil, nil, err
	}
	return store, snapshots, engine, runs, nil
}
func printSelection(run *selection.Run) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(run)
}
