package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/trading"
)

type evidenceSegmentMetric struct {
	Window  int                  `json:"window"`
	Segment string               `json:"segment"`
	Metrics experiment.MetricSet `json:"metrics"`
}

type evidenceTransactionSegment struct {
	Window     int                       `json:"window"`
	Segment    string                    `json:"segment"`
	Fills      []trading.Fill            `json:"fills"`
	Rejections []backtest.Rejection      `json:"rejections"`
	Trades     []backtest.CompletedTrade `json:"trades"`
}

type evidenceExecutionManifest struct {
	SnapshotID      string `json:"snapshot_id"`
	SnapshotHash    string `json:"snapshot_hash"`
	ParadigmID      string `json:"paradigm_id"`
	ConfigHash      string `json:"config_hash"`
	TransactionHash string `json:"transaction_hash"`
}

func unavailableParadigmEvidence(p *paradigms.Paradigm, reason string) *paradigms.EvidenceCard {
	return &paradigms.EvidenceCard{
		ParadigmID: p.ID, ParadigmName: p.Name, StockCode: p.StockCode, StockName: p.StockName,
		GeneratedAt: time.Now(), Available: false, PromotionEligible: false,
		UnavailableReasons: []string{reason},
		PromotionBlockers:  []string{"真实实验、冻结快照和交易级证据不完整，禁止晋级"},
		CounterEvidence:    []paradigms.CounterExample{},
		RiskFlags: []paradigms.RiskFlag{{
			Category: "evidence", Level: "critical", Message: reason,
			Mitigation: "先运行生产范式回测并保存完整实验制品",
		}},
	}
}

