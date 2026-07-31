package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
)

type paradigmBacktestRequest struct {
	ParadigmID       string                    `json:"paradigm_id"`
	SnapshotID       string                    `json:"snapshot_id,omitempty"`
	StartDate        string                    `json:"start_date,omitempty"`
	EndDate          string                    `json:"end_date,omitempty"`
	KType            uint8                     `json:"ktype,omitempty"`
	Split            experiment.SplitConfigRef `json:"split,omitempty"`
	InitialCash      float64                   `json:"initial_cash,omitempty"`
	CommissionRate   float64                   `json:"commission_rate,omitempty"`
	MinCommission    float64                   `json:"min_commission,omitempty"`
	StampDutyRate    float64                   `json:"stamp_duty_rate,omitempty"`
	TransferFeeRate  float64                   `json:"transfer_fee_rate,omitempty"`
	SlippageBps      float64                   `json:"slippage_bps,omitempty"`
	MaxPositionSize  float64                   `json:"max_position_size,omitempty"`
	StopLossRatio    float64                   `json:"stop_loss_ratio,omitempty"`
	TakeProfitRatio  float64                   `json:"take_profit_ratio,omitempty"`
	EnableT1         *bool                     `json:"enable_t_1,omitempty"`
	EnablePriceLimit *bool                     `json:"enable_price_limit,omitempty"`
}

type paradigmBacktestResponse struct {
	ParadigmID      string                `json:"paradigm_id"`
	StockCode       string                `json:"stock_code"`
	ExperimentID    string                `json:"experiment_id"`
	RunID           string                `json:"run_id"`
	SnapshotID      string                `json:"snapshot_id"`
	ConfigHash      string                `json:"config_hash"`
	ResultHash      string                `json:"result_hash"`
	Metrics         *experiment.MetricSet `json:"metrics"`
	SegmentedMetric json.RawMessage       `json:"segmented_metrics"`
	Artifacts       []experiment.Artifact `json:"artifacts"`
}

type paradigmExperimentEvidence struct {
	Paradigm   *paradigms.Paradigm         `json:"paradigm"`
	Experiment *experiment.Experiment      `json:"experiment"`
	Runs       []*experiment.ExperimentRun `json:"runs"`
}

func (s *Server) handleParadigmBacktest(c *gin.Context) {
	if s.paradigmStore == nil || s.paradigmSnapshots == nil || s.experimentRegistry == nil || s.storage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "paradigm experiment storage is not initialized"})
		return
	}
	var req paradigmBacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backtest request: " + err.Error()})
		return
	}
	req.ParadigmID = strings.TrimSpace(req.ParadigmID)
	if req.ParadigmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paradigm_id is required"})
		return
	}
	p, err := s.paradigmStore.Get(req.ParadigmID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if len(p.BuyConds) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "paradigm has no executable buy conditions"})
		return
	}

	applyParadigmBacktestDefaults(&req)
	snapshotID, err := s.resolveParadigmSnapshot(p, req)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	config := experiment.ExperimentConfig{
		StrategyName: "paradigm", StrategyVersion: "1",
		DataSnapshotID: snapshotID, KType: req.KType,
		Board: string(backtest.BoardForCode(p.StockCode)), SplitConfig: req.Split,
		InitialCash: req.InitialCash, CommissionRate: req.CommissionRate,
		MinCommission: req.MinCommission, StampDutyRate: req.StampDutyRate,
		TransferFeeRate: req.TransferFeeRate, SlippageBps: req.SlippageBps,
		MaxPositionSize: req.MaxPositionSize, StopLossRatio: req.StopLossRatio,
		TakeProfitRatio: req.TakeProfitRatio, EnableT1: *req.EnableT1,
		EnablePriceLimit: *req.EnablePriceLimit,
		StrategyParams: map[string]interface{}{
			"paradigm_id": p.ID,
			"selection":   "pre_registered",
		},
	}
	exp := experiment.NewExperiment(
		fmt.Sprintf("paradigm-%s", p.ID),
		fmt.Sprintf("Production backtest for paradigm %s on frozen real K-line data", p.ID),
		config,
	)
	exp.CreatedBy = "api"
	exp.Tags = []string{"paradigm", p.StockCode, "production"}
	if err := s.experimentRegistry.Create(exp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create experiment: " + err.Error()})
		return
	}
	if err := s.paradigmSnapshots.BindExperiment(exp.ID, []string{snapshotID}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "bind frozen snapshot: " + err.Error(), "experiment_id": exp.ID,
		})
		return
	}
	run, err := experiment.NewExperimentRunner(s.experimentRegistry).Run(
		c.Request.Context(), exp, &backtest.ParadigmExperimentExecutor{
			SnapshotStore: s.paradigmSnapshots,
			Paradigm:      p,
		},
	)
	if err != nil {
		body := gin.H{"error": err.Error(), "experiment_id": exp.ID, "snapshot_id": snapshotID}
		if run != nil {
			body["run_id"] = run.ID
		}
		c.JSON(http.StatusUnprocessableEntity, body)
		return
	}
	c.JSON(http.StatusCreated, paradigmBacktestResponse{
		ParadigmID: p.ID, StockCode: p.StockCode, ExperimentID: exp.ID,
		RunID: run.ID, SnapshotID: snapshotID, ConfigHash: exp.ConfigHash,
		ResultHash: run.ResultHash, Metrics: run.Metrics,
		SegmentedMetric: artifactContent(run.Artifacts, "segment_metrics"),
		Artifacts:       run.Artifacts,
	})
}

