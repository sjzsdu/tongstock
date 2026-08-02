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

func init() { automationCmd.AddCommand(automationRunCmd); rootCmd.AddCommand(automationCmd) }
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
