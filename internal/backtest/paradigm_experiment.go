package backtest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/trading"
)

// ParadigmExperimentExecutor 从绑定的不可变快照执行固定切分或 Walk-Forward 实验。
type ParadigmExperimentExecutor struct {
	SnapshotStore *paradigm.DatasetSnapshotStore
	Paradigm      *paradigms.Paradigm
}

type SegmentResult struct {
	Window  int                     `json:"window"`
	Segment DataSegment             `json:"segment"`
	Range   TimeSegment             `json:"range"`
	Result  *ParadigmBacktestResult `json:"result"`
}

type splitArtifact struct {
	Type        SplitType              `json:"type"`
	Fixed       *SplitResult           `json:"fixed,omitempty"`
	WalkForward *WalkForwardResult     `json:"walk_forward,omitempty"`
	Segments    []segmentArtifactRange `json:"segments"`
}

type segmentArtifactRange struct {
	Window  int         `json:"window"`
	Segment DataSegment `json:"segment"`
	Range   TimeSegment `json:"range"`
}

type executionManifest struct {
	SnapshotID        string                           `json:"snapshot_id"`
	SnapshotHash      string                           `json:"snapshot_hash"`
	KlineManifests    []paradigm.KlineSnapshotManifest `json:"kline_manifests"`
	ParadigmID        string                           `json:"paradigm_id"`
	LockedParamsHash  string                           `json:"locked_params_hash"`
	ConfigHash        string                           `json:"config_hash"`
	SelectionMode     string                           `json:"selection_mode"`
	SelectionSegments []DataSegment                    `json:"selection_segments"`
	ExcludedSegment   DataSegment                      `json:"excluded_segment"`
	TransactionHash   string                           `json:"transaction_hash"`
}