func applyParadigmBacktestDefaults(req *paradigmBacktestRequest) {
	if req.KType == 0 {
		req.KType = 9
	}
	if req.Split.Type == "" {
		req.Split = experiment.SplitConfigRef{
			Type: string(backtest.SplitFixed), TrainRatio: 0.6, ValidRatio: 0.2,
			EmbargoDays: 2, PurgeDays: 2, MinTrainSize: 60,
		}
	}
	if req.InitialCash <= 0 {
		req.InitialCash = 100000
	}
	if req.CommissionRate <= 0 {
		req.CommissionRate = 0.00025
	}
	if req.MinCommission <= 0 {
		req.MinCommission = 5
	}
	if req.StampDutyRate <= 0 {
		req.StampDutyRate = 0.0005
	}
	if req.TransferFeeRate <= 0 {
		req.TransferFeeRate = 0.00001
	}
	if req.SlippageBps <= 0 {
		req.SlippageBps = 10
	}
	if req.MaxPositionSize <= 0 {
		req.MaxPositionSize = 0.5
	}
	if req.EnableT1 == nil {
		value := true
		req.EnableT1 = &value
	}
	if req.EnablePriceLimit == nil {
		value := true
		req.EnablePriceLimit = &value
	}
}

func (s *Server) resolveParadigmSnapshot(p *paradigms.Paradigm, req paradigmBacktestRequest) (string, error) {
	if id := strings.TrimSpace(req.SnapshotID); id != "" {
		snapshot, err := s.paradigmSnapshots.GetByID(id)
		if err != nil {
			return "", fmt.Errorf("load snapshot %s: %w", id, err)
		}
		if !containsStockCode(snapshot.Universe, p.StockCode) {
			return "", fmt.Errorf("snapshot %s does not contain stock %s", id, p.StockCode)
		}
		if err := s.paradigmSnapshots.VerifyContent(id); err != nil {
			return "", fmt.Errorf("snapshot %s is not a valid frozen data input: %w", id, err)
		}
		if _, err := s.paradigmSnapshots.GetFrozenKlines(id, p.StockCode, req.KType); err != nil {
			return "", err
		}
		return id, nil
	}

	start, end := strings.TrimSpace(req.StartDate), strings.TrimSpace(req.EndDate)
	if start == "" || end == "" {
		var availableStart, availableEnd sql.NullString
		err := s.storage.DB().QueryRow(`SELECT MIN(REPLACE(date, '-', '')), MAX(REPLACE(date, '-', ''))
			FROM kline WHERE code = ? AND ktype = ?`, p.StockCode, req.KType).
			Scan(&availableStart, &availableEnd)
		if err != nil {
			return "", fmt.Errorf("inspect real K-line range: %w", err)
		}
		if !availableStart.Valid || !availableEnd.Valid || availableStart.String == "" || availableEnd.String == "" {
			return "", fmt.Errorf("no real K-line data for %s with ktype %d", p.StockCode, req.KType)
		}
		if start == "" {
			start = availableStart.String
		}
		if end == "" {
			end = availableEnd.String
		}
	}
	now := time.Now()
	id := fmt.Sprintf("snapshot-api-%s-%d", p.StockCode, now.UnixNano())
	snapshot := &paradigm.DatasetSnapshot{
		ID: id, Version: "v1", Universe: []string{p.StockCode},
		DateRange: paradigm.DateRange{Start: start, End: end},
		Market:    "A", PriceAdjustment: paradigm.PriceRaw,
		Description: fmt.Sprintf("API-created immutable K-line snapshot for paradigm %s", p.ID),
		CreatedAt:   now,
	}
	if err := s.paradigmSnapshots.CreateKlineSnapshot(snapshot, req.KType); err != nil {
		return "", fmt.Errorf("create frozen snapshot from real K-line data: %w", err)
	}
	return id, nil
}

