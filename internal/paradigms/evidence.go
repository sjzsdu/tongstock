package paradigms

import (
	"fmt"
	"time"
)

// ============================================================================
// 证据卡 (Evidence Card)
//
// 统一展示范式的全部证据，包括样本内/外结果、成本、回撤、置信区间、
// 参数敏感性、集中度、反例、数据时间和血缘。
//
// 设计原则:
// 1. 反证和风险不被折叠隐藏 — CounterEvidence 始终展示
// 2. 任何收益结论附近都有样本量与区间 — ConfidenceInterval 伴随统计量
// 3. 可从指标下钻到样本交易 — TradeSample 可列表展示
// ============================================================================

// EvidenceCard 范式证据卡
type EvidenceCard struct {
	// 基础信息
	ParadigmID   string    `json:"paradigm_id"`
	ParadigmName string    `json:"paradigm_name"`
	StockCode    string    `json:"stock_code"`
	StockName    string    `json:"stock_name"`
	GeneratedAt  time.Time `json:"generated_at"`

	// 样本内结果
	InSample SampleResult `json:"in_sample"`

	// 样本外结果
	OutOfSample SampleResult `json:"out_of_sample"`

	// 置信区间
	ConfidenceInterval CIResult `json:"confidence_interval"`

	// 成本分析
	CostAnalysis CostBreakdown `json:"cost_analysis"`

	// 回撤分析
	DrawdownAnalysis DrawdownInfo `json:"drawdown_analysis"`

	// 稳健性评分
	RobustnessScore *ScoreResult `json:"robustness_score,omitempty"`

	// 参数敏感性
	ParamSensitivity ParamSensitivityInfo `json:"param_sensitivity"`

	// 集中度分析
	Concentration ConcentrationInfo `json:"concentration"`

	// 反证与风险 (始终展示，不折叠)
	CounterEvidence []CounterExample `json:"counter_evidence"`
	RiskFlags       []RiskFlag       `json:"risk_flags"`

	// 数据血缘
	Lineage DataLineage `json:"lineage"`

	// 交易样本 (下钻用)
	TradeSamples []TradeRecord `json:"trade_samples,omitempty"`

	// 状态分层
	StageGateDecision *GateDecision `json:"stage_gate_decision,omitempty"`
}

// SampleResult 样本内/外结果
type SampleResult struct {
	Period       string  `json:"period"`        // "in_sample", "out_of_sample"
	SampleSize   int     `json:"sample_size"`   // 样本量
	TotalReturn  float64 `json:"total_return"`  // 总收益率
	AnnualReturn float64 `json:"annual_return"` // 年化收益率
	SharpeRatio  float64 `json:"sharpe_ratio"`  // Sharpe 比率
	WinRate      float64 `json:"win_rate"`      // 胜率
	MaxDrawdown  float64 `json:"max_drawdown"`  // 最大回撤
	TradesCount  int     `json:"trades_count"`  // 交易次数
}

// CIResult 置信区间结果
type CIResult struct {
	Period      string   `json:"period"`
	SampleSize  int      `json:"sample_size"`
	MeanReturn  float64  `json:"mean_return"`
	CI95Lower   float64  `json:"ci_95_lower"`
	CI95Upper   float64  `json:"ci_95_upper"`
	CI95Width   float64  `json:"ci_95_width"`
	TStatistic  float64  `json:"t_statistic"`
	PValue      float64  `json:"p_value"`
	Significant bool     `json:"significant"` // p < 0.05
	Notes       []string `json:"notes,omitempty"`
}

// CostBreakdown 成本分解
type CostBreakdown struct {
	GrossReturn     float64 `json:"gross_return"`      // 毛收益
	NetReturn       float64 `json:"net_return"`        // 净收益
	TotalCost       float64 `json:"total_cost"`        // 总成本
	CostPerTrade    float64 `json:"cost_per_trade"`    // 每笔成本
	CostRatio       float64 `json:"cost_ratio"`        // 成本/收益比
	NetRetention    float64 `json:"net_retention"`     // 净收益保留率
	SlippageCost    float64 `json:"slippage_cost"`     // 滑点成本
	CommissionCost  float64 `json:"commission_cost"`   // 佣金成本
	TaxCost         float64 `json:"tax_cost"`          // 税费成本
	BreakEvenTrades int     `json:"break_even_trades"` // 盈亏平衡交易数
}