func buildParadigmEvidence(
	p *paradigms.Paradigm,
	exp *experiment.Experiment,
	run *experiment.ExperimentRun,
	snapshot *paradigm.DatasetSnapshot,
) (*paradigms.EvidenceCard, error) {
	card := &paradigms.EvidenceCard{
		ParadigmID: p.ID, ParadigmName: p.Name, StockCode: p.StockCode, StockName: p.StockName,
		GeneratedAt: time.Now(), ExperimentID: exp.ID, RunID: run.ID,
		SnapshotID: snapshot.ID, ResultHash: run.ResultHash,
		CounterEvidence: []paradigms.CounterExample{}, RiskFlags: []paradigms.RiskFlag{},
	}
	segmentArtifact := findEvidenceArtifact(run.Artifacts, "segment_metrics")
	transactionArtifact := findEvidenceArtifact(run.Artifacts, "transactions")
	manifestArtifact := findEvidenceArtifact(run.Artifacts, "reproducibility_manifest")
	for name, artifact := range map[string]*experiment.Artifact{
		"segment_metrics":          segmentArtifact,
		"transactions":             transactionArtifact,
		"reproducibility_manifest": manifestArtifact,
	} {
		if artifact == nil {
			card.UnavailableReasons = append(card.UnavailableReasons, "缺少持久化制品: "+name)
		}
	}
	if run.Metrics == nil {
		card.UnavailableReasons = append(card.UnavailableReasons, "运行缺少样本外指标")
	}
	if snapshot.ContentHash == "" {
		card.UnavailableReasons = append(card.UnavailableReasons, "冻结快照缺少内容哈希")
	}
	if len(card.UnavailableReasons) > 0 {
		card.PromotionBlockers = append(card.PromotionBlockers, card.UnavailableReasons...)
		card.RiskFlags = append(card.RiskFlags, paradigms.RiskFlag{
			Category: "evidence", Level: "critical",
			Message:    "实验制品不完整，数值证据不可用",
			Mitigation: "重新运行生产实验并检查制品持久化",
		})
		return card, nil
	}
	for _, artifact := range run.Artifacts {
		sum := sha256.Sum256(artifact.Content)
		if artifact.ContentHash == "" || artifact.ContentHash != hex.EncodeToString(sum[:]) {
			return nil, fmt.Errorf("artifact %s content hash mismatch", artifact.Name)
		}
	}
	var manifest evidenceExecutionManifest
	if err := json.Unmarshal(manifestArtifact.Content, &manifest); err != nil {
		return nil, fmt.Errorf("decode reproducibility_manifest artifact: %w", err)
	}
	transactionSum := sha256.Sum256(transactionArtifact.Content)
	if manifest.SnapshotID != snapshot.ID || manifest.SnapshotHash != snapshot.ContentHash ||
		manifest.ParadigmID != p.ID || manifest.ConfigHash != exp.ConfigHash ||
		manifest.TransactionHash != hex.EncodeToString(transactionSum[:]) {
		return nil, fmt.Errorf("reproducibility manifest does not match evidence inputs")
	}

	var segments []evidenceSegmentMetric
	if err := json.Unmarshal(segmentArtifact.Content, &segments); err != nil {
		return nil, fmt.Errorf("decode segment_metrics artifact: %w", err)
	}
	var transactions []evidenceTransactionSegment
	if err := json.Unmarshal(transactionArtifact.Content, &transactions); err != nil {
		return nil, fmt.Errorf("decode transactions artifact: %w", err)
	}
	inMetrics := selectEvidenceMetrics(segments, false)
	outMetrics := selectEvidenceMetrics(segments, true)
	if len(inMetrics) == 0 || len(outMetrics) == 0 {
		card.UnavailableReasons = append(card.UnavailableReasons, "分段制品缺少样本内或样本外指标")
		card.PromotionBlockers = append(card.PromotionBlockers, card.UnavailableReasons...)
		return card, nil
	}
	card.InSample = evidenceSample("in_sample", inMetrics, exp.Config.InitialCash)
	card.OutOfSample = evidenceSample("out_of_sample", outMetrics, exp.Config.InitialCash)

	var outTrades []backtest.CompletedTrade
	var outFills []trading.Fill
	var rejectionCount int
	for _, segment := range transactions {
		for index, trade := range segment.Trades {
			value := evidenceTradeRecord(run.ID, segment.Window, segment.Segment, index, trade)
			card.TradeSamples = append(card.TradeSamples, value)
			if segment.Segment == string(backtest.SegmentTest) {
				outTrades = append(outTrades, trade)
				if trade.NetPnL < 0 {
					ret := value.Return
					card.CounterEvidence = append(card.CounterEvidence, paradigms.CounterExample{
						Type: "losing_trade", Description: "真实样本外亏损交易",
						Period: trade.BuyExecutionDate.Format("2006-01-02") + " 至 " + trade.SellExecutionDate.Format("2006-01-02"),
						Return: ret, Reason: fmt.Sprintf("交易 %s 净亏损 %.2f 元", value.TradeID, -trade.NetPnL),
						Severity: "medium",
					})
				}
			}
		}
		if segment.Segment == string(backtest.SegmentTest) {
			outFills = append(outFills, segment.Fills...)
			rejectionCount += len(segment.Rejections)
		}
	}
	card.ConfidenceInterval = confidenceFromTrades(outTrades)
	card.CostAnalysis = evidenceCosts(card.OutOfSample, outTrades, outFills)
	card.DrawdownAnalysis = evidenceDrawdown(card.OutOfSample)
	card.Concentration = evidenceConcentration(p, exp.Config.InitialCash, outFills)
	card.Lineage = evidenceLineage(p, exp, run, snapshot)
	card.EvidenceHash = evidenceHash(exp, run, snapshot)
	card.Available = true

	card.PromotionBlockers = evidencePromotionBlockers(card)
	card.PromotionEligible = len(card.PromotionBlockers) == 0
	card.RiskFlags = evidenceRiskFlags(card, rejectionCount)
	if rejectionCount > 0 {
		card.CounterEvidence = append(card.CounterEvidence, paradigms.CounterExample{
			Type: "execution_rejection", Description: "样本外存在真实拒单",
			Period:   "out_of_sample",
			Reason:   fmt.Sprintf("%d 个订单因 T+1、涨跌停、停牌、资金或交易单位约束未成交", rejectionCount),
			Severity: "high",
		})
	}
	return card, nil
}

func findEvidenceArtifact(artifacts []experiment.Artifact, name string) *experiment.Artifact {
	for i := range artifacts {
		if artifacts[i].Name == name {
			return &artifacts[i]
		}
	}
	return nil
}

func selectEvidenceMetrics(all []evidenceSegmentMetric, outOfSample bool) []experiment.MetricSet {
	var result []experiment.MetricSet
	for _, item := range all {
		isTest := item.Segment == string(backtest.SegmentTest)
		if isTest == outOfSample {
			result = append(result, item.Metrics)
		}
	}
	return result
}

func evidenceSample(period string, metrics []experiment.MetricSet, initialCash float64) *paradigms.SampleResult {
	var gross, net, wins float64
	var trades int
	var maxDrawdown float64
	for _, metric := range metrics {
		gross += metric.GrossPnL
		net += metric.NetPnL
		trades += metric.TotalTrades
		wins += metric.WinRate * float64(metric.TotalTrades)
		if metric.MaxDrawdown > maxDrawdown {
			maxDrawdown = metric.MaxDrawdown
		}
	}
	totalReturn := 0.0
	if initialCash > 0 {
		totalReturn = net / (initialCash * float64(len(metrics)))
	}
	winRate := 0.0
	if trades > 0 {
		winRate = wins / float64(trades)
	}
	return &paradigms.SampleResult{
		Period: period, SampleSize: trades, TradesCount: trades,
		TotalReturn: &totalReturn, WinRate: &winRate, MaxDrawdown: &maxDrawdown,
		GrossPnL: &gross, NetPnL: &net,
	}
}