func (e *ParadigmExperimentExecutor) Execute(ctx context.Context, exp *experiment.Experiment) (experiment.MetricSet, []experiment.Artifact, error) {
	if e == nil || e.SnapshotStore == nil {
		return experiment.MetricSet{}, nil, fmt.Errorf("snapshot store is required")
	}
	if e.Paradigm == nil || strings.TrimSpace(e.Paradigm.StockCode) == "" {
		return experiment.MetricSet{}, nil, fmt.Errorf("paradigm with stock code is required")
	}
	if exp == nil || exp.ID == "" || exp.Config.DataSnapshotID == "" {
		return experiment.MetricSet{}, nil, fmt.Errorf("experiment and snapshot ID are required")
	}
	bound, err := e.SnapshotStore.GetBoundSnapshots(exp.ID)
	if err != nil {
		return experiment.MetricSet{}, nil, fmt.Errorf("read experiment snapshot binding: %w", err)
	}
	if !containsString(bound, exp.Config.DataSnapshotID) {
		return experiment.MetricSet{}, nil, fmt.Errorf("experiment %s is not bound to snapshot %s", exp.ID, exp.Config.DataSnapshotID)
	}
	if err := e.SnapshotStore.VerifyContent(exp.Config.DataSnapshotID); err != nil {
		return experiment.MetricSet{}, nil, fmt.Errorf("verify frozen snapshot: %w", err)
	}
	snapshot, err := e.SnapshotStore.GetByID(exp.Config.DataSnapshotID)
	if err != nil {
		return experiment.MetricSet{}, nil, fmt.Errorf("load frozen snapshot: %w", err)
	}
	frozen, err := e.SnapshotStore.GetFrozenKlines(
		exp.Config.DataSnapshotID, e.Paradigm.StockCode, exp.Config.KType)
	if err != nil {
		return experiment.MetricSet{}, nil, err
	}
	board := trading.Board(exp.Config.Board)
	if board == trading.BoardUnknown {
		board = BoardForCode(e.Paradigm.StockCode)
	}
	bars := MarketBarsFromSnapshot(frozen, board)
	dates := make([]time.Time, len(bars))
	for i := range bars {
		dates[i] = bars[i].Date
	}

	// 参数在任何测试段执行前锁定。当前范式不做自动调参，因此测试集没有参数选择通道。
	lockedParamsHash, err := lockedParadigmHash(e.Paradigm, exp.Config.StrategyParams)
	if err != nil {
		return experiment.MetricSet{}, nil, err
	}
	executionConfig := executionConfigFromExperiment(exp.Config, board)

	splitInfo, segments, err := e.executeSplits(ctx, exp.Config.SplitConfig, dates, bars, executionConfig)
	if err != nil {
		return experiment.MetricSet{}, nil, err
	}
	testResults := filterSegmentResults(segments, SegmentTest)
	if len(testResults) == 0 {
		return experiment.MetricSet{}, nil, fmt.Errorf("experiment produced no out-of-sample test segments")
	}
	metrics := aggregateMetrics(testResults)

	transactions := make([]struct {
		Window     int              `json:"window"`
		Segment    DataSegment      `json:"segment"`
		Signals    []SignalRecord   `json:"signals"`
		Fills      []trading.Fill   `json:"fills"`
		Rejections []Rejection      `json:"rejections"`
		Trades     []CompletedTrade `json:"trades"`
	}, len(segments))
	equity := make([]struct {
		Window  int           `json:"window"`
		Segment DataSegment   `json:"segment"`
		Points  []EquityPoint `json:"points"`
	}, len(segments))
	segmentMetrics := make([]struct {
		Window  int                  `json:"window"`
		Segment DataSegment          `json:"segment"`
		Metrics experiment.MetricSet `json:"metrics"`
	}, len(segments))
	for i, segment := range segments {
		transactions[i].Window = segment.Window
		transactions[i].Segment = segment.Segment
		transactions[i].Signals = segment.Result.Signals
		transactions[i].Fills = segment.Result.Fills
		transactions[i].Rejections = segment.Result.Rejections
		transactions[i].Trades = segment.Result.Trades
		equity[i].Window = segment.Window
		equity[i].Segment = segment.Segment
		equity[i].Points = segment.Result.EquityCurve
		segmentMetrics[i].Window = segment.Window
		segmentMetrics[i].Segment = segment.Segment
		segmentMetrics[i].Metrics = aggregateMetrics([]SegmentResult{segment})
	}
	transactionJSON, err := json.Marshal(transactions)
	if err != nil {
		return experiment.MetricSet{}, nil, err
	}
	manifest := executionManifest{
		SnapshotID: exp.Config.DataSnapshotID, SnapshotHash: snapshot.ContentHash,
		KlineManifests: snapshot.KlineManifests, ParadigmID: e.Paradigm.ID,
		LockedParamsHash: lockedParamsHash, ConfigHash: exp.ConfigHash,
		SelectionMode: "fixed_pre_registered", SelectionSegments: nonTestSegments(segments),
		ExcludedSegment: SegmentTest, TransactionHash: sha256Hex(transactionJSON),
	}

	artifacts := make([]experiment.Artifact, 0, 5)
	for _, item := range []struct {
		kind experiment.ArtifactType
		name string
		data any
	}{
		{experiment.ArtifactSplit, "time_split", splitInfo},
		{experiment.ArtifactMetrics, "segment_metrics", segmentMetrics},
		{experiment.ArtifactFills, "transactions", json.RawMessage(transactionJSON)},
		{experiment.ArtifactEquity, "equity_curves", equity},
		{experiment.ArtifactManifest, "reproducibility_manifest", manifest},
	} {
		artifact, err := newJSONArtifact(item.kind, item.name, item.data)
		if err != nil {
			return experiment.MetricSet{}, nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return metrics, artifacts, nil
}

func (e *ParadigmExperimentExecutor) executeSplits(
	ctx context.Context,
	ref experiment.SplitConfigRef,
	dates []time.Time,
	bars []MarketBar,
	cfg ParadigmExecutionConfig,
) (*splitArtifact, []SegmentResult, error) {
	switch SplitType(ref.Type) {
	case SplitFixed:
		minTrain := ref.MinTrainSize
		if minTrain <= 0 {
			minTrain = 1
		}
		splitter, err := NewTimeSeriesSplitter(TimeSeriesSplitConfig{
			Type: SplitFixed, TrainRatio: ref.TrainRatio, ValidRatio: ref.ValidRatio,
			EmbargoDays: ref.EmbargoDays, PurgeDays: ref.PurgeDays, MinTrainSize: minTrain,
		})
		if err != nil {
			return nil, nil, err
		}
		split, err := splitter.Split(dates)
		if err != nil {
			return nil, nil, err
		}
		segments, err := executeSplitSegments(ctx, e.Paradigm, bars, cfg, 0, split)
		if err != nil {
			return nil, nil, err
		}
		info := &splitArtifact{Type: SplitFixed, Fixed: split}
		info.Segments = segmentRanges(segments)
		return info, segments, nil
	case SplitWalkForward:
		splitter, err := NewWalkForwardSplitter(WalkForwardConfig{
			Windows: ref.Windows, TrainWindowDays: ref.TrainWindowDays,
			ValidWindowDays: ref.ValidWindowDays, TestWindowDays: ref.TestWindowDays,
			StepDays: ref.StepDays, EmbargoDays: ref.EmbargoDays, PurgeDays: ref.PurgeDays,
		})
		if err != nil {
			return nil, nil, err
		}
		walk, err := splitter.SplitWalkForward(dates)
		if err != nil {
			return nil, nil, err
		}
		var segments []SegmentResult
		for _, window := range walk.Windows {
			windowSegments, err := executeSplitSegments(ctx, e.Paradigm, bars, cfg, window.Index, &window.Split)
			if err != nil {
				return nil, nil, fmt.Errorf("execute window %d: %w", window.Index, err)
			}
			segments = append(segments, windowSegments...)
		}
		info := &splitArtifact{Type: SplitWalkForward, WalkForward: walk}
		info.Segments = segmentRanges(segments)
		return info, segments, nil
	default:
		return nil, nil, fmt.Errorf("unsupported split type %q", ref.Type)
	}
}

func executeSplitSegments(ctx context.Context, p *paradigms.Paradigm, bars []MarketBar,
	base ParadigmExecutionConfig, window int, split *SplitResult) ([]SegmentResult, error) {
	ranges := []struct {
		kind  DataSegment
		value *TimeSegment
	}{
		{SegmentTrain, &split.Train},
		{SegmentValid, split.Valid},
		{SegmentTest, &split.Test},
	}
	results := make([]SegmentResult, 0, len(ranges))
	for _, item := range ranges {
		if item.value == nil {
			continue
		}
		cfg := base
		cfg.EvaluationStart = item.value.Start
		cfg.EvaluationEnd = item.value.End
		result, err := RunParadigm(ctx, p, bars, cfg)
		if err != nil {
			return nil, fmt.Errorf("%s segment: %w", item.kind, err)
		}
		results = append(results, SegmentResult{
			Window: window, Segment: item.kind, Range: *item.value, Result: result,
		})
	}
	return results, nil
}

func executionConfigFromExperiment(config experiment.ExperimentConfig, board trading.Board) ParadigmExecutionConfig {
	constraints := trading.DefaultTradingConstraints()
	constraints.Board = board
	constraints.EnableT1 = config.EnableT1
	constraints.EnablePriceLimit = config.EnablePriceLimit
	cost := trading.CostModel{
		CommissionRate: config.CommissionRate, MinCommission: config.MinCommission,
		StampDutyRate: config.StampDutyRate, TransferFeeRate: config.TransferFeeRate,
		SlippageBps: config.SlippageBps, EnableStampDuty: config.StampDutyRate > 0,
	}
	return ParadigmExecutionConfig{
		InitialCash: config.InitialCash, PositionSize: config.MaxPositionSize,
		Constraints: constraints, CostModel: cost,
	}
}

// BoardForCode 基于 A 股代码推断交易板块。
func BoardForCode(code string) trading.Board {
	switch {
	case strings.HasPrefix(code, "300"), strings.HasPrefix(code, "301"):
		return trading.BoardChiNext
	case strings.HasPrefix(code, "688"), strings.HasPrefix(code, "689"):
		return trading.BoardSTAR
	case strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"):
		return trading.BoardBJ
	default:
		return trading.BoardMain
	}
}

func aggregateMetrics(results []SegmentResult) experiment.MetricSet {
	metrics := experiment.MetricSet{Custom: map[string]float64{}}
	if len(results) == 0 {
		return metrics
	}
	var initial, wins, completed, gains, losses, costs, rejections float64
	for _, segment := range results {
		result := segment.Result
		initial += result.InitialCash
		metrics.GrossPnL += result.GrossPnL
		metrics.NetPnL += result.NetPnL
		costs += result.TotalCost
		rejections += float64(len(result.Rejections))
		drawdown := maxDrawdown(result.EquityCurve)
		if drawdown > metrics.MaxDrawdown {
			metrics.MaxDrawdown = drawdown
		}
		for _, trade := range result.Trades {
			completed++
			if trade.NetPnL > 0 {
				wins++
				gains += trade.NetPnL
			} else {
				losses -= trade.NetPnL
			}
		}
	}
	if initial > 0 {
		metrics.TotalReturn = metrics.NetPnL / initial
	}
	metrics.TotalTrades = int(completed)
	if completed > 0 {
		metrics.WinRate = wins / completed
	}
	if losses > 0 {
		metrics.ProfitFactor = gains / losses
	}
	metrics.Custom["segment_count"] = float64(len(results))
	metrics.Custom["total_cost"] = costs
	metrics.Custom["rejection_count"] = rejections
	return metrics
}

func maxDrawdown(points []EquityPoint) float64 {
	var peak, maximum float64
	for _, point := range points {
		if point.Equity > peak {
			peak = point.Equity
		}
		if peak > 0 {
			drawdown := (peak - point.Equity) / peak
			if drawdown > maximum {
				maximum = drawdown
			}
		}
	}
	return maximum
}

func lockedParadigmHash(p *paradigms.Paradigm, params map[string]interface{}) (string, error) {
	payload := struct {
		BuyConds  []paradigms.Condition    `json:"buy_conditions"`
		SellConds paradigms.SellConditions `json:"sell_conditions"`
		Params    map[string]interface{}   `json:"strategy_params"`
	}{BuyConds: p.BuyConds, SellConds: p.SellConds, Params: params}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal locked paradigm parameters: %w", err)
	}
	return sha256Hex(data), nil
}

func newJSONArtifact(kind experiment.ArtifactType, name string, value any) (experiment.Artifact, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return experiment.Artifact{}, fmt.Errorf("marshal artifact %s: %w", name, err)
	}
	return experiment.Artifact{
		Type: kind, Name: name, Content: content, ContentHash: sha256Hex(content),
	}, nil
}

func sha256Hex(value []byte) string {
	hash := sha256.Sum256(value)
	return fmt.Sprintf("%x", hash[:])
}

func filterSegmentResults(results []SegmentResult, segment DataSegment) []SegmentResult {
	filtered := make([]SegmentResult, 0, len(results))
	for _, result := range results {
		if result.Segment == segment {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func segmentRanges(results []SegmentResult) []segmentArtifactRange {
	ranges := make([]segmentArtifactRange, len(results))
	for i, result := range results {
		ranges[i] = segmentArtifactRange{
			Window: result.Window, Segment: result.Segment, Range: result.Range,
		}
	}
	return ranges
}

func nonTestSegments(results []SegmentResult) []DataSegment {
	var segments []DataSegment
	for _, candidate := range []DataSegment{SegmentTrain, SegmentValid} {
		for _, result := range results {
			if result.Segment == candidate {
				segments = append(segments, candidate)
				break
			}
		}
	}
	return segments
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
