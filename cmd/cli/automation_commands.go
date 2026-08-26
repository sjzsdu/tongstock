package main

import (
	"encoding/json"
	"fmt"
	"github.com/sjzsdu/tongstock/internal/adapter/automationrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/positiondecisionrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/automation"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"github.com/spf13/cobra"
	"os"
)

var automationCmd = &cobra.Command{Use: "automation", Short: "每日真实数据选股、持仓判断和提醒闭环"}
var automationRunCmd = &cobra.Command{Use: "run [snapshot_id]", Short: "幂等运行一个冻结快照的每日闭环", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
	s, sn, o, err := wireAutomation()
	if err != nil {
		return err
	}
	defer s.Close()
	id := ""
	if len(args) > 0 {
		id = args[0]
	} else {
		xs, err := sn.ListMarketSnapshots("", "", "ready")
		if err != nil {
			return err
		}
		for _, x := range xs {
			if x.Frozen {
				id = x.ID
				break
			}
		}
	}
	if id == "" {
		return fmt.Errorf("没有 frozen + ready 市场快照")
	}
	j, err := o.Run(cmd.Context(), id)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(j)
}}

var automationUnlockCmd = &cobra.Command{
	Use:   "unlock <snapshot_id>",
	Short: "强制释放卡死的自动化任务锁（标记为 failed，下次 run 自动重试）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			cfg = config.DefaultConfig()
		}
		s, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
		if err != nil {
			return err
		}
		defer s.Close()
		if err = s.Migrate(); err != nil {
			return err
		}
		r, err := automationrepo.New(s)
		if err != nil {
			return err
		}
		unlocked, err := r.Unlock(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if !unlocked {
			fmt.Printf("快照 %s 没有 running 中的任务锁，无需解锁\n", args[0])
			return nil
		}
		fmt.Printf("已释放快照 %s 的任务锁（原任务标记为 failed），可重新运行 automation run\n", args[0])
		return nil
	},
}

func init() {
	automationCmd.AddCommand(automationRunCmd, automationUnlockCmd)
	rootCmd.AddCommand(automationCmd)
}
func wireAutomation() (*storage.Storage, *marketsnapshotrepo.SQLiteRepository, *automation.Orchestrator, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	s, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, nil, nil, err
	}
	if err = s.Migrate(); err != nil {
		return nil, nil, nil, err
	}
	sn, _ := marketsnapshotrepo.New(s)
	mr, _ := methodregistryrepo.New(s)
	sr, _ := selectionrepo.New(s)
	pr, _ := positiondecisionrepo.New(s)
	ar, _ := automationrepo.New(s)
	tr, _ := trading.New(s)
	se, _ := selection.NewEngine(sn, mr, sr)
	pe, _ := positiondecision.NewEngine(sn, tr, mr, pr)
	l, err := ledger.NewSQLiteSignalLedger(s)
	if err != nil {
		return nil, nil, nil, err
	}
	o, err := automation.New(se, pe, sn, l, ar)
	return s, sn, o, err
}