func evidenceTradeRecord(runID string, window int, segment string, index int, trade backtest.CompletedTrade) paradigms.TradeRecord {
	var tradeReturn *float64
	if invested := trade.BuyPrice * float64(trade.Quantity); invested > 0 {
		value := trade.NetPnL / invested
		tradeReturn = &value
	}
	return paradigms.TradeRecord{
		TradeID: fmt.Sprintf("%s-w%d-%s-%04d", runID, window, segment, index+1),
		Window:  window, Segment: segment, StockCode: trade.StockCode,
		BuySignalDate: trade.BuySignalDate, BuyExecutionDate: trade.BuyExecutionDate,
		SellSignalDate: trade.SellSignalDate, SellExecutionDate: trade.SellExecutionDate,
		Quantity: trade.Quantity, BuyPrice: trade.BuyPrice, SellPrice: trade.SellPrice,
		GrossPnL: trade.GrossPnL, NetPnL: trade.NetPnL, TotalCost: trade.TotalCost,
		Return: tradeReturn,
	}
}

func confidenceFromTrades(trades []backtest.CompletedTrade) *paradigms.CIResult {
	if len(trades) < 2 {
		return nil
	}
	returns := make([]float64, 0, len(trades))
	var mean float64
	for _, trade := range trades {
		invested := trade.BuyPrice * float64(trade.Quantity)
		if invested <= 0 {
			continue
		}
		value := trade.NetPnL / invested
		returns = append(returns, value)
		mean += value
	}
	if len(returns) < 2 {
		return nil
	}
	mean /= float64(len(returns))
	var sumSquares float64
	for _, value := range returns {
		delta := value - mean
		sumSquares += delta * delta
	}
	stddev := math.Sqrt(sumSquares / float64(len(returns)-1))
	standardError := stddev / math.Sqrt(float64(len(returns)))
	if standardError == 0 {
		return nil
	}
	tStatistic := mean / standardError
	lower, upper := mean-1.96*standardError, mean+1.96*standardError
	pValue := math.Erfc(math.Abs(tStatistic) / math.Sqrt2)
	return &paradigms.CIResult{
		Period: "out_of_sample", SampleSize: len(returns), MeanReturn: mean,
		CI95Lower: lower, CI95Upper: upper, CI95Width: upper - lower,
		TStatistic: tStatistic, PValue: pValue, Significant: pValue < 0.05,
		Method: "normal approximation over persisted out-of-sample trade returns",
	}
}

func evidenceCosts(sample *paradigms.SampleResult, trades []backtest.CompletedTrade, fills []trading.Fill) *paradigms.CostBreakdown {
	result := &paradigms.CostBreakdown{}
	for _, trade := range trades {
		result.TotalCost += trade.TotalCost
	}
	for _, fill := range fills {
		result.CommissionCost += fill.Cost.Commission
		result.TaxCost += fill.Cost.StampDuty
		result.TransferFee += fill.Cost.TransferFee
		result.SlippageCost += math.Abs(fill.Price-fill.SignalPrice) * float64(fill.Quantity)
	}
	if sample != nil {
		if sample.TotalReturn != nil && sample.NetPnL != nil && sample.GrossPnL != nil && *sample.NetPnL != 0 {
			grossReturn := *sample.TotalReturn * *sample.GrossPnL / *sample.NetPnL
			result.GrossReturn = &grossReturn
		}
		if sample.NetPnL != nil && sample.GrossPnL != nil && *sample.GrossPnL != 0 {
			netReturn := *sample.NetPnL
			grossReturn := *sample.GrossPnL
			retention := netReturn / grossReturn
			ratio := result.TotalCost / math.Abs(grossReturn)
			result.NetRetention = &retention
			result.CostRatio = &ratio
		}
		result.NetReturn = sample.TotalReturn
		if sample.TradesCount > 0 {
			perTrade := result.TotalCost / float64(sample.TradesCount)
			result.CostPerTrade = &perTrade
		}
	}
	return result
}

func evidenceDrawdown(sample *paradigms.SampleResult) *paradigms.DrawdownInfo {
	if sample == nil || sample.MaxDrawdown == nil {
		return nil
	}
	result := &paradigms.DrawdownInfo{MaxDrawdown: *sample.MaxDrawdown}
	if sample.TotalReturn != nil && *sample.TotalReturn != 0 {
		ratio := result.MaxDrawdown / math.Abs(*sample.TotalReturn)
		result.DrawdownRatio = &ratio
		if ratio > 1 {
			result.Warning = "样本外最大回撤高于净收益绝对值"
		}
	}
	return result
}

