package main

import (
	"encoding/json"
	"fmt"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type acceptanceReport struct {
	VerifiedAt          time.Time `json:"verified_at"`
	Database            string    `json:"database"`
	MarketSnapshotID    string    `json:"market_snapshot_id"`
	SnapshotDate        string    `json:"snapshot_date"`
	SnapshotHash        string    `json:"snapshot_hash"`
	FeatureSnapshotID   string    `json:"feature_snapshot_id"`
	FeatureHash         string    `json:"feature_hash"`
	UniverseStocks      int       `json:"universe_stocks"`
	FeatureValues       int       `json:"feature_values"`
	DiscoveryResearch   int       `json:"discovery_research"`
	NamedResearch       int       `json:"named_research"`
	ValidationEvidence  int       `json:"validation_evidence"`
	RegisteredMethods   int       `json:"registered_methods"`
	EligibleMethods     int       `json:"eligible_methods"`
	SelectionRunID      string    `json:"selection_run_id"`
	SelectionCandidates int       `json:"selection_candidates"`
	PositionRunID       string    `json:"position_run_id"`
	PositionDecisions   int       `json:"position_decisions"`
	AutomationJobID     string    `json:"automation_job_id"`
	AutomationStatus    string    `json:"automation_status"`
	Limitations         []string  `json:"limitations"`
}

var acceptanceCmd = &cobra.Command{Use: "acceptance", Short: "真实数据端到端验收"}
var acceptanceVerifyCmd = &cobra.Command{Use: "verify", Short: "验证正式数据库全链路血缘；缺失即失败", RunE: func(cmd *cobra.Command, args []string) error {
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
	sn, _ := marketsnapshotrepo.New(s)
	var acceptedSnapshotID string
	_ = s.DB().QueryRow(`SELECT snapshot_id FROM automation_job_run WHERE status='completed' ORDER BY started_at_ns DESC LIMIT 1`).Scan(&acceptedSnapshotID)
	var m *marketsnapshot.MarketSnapshot
	if acceptedSnapshotID != "" {
		m, err = sn.LoadMarketSnapshot(acceptedSnapshotID, true)
		if err != nil {
			return err
		}
	}
	if m == nil {
		return fmt.Errorf("acceptance failed: no frozen ready MarketSnapshot")
	}
	mh, _ := marketsnapshot.ComputeContentHash(m)
	if mh != m.ContentHash {
		return fmt.Errorf("acceptance failed: market snapshot hash mismatch")
	}
	fsList, _ := sn.ListFeatureSnapshots(m.ID)
	if len(fsList) == 0 {
		return fmt.Errorf("acceptance failed: no FeatureSnapshot")
	}
	f, err := sn.LoadFeatureSnapshot(fsList[0].ID, true)
	if err != nil {
		return err
	}
	fh, _ := marketsnapshot.ComputeFeatureContentHash(f)
	if fh != f.ContentHash || !f.LeakChecked {
		return fmt.Errorf("acceptance failed: feature hash/leak gate")
	}
	r := acceptanceReport{VerifiedAt: time.Now().UTC(), Database: cfg.Database.DSN, MarketSnapshotID: m.ID, SnapshotDate: m.SnapshotDate, SnapshotHash: m.ContentHash, FeatureSnapshotID: f.ID, FeatureHash: f.ContentHash, UniverseStocks: len(f.Values), FeatureValues: f.RowsWritten, Limitations: []string{"验收证明软件链路与血缘，不承诺未来收益", "当前持仓为空时只能验证空决策，不能伪造真实持仓场景"}}
	db := s.DB()
	_ = db.QueryRow(`SELECT COUNT(*) FROM discovery_research_trace`).Scan(&r.DiscoveryResearch)
	_ = db.QueryRow(`SELECT COUNT(*) FROM method_research_artifact`).Scan(&r.NamedResearch)
	_ = db.QueryRow(`SELECT COUNT(*) FROM validation_evidence_artifact`).Scan(&r.ValidationEvidence)
	_ = db.QueryRow(`SELECT COUNT(*) FROM investment_method_registry`).Scan(&r.RegisteredMethods)
	_ = db.QueryRow(`SELECT COUNT(*) FROM investment_method_registry WHERE status IN ('verified','observing')`).Scan(&r.EligibleMethods)
	_ = db.QueryRow(`SELECT run_id,candidate_count FROM daily_selection_run WHERE snapshot_id=? ORDER BY created_at_ns DESC LIMIT 1`, m.ID).Scan(&r.SelectionRunID, &r.SelectionCandidates)
	_ = db.QueryRow(`SELECT run_id,json_array_length(json_extract(decision_json,'$.decisions')) FROM position_decision_run WHERE snapshot_id=? ORDER BY created_at_ns DESC LIMIT 1`, m.ID).Scan(&r.PositionRunID, &r.PositionDecisions)
	_ = db.QueryRow(`SELECT job_id,status FROM automation_job_run WHERE snapshot_id=? ORDER BY started_at_ns DESC LIMIT 1`, m.ID).Scan(&r.AutomationJobID, &r.AutomationStatus)
	if r.DiscoveryResearch == 0 || r.NamedResearch == 0 || r.ValidationEvidence == 0 || r.RegisteredMethods == 0 || r.SelectionRunID == "" || r.PositionRunID == "" || r.AutomationStatus != "completed" {
		return fmt.Errorf("acceptance failed: incomplete lineage: %+v", r)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}}

func init() { acceptanceCmd.AddCommand(acceptanceVerifyCmd); rootCmd.AddCommand(acceptanceCmd) }