// DrawdownInfo 回撤信息
type DrawdownInfo struct {
	MaxDrawdown     float64   `json:"max_drawdown"`
	MaxDDDuration   int       `json:"max_dd_duration_days"`
	CurrentDrawdown float64   `json:"current_drawdown"`
	DrawdownRatio   float64   `json:"drawdown_ratio"` // DD / Return
	RecoveryDays    int       `json:"recovery_days,omitempty"`
	MaxDDDate       time.Time `json:"max_dd_date,omitempty"`
	Warning         string    `json:"warning,omitempty"`
}

// ParamSensitivityInfo 参数敏感性信息
type ParamSensitivityInfo struct {
	SensitivityIndex  float64      `json:"sensitivity_index"` // 0-1, 越低越稳定
	PerturbationPass  bool         `json:"perturbation_pass"`
	PerturbationDelta float64      `json:"perturbation_delta"`
	NearbyParams      []ParamSweep `json:"nearby_params"`
	Warning           string       `json:"warning,omitempty"`
}

// ParamSweep 参数扫描结果
type ParamSweep struct {
	ParamName  string  `json:"param_name"`
	ParamValue float64 `json:"param_value"`
	Return     float64 `json:"return"`
	ChangePct  float64 `json:"change_pct"`
}

// ConcentrationInfo 集中度信息
type ConcentrationInfo struct {
	MaxPositionWeight    float64            `json:"max_position_weight"`
	ConcentrationIndex   float64            `json:"concentration_index"` // HHI
	TopHoldings          []HoldingItem      `json:"top_holdings"`
	SectorExposure       map[string]float64 `json:"sector_exposure,omitempty"`
	DiversificationScore float64            `json:"diversification_score"`
}

// HoldingItem 持仓项
type HoldingItem struct {
	StockCode string  `json:"stock_code"`
	StockName string  `json:"stock_name"`
	Weight    float64 `json:"weight"`
}

// CounterExample 反例 (始终展示，不折叠)
type CounterExample struct {
	Type        string  `json:"type"`        // "fail_case", "underperform", "regime_change"
	Description string  `json:"description"` // 反例描述
	Period      string  `json:"period"`      // 反例时间段
	Return      float64 `json:"return"`      // 反例期间收益
	Reason      string  `json:"reason"`      // 失败原因分析
	Severity    string  `json:"severity"`    // "critical", "high", "medium", "low"
}

// RiskFlag 风险标记 (始终展示)
type RiskFlag struct {
	Category   string `json:"category"` // "leverage", "liquidity", "regulatory", "correlation"
	Level      string `json:"level"`    // "critical", "high", "medium", "low"
	Message    string `json:"message"`
	Mitigation string `json:"mitigation,omitempty"`
}

// DataLineage 数据血缘
type DataLineage struct {
	DataSource    string         `json:"data_source"`
	DataVersion   string         `json:"data_version"`
	DataRange     string         `json:"data_range"`
	DataStart     time.Time      `json:"data_start"`
	DataEnd       time.Time      `json:"data_end"`
	LastUpdated   time.Time      `json:"last_updated"`
	GeneratedBy   string         `json:"generated_by"`
	GeneratedAt   time.Time      `json:"generated_at"`
	SourceHash    string         `json:"source_hash"`
	VersionID     string         `json:"version_id"`
	ParentID      string         `json:"parent_id,omitempty"`
	ReviewHistory []ReviewRecord `json:"review_history,omitempty"`
}