func evidenceConcentration(p *paradigms.Paradigm, initialCash float64, fills []trading.Fill) *paradigms.ConcentrationInfo {
	var maxWeight float64
	if initialCash > 0 {
		for _, fill := range fills {
			if fill.Side != trading.OrderBuy {
				continue
			}
			weight := fill.Price * float64(fill.Quantity) / initialCash
			if weight > maxWeight {
				maxWeight = weight
			}
		}
	}
	return &paradigms.ConcentrationInfo{
		MaxPositionWeight: maxWeight, ConcentrationIndex: 1,
		TopHoldings: []paradigms.HoldingItem{{
			StockCode: p.StockCode, StockName: p.StockName, Weight: maxWeight,
		}},
		DiversificationScore: 0,
	}
}

func evidenceLineage(p *paradigms.Paradigm, exp *experiment.Experiment, run *experiment.ExperimentRun, snapshot *paradigm.DatasetSnapshot) *paradigms.DataLineage {
	start, _ := parseEvidenceDate(snapshot.DateRange.Start)
	end, _ := parseEvidenceDate(snapshot.DateRange.End)
	artifactHashes := make(map[string]string, len(run.Artifacts))
	for _, artifact := range run.Artifacts {
		artifactHashes[artifact.Name] = artifact.ContentHash
	}
	manifestHashes := make(map[string]string, len(snapshot.KlineManifests))
	for _, manifest := range snapshot.KlineManifests {
		manifestHashes[fmt.Sprintf("%s:%d", manifest.Code, manifest.KType)] = manifest.ContentHash
	}
	lineage := &paradigms.DataLineage{
		DataSource: "frozen_kline_snapshot", DataVersion: snapshot.ContentHash,
		DataRange: snapshot.DateRange.Start + ".." + snapshot.DateRange.End,
		DataStart: start, DataEnd: end, LastUpdated: snapshot.CreatedAt,
		GeneratedBy: "paradigm_experiment_executor", GeneratedAt: run.StartTime,
		SourceHash: snapshot.ContentHash, SnapshotID: snapshot.ID,
		ExperimentID: exp.ID, RunID: run.ID, ResultHash: run.ResultHash,
		ArtifactHashes: artifactHashes, KlineManifestHashes: manifestHashes,
	}
	if p.ReviewStatus != "" {
		lineage.ReviewHistory = append(lineage.ReviewHistory, paradigms.ReviewRecord{
			Reviewer: "human", Action: p.ReviewStatus, Note: p.ReviewNote,
			Rating: p.ReviewRating, Timestamp: p.UpdatedAt,
		})
	}
	return lineage
}

func evidenceHash(exp *experiment.Experiment, run *experiment.ExperimentRun, snapshot *paradigm.DatasetSnapshot) string {
	values := []string{exp.ID, exp.ConfigHash, run.ID, run.ResultHash, snapshot.ID, snapshot.ContentHash}
	for _, artifact := range run.Artifacts {
		values = append(values, artifact.Name+"="+artifact.ContentHash)
	}
	sort.Strings(values[6:])
	sum := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(sum[:])
}

func evidencePromotionBlockers(card *paradigms.EvidenceCard) []string {
	var blockers []string
	if !card.Available {
		blockers = append(blockers, card.UnavailableReasons...)
	}
	if card.OutOfSample == nil || card.OutOfSample.TradesCount < 30 {
		blockers = append(blockers, "样本外真实完成交易少于 30 笔")
	}
	if card.ConfidenceInterval == nil {
		blockers = append(blockers, "真实样本外交易不足以计算置信区间")
	} else if card.ConfidenceInterval.CI95Lower <= 0 {
		blockers = append(blockers, "样本外交易收益的 95% 置信区间下界不大于 0")
	}
	if card.ParamSensitivity == nil {
		blockers = append(blockers, "缺少基于持久化参数扫描的敏感性证据")
	}
	if card.RobustnessScore == nil {
		blockers = append(blockers, "缺少完全由真实实验派生的稳健性评分")
	}
	return blockers
}

func evidenceRiskFlags(card *paradigms.EvidenceCard, rejectionCount int) []paradigms.RiskFlag {
	var flags []paradigms.RiskFlag
	for _, blocker := range card.PromotionBlockers {
		flags = append(flags, paradigms.RiskFlag{
			Category: "promotion_gate", Level: "high", Message: blocker,
		})
	}
	if card.DrawdownAnalysis != nil && card.DrawdownAnalysis.MaxDrawdown > 0.15 {
		flags = append(flags, paradigms.RiskFlag{
			Category: "drawdown", Level: "critical", Message: "样本外最大回撤超过 15%",
		})
	}
	if rejectionCount > 0 {
		flags = append(flags, paradigms.RiskFlag{
			Category: "execution", Level: "high",
			Message: fmt.Sprintf("样本外存在 %d 个真实拒单", rejectionCount),
		})
	}
	return flags
}

func parseEvidenceDate(value string) (time.Time, error) {
	for _, layout := range []string{"20060102", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported date %q", value)
}