func (s *Server) handleParadigmExperimentGet(c *gin.Context) {
	if s.experimentRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "experiment registry is not initialized"})
		return
	}
	exp, err := s.experimentRegistry.GetByID(c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	runs, err := s.experimentRegistry.ListRuns(exp.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"experiment": exp, "runs": runs})
}

func (s *Server) latestParadigmExperimentEvidence(paradigmID, requestedExperimentID string) (*paradigmExperimentEvidence, error) {
	if s.experimentRegistry == nil {
		return nil, fmt.Errorf("experiment registry is not initialized")
	}
	p, err := s.paradigmStore.Get(paradigmID)
	if err != nil {
		return nil, err
	}
	var exp *experiment.Experiment
	if requestedExperimentID != "" {
		exp, err = s.experimentRegistry.GetByID(requestedExperimentID)
		if err != nil {
			return nil, err
		}
		if paradigmIDFromExperiment(exp) != paradigmID {
			return nil, fmt.Errorf("experiment %s does not belong to paradigm %s", exp.ID, paradigmID)
		}
	} else {
		experiments, listErr := s.experimentRegistry.List()
		if listErr != nil {
			return nil, listErr
		}
		for _, candidate := range experiments {
			if paradigmIDFromExperiment(candidate) == paradigmID {
				exp = candidate
				break
			}
		}
		if exp == nil {
			return nil, fmt.Errorf("no persisted experiment evidence for paradigm %s", paradigmID)
		}
	}
	runs, err := s.experimentRegistry.ListRuns(exp.ID)
	if err != nil {
		return nil, err
	}
	return &paradigmExperimentEvidence{Paradigm: p, Experiment: exp, Runs: runs}, nil
}

func paradigmIDFromExperiment(exp *experiment.Experiment) string {
	if exp == nil || exp.Config.StrategyParams == nil {
		return ""
	}
	value, _ := exp.Config.StrategyParams["paradigm_id"].(string)
	return value
}

func artifactContent(artifacts []experiment.Artifact, name string) json.RawMessage {
	for _, artifact := range artifacts {
		if artifact.Name == name {
			return artifact.Content
		}
	}
	return json.RawMessage(`[]`)
}

func containsStockCode(codes []string, wanted string) bool {
	for _, code := range codes {
		if code == wanted {
			return true
		}
	}
	return false
}
