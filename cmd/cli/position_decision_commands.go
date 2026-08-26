package main

import (
	"encoding/json"
	"fmt"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/positiondecisionrepo"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"github.com/spf13/cobra"
	"os"
)

var positionFeatureID string
var positionCmd = &cobra.Command{Use: "position", Short: "基于真实持仓和冻结快照判断何时卖"}
var positionDecideCmd = &cobra.Command{Use: "decide [market_snapshot_id]", Short: "生成不可变持仓决策", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
	s, sn, e, _, err := wirePositionDecision()
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
	run, err := e.Run(cmd.Context(), positiondecision.Request{MarketSnapshotID: id, FeatureSnapshotID: positionFeatureID})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(run)
}}

func init() {
	positionDecideCmd.Flags().StringVar(&positionFeatureID, "feature-snapshot", "", "指定特征快照")
	positionCmd.AddCommand(positionDecideCmd)
	rootCmd.AddCommand(positionCmd)
}
func wirePositionDecision() (*storage.Storage, *marketsnapshotrepo.SQLiteRepository, *positiondecision.Engine, *positiondecisionrepo.SQLiteRepository, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	s, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err = s.Migrate(); err != nil {
		return nil, nil, nil, nil, err
	}
	sn, _ := marketsnapshotrepo.New(s)
	mr, _ := methodregistryrepo.New(s)
	rr, _ := positiondecisionrepo.New(s)
	ts, _ := trading.New(s)
	e, err := positiondecision.NewEngine(sn, ts, mr, rr)
	return s, sn, e, rr, err
}