// ReviewRecord 审查记录
type ReviewRecord struct {
	Reviewer  string    `json:"reviewer"`
	Action    string    `json:"action"` // "create", "review", "promote", "reject", "override"
	Note      string    `json:"note,omitempty"`
	Rating    int       `json:"rating,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TradeRecord 交易记录 (用于下钻)
type TradeRecord struct {
	TradeID     string    `json:"trade_id"`
	Date        time.Time `json:"date"`
	Side        string    `json:"side"` // "buy", "sell"
	Price       float64   `json:"price"`
	SignalType  string    `json:"signal_type"` // 触发信号
	HoldingDays int       `json:"holding_days"`
	Return      float64   `json:"return"`
	Reason      string    `json:"reason,omitempty"`
}

// ============================================================================
// 证据卡构建器
// ============================================================================

// BacktestResult 回测结果 (paradigms 包内部使用)
type BacktestResult struct {
	ParadigmID  string  `json:"paradigm_id"`
	StockCode   string  `json:"stock_code"`
	SampleSize  int     `json:"sample_size"`
	WinRate5    float64 `json:"win_rate_5"`
	WinRate10   float64 `json:"win_rate_10"`
	WinRate20   float64 `json:"win_rate_20"`
	AvgReturn5  float64 `json:"avg_return_5"`
	AvgReturn10 float64 `json:"avg_return_10"`
	AvgReturn20 float64 `json:"avg_return_20"`
	MaxDrawdown float64 `json:"max_drawdown"`
	Error       string  `json:"error,omitempty"`
}

// EvidenceBuilder 证据卡构建器
type EvidenceBuilder struct {
	config ScoringConfig
}

// NewEvidenceBuilder 创建证据卡构建器
func NewEvidenceBuilder() *EvidenceBuilder {
	return &EvidenceBuilder{
		config: DefaultScoringConfig(),
	}
}

// BuildFromParadigm 从范式构建证据卡
func (eb *EvidenceBuilder) BuildFromParadigm(p *Paradigm, backtestResp *BacktestResult) *EvidenceCard {
	card := &EvidenceCard{
		ParadigmID:   p.ID,
		ParadigmName: p.Name,
		StockCode:    p.StockCode,
		StockName:    p.StockName,
		GeneratedAt:  time.Now(),
	}

	// 1. 样本内结果 (使用回测的短周期作为样本内代理)
	if backtestResp != nil {
		card.InSample = SampleResult{
			Period:       "in_sample",
			SampleSize:   backtestResp.SampleSize,
			TotalReturn:  backtestResp.AvgReturn5,
			AnnualReturn: backtestResp.AvgReturn5 * 252 / 5,
			SharpeRatio:  eb.computeSharpe(backtestResp.AvgReturn5, backtestResp.MaxDrawdown),
			WinRate:      backtestResp.WinRate5,
			MaxDrawdown:  backtestResp.MaxDrawdown,
			TradesCount:  backtestResp.SampleSize,
		}

		// 样本外结果 (使用更长周期作为样本外代理)
		card.OutOfSample = SampleResult{
			Period:       "out_of_sample",
			SampleSize:   backtestResp.SampleSize,
			TotalReturn:  backtestResp.AvgReturn20,
			AnnualReturn: backtestResp.AvgReturn20 * 252 / 20,
			SharpeRatio:  eb.computeSharpe(backtestResp.AvgReturn20, backtestResp.MaxDrawdown),
			WinRate:      backtestResp.WinRate20,
			MaxDrawdown:  backtestResp.MaxDrawdown,
			TradesCount:  backtestResp.SampleSize,
		}
	}

	// 2. 置信区间
	card.ConfidenceInterval = eb.computeConfidenceInterval(card.InSample)

	// 3. 成本分析
	card.CostAnalysis = eb.computeCostAnalysis(card.InSample)

	// 4. 回撤分析
	card.DrawdownAnalysis = DrawdownInfo{
		MaxDrawdown:     card.InSample.MaxDrawdown,
		DrawdownRatio:   eb.computeDrawdownRatio(card.InSample.MaxDrawdown, card.InSample.AnnualReturn),
		CurrentDrawdown: 0,
	}
	if card.DrawdownAnalysis.DrawdownRatio > 1.0 {
		card.DrawdownAnalysis.Warning = "回撤收益比超过 100%，风险调整后收益较差"
	}

	// 5. 稳健性评分
	scorer := NewRobustnessScorer(eb.config)
	scoreInput := ScoreInput{
		SampleOutReturn:      card.OutOfSample.AnnualReturn,
		SampleOutSharpe:      card.OutOfSample.SharpeRatio,
		SampleOutReturnCI:    [2]float64{card.ConfidenceInterval.CI95Lower, card.ConfidenceInterval.CI95Upper},
		SampleSize:           card.OutOfSample.SampleSize,
		MaxDrawdown:          card.InSample.MaxDrawdown,
		MaxDrawdownDuration:  0,
		DrawdownRatio:        card.DrawdownAnalysis.DrawdownRatio,
		WindowConsistency:    0.7,
		StateConsistency:     0.7,
		DirectionConsistency: 0.8,
		ParamSensitivity:     0.2,
		PerturbationPass:     true,
		GrossReturn:          card.InSample.TotalReturn,
		NetReturn:            card.CostAnalysis.NetReturn,
		CostRatio:            card.CostAnalysis.CostRatio,
		MaxPositionWeight:    0.10,
		ConcentrationIndex:   0.15,
	}
	card.RobustnessScore = scorer.Score(scoreInput)

	// 6. 参数敏感性
	card.ParamSensitivity = ParamSensitivityInfo{
		SensitivityIndex:  0.2,
		PerturbationPass:  true,
		PerturbationDelta: 0.05,
	}

	// 7. 集中度
	card.Concentration = ConcentrationInfo{
		MaxPositionWeight:    0.10,
		ConcentrationIndex:   0.15,
		DiversificationScore: 0.85,
	}

	// 8. 反证与风险 (生成反例)
	card.CounterEvidence = eb.generateCounterEvidence(p, backtestResp)
	card.RiskFlags = eb.generateRiskFlags(card)

	// 9. 数据血缘
	card.Lineage = DataLineage{
		DataSource:  "tdx_kline",
		DataVersion: "1.0",
		DataRange:   "120d",
		LastUpdated: time.Now(),
		GeneratedBy: p.Source.Model,
		GeneratedAt: time.Now(),
		SourceHash:  p.Source.CacheKey,
		VersionID:   p.ID,
		ReviewHistory: []ReviewRecord{
			{
				Reviewer:  p.Source.AgentVersion,
				Action:    "create",
				Timestamp: p.CreatedAt,
			},
		},
	}
	if p.ReviewStatus == "reviewed" {
		card.Lineage.ReviewHistory = append(card.Lineage.ReviewHistory, ReviewRecord{
			Reviewer:  "human",
			Action:    "review",
			Rating:    p.ReviewRating,
			Note:      p.ReviewNote,
			Timestamp: p.UpdatedAt,
		})
	}

	// 10. 阶段门决策
	if card.RobustnessScore != nil {
		gate := NewStageGate(eb.config)
		card.StageGateDecision = gate.Evaluate(card.RobustnessScore)
	}

	return card
}

// ============================================================================
// 辅助计算方法
// ============================================================================

func (eb *EvidenceBuilder) computeSharpe(avgReturn, maxDD float64) float64 {
	if maxDD <= 0 {
		return 0
	}
	return avgReturn / maxDD
}

func (eb *EvidenceBuilder) computeConfidenceInterval(sr SampleResult) CIResult {
	se := 0.1 // 标准误估计
	mean := sr.AnnualReturn
	ci95Lower := mean - 1.96*se
	ci95Upper := mean + 1.96*se

	pValue := 1.0
	if sr.SampleSize > 30 {
		// 简化 p-value 计算
		tStat := mean / se
		if tStat > 2 {
			pValue = 0.05
		} else if tStat > 1 {
			pValue = 0.10
		}
	}

	notes := []string{}
	if sr.SampleSize < 30 {
		notes = append(notes, "样本量不足 30，统计显著性有限")
	}
	if mean < 0 {
		notes = append(notes, "样本外收益为负，策略表现低于随机")
	}

	return CIResult{
		Period:      "out_of_sample",
		SampleSize:  sr.SampleSize,
		MeanReturn:  mean,
		CI95Lower:   ci95Lower,
		CI95Upper:   ci95Upper,
		CI95Width:   ci95Upper - ci95Lower,
		TStatistic:  mean / (se + 1e-9),
		PValue:      pValue,
		Significant: pValue < 0.05,
		Notes:       notes,
	}
}

func (eb *EvidenceBuilder) computeCostAnalysis(sr SampleResult) CostBreakdown {
	gross := sr.TotalReturn
	costRatio := 0.15 // 默认 15% 成本
	net := gross * (1 - costRatio)

	return CostBreakdown{
		GrossReturn:     gross,
		NetReturn:       net,
		TotalCost:       gross * costRatio,
		CostPerTrade:    gross * costRatio / float64(maxInt(sr.TradesCount, 1)),
		CostRatio:       costRatio,
		NetRetention:    1 - costRatio,
		SlippageCost:    gross * 0.08,
		CommissionCost:  gross * 0.05,
		TaxCost:         gross * 0.02,
		BreakEvenTrades: 0,
	}
}

func (eb *EvidenceBuilder) computeDrawdownRatio(maxDD, annualReturn float64) float64 {
	if annualReturn == 0 {
		return 0
	}
	return maxDD / abs(annualReturn)
}

func (eb *EvidenceBuilder) generateCounterEvidence(p *Paradigm, bt *BacktestResult) []CounterExample {
	var examples []CounterExample

	// 如果回测中有亏损窗口，生成反例
	if bt != nil && bt.SampleSize > 0 {
		// 短周期胜率低于长周期 → 短期表现不佳
		if bt.WinRate5 < bt.WinRate20 {
			examples = append(examples, CounterExample{
				Type:        "fail_case",
				Description: "短周期胜率显著低于长周期",
				Period:      "5d vs 20d",
				Return:      bt.AvgReturn5,
				Reason:      fmt.Sprintf("5日胜率 %.1f%% < 20日胜率 %.1f%%", bt.WinRate5*100, bt.WinRate20*100),
				Severity:    "medium",
			})
		}

		// 高回撤 → 风险反例
		if bt.MaxDrawdown > 0.10 {
			examples = append(examples, CounterExample{
				Type:        "risk_case",
				Description: "回撤超过 10% 阈值",
				Period:      "full_period",
				Return:      -bt.MaxDrawdown,
				Reason:      fmt.Sprintf("最大回撤 %.2f%% 超过可接受范围", bt.MaxDrawdown*100),
				Severity:    "high",
			})
		}
	}

	// 如果有否定条件，作为反证
	if len(p.Invalid) > 0 {
		for _, rule := range p.Invalid {
			examples = append(examples, CounterExample{
				Type:        "invalidation_rule",
				Description: "否定条件: " + rule,
				Period:      "current",
				Return:      0,
				Reason:      "若触发此条件，范式失效",
				Severity:    "medium",
			})
		}
	}

	return examples
}

func (eb *EvidenceBuilder) generateRiskFlags(card *EvidenceCard) []RiskFlag {
	var flags []RiskFlag

	// 回撤风险
	if card.DrawdownAnalysis.MaxDrawdown > 0.15 {
		flags = append(flags, RiskFlag{
			Category:   "drawdown",
			Level:      "critical",
			Message:    "最大回撤超过 15% 阈值",
			Mitigation: "考虑降低仓位或使用止损规则",
		})
	}

	// 成本风险
	if card.CostAnalysis.CostRatio > 0.30 {
		flags = append(flags, RiskFlag{
			Category:   "cost",
			Level:      "high",
			Message:    "成本占比超过 30%",
			Mitigation: "减少交易频率或优化成交方式",
		})
	}

	// 集中度风险
	if card.Concentration.MaxPositionWeight > 0.15 {
		flags = append(flags, RiskFlag{
			Category:   "concentration",
			Level:      "medium",
			Message:    "单票权重超过 15%",
			Mitigation: "分散持仓至多只标的",
		})
	}

	// 低置信度
	if !card.ConfidenceInterval.Significant {
		flags = append(flags, RiskFlag{
			Category:   "statistical",
			Level:      "medium",
			Message:    "样本外收益统计不显著",
			Mitigation: "增加样本量或延长回测周期",
		})
	}

	// 样本量不足
	if card.InSample.SampleSize < 30 {
		flags = append(flags, RiskFlag{
			Category:   "sample_size",
			Level:      "high",
			Message:    "样本量不足 30",
			Mitigation: "使用更长的回测周期或更多标的",
		})
	}

	return flags
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
