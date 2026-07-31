package paradigms

import "time"

// EvidenceCard 只承载可追溯的实验事实。无法从冻结快照、持久化运行或
// 交易制品证明的字段必须保持 nil，并在 UnavailableReasons 中说明。
type EvidenceCard struct {
	ParadigmID        string    `json:"paradigm_id"`
	ParadigmName      string    `json:"paradigm_name"`
	StockCode         string    `json:"stock_code"`
	StockName         string    `json:"stock_name"`
	GeneratedAt       time.Time `json:"generated_at"`
	Available         bool      `json:"available"`
	PromotionEligible bool      `json:"promotion_eligible"`

	UnavailableReasons []string `json:"unavailable_reasons,omitempty"`
	PromotionBlockers  []string `json:"promotion_blockers,omitempty"`

	ExperimentID string `json:"experiment_id,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	SnapshotID   string `json:"snapshot_id,omitempty"`
	EvidenceHash string `json:"evidence_hash,omitempty"`
	ResultHash   string `json:"result_hash,omitempty"`

	InSample           *SampleResult         `json:"in_sample,omitempty"`
	OutOfSample        *SampleResult         `json:"out_of_sample,omitempty"`
	ConfidenceInterval *CIResult             `json:"confidence_interval,omitempty"`
	CostAnalysis       *CostBreakdown        `json:"cost_analysis,omitempty"`
	DrawdownAnalysis   *DrawdownInfo         `json:"drawdown_analysis,omitempty"`
	RobustnessScore    *ScoreResult          `json:"robustness_score,omitempty"`
	ParamSensitivity   *ParamSensitivityInfo `json:"param_sensitivity,omitempty"`
	Concentration      *ConcentrationInfo    `json:"concentration,omitempty"`
	CounterEvidence    []CounterExample      `json:"counter_evidence"`
	RiskFlags          []RiskFlag            `json:"risk_flags"`
	Lineage            *DataLineage          `json:"lineage,omitempty"`
	TradeSamples       []TradeRecord         `json:"trade_samples,omitempty"`
	StageGateDecision  *GateDecision         `json:"stage_gate_decision,omitempty"`
}

type SampleResult struct {
	Period       string   `json:"period"`
	SampleSize   int      `json:"sample_size"`
	TotalReturn  *float64 `json:"total_return,omitempty"`
	AnnualReturn *float64 `json:"annual_return,omitempty"`
	SharpeRatio  *float64 `json:"sharpe_ratio,omitempty"`
	WinRate      *float64 `json:"win_rate,omitempty"`
	MaxDrawdown  *float64 `json:"max_drawdown,omitempty"`
	TradesCount  int      `json:"trades_count"`
	GrossPnL     *float64 `json:"gross_pnl,omitempty"`
	NetPnL       *float64 `json:"net_pnl,omitempty"`
}

type CIResult struct {
	Period      string   `json:"period"`
	SampleSize  int      `json:"sample_size"`
	MeanReturn  float64  `json:"mean_return"`
	CI95Lower   float64  `json:"ci_95_lower"`
	CI95Upper   float64  `json:"ci_95_upper"`
	CI95Width   float64  `json:"ci_95_width"`
	TStatistic  float64  `json:"t_statistic"`
	PValue      float64  `json:"p_value"`
	Significant bool     `json:"significant"`
	Method      string   `json:"method"`
	Notes       []string `json:"notes,omitempty"`
}

type CostBreakdown struct {
	GrossReturn    *float64 `json:"gross_return,omitempty"`
	NetReturn      *float64 `json:"net_return,omitempty"`
	TotalCost      float64  `json:"total_cost"`
	CostPerTrade   *float64 `json:"cost_per_trade,omitempty"`
	CostRatio      *float64 `json:"cost_ratio,omitempty"`
	NetRetention   *float64 `json:"net_retention,omitempty"`
	SlippageCost   float64  `json:"slippage_cost"`
	CommissionCost float64  `json:"commission_cost"`
	TaxCost        float64  `json:"tax_cost"`
	TransferFee    float64  `json:"transfer_fee"`
}

type DrawdownInfo struct {
	MaxDrawdown   float64  `json:"max_drawdown"`
	DrawdownRatio *float64 `json:"drawdown_ratio,omitempty"`
	Warning       string   `json:"warning,omitempty"`
}

type ParamSensitivityInfo struct {
	SensitivityIndex  float64      `json:"sensitivity_index"`
	PerturbationPass  bool         `json:"perturbation_pass"`
	PerturbationDelta float64      `json:"perturbation_delta"`
	NearbyParams      []ParamSweep `json:"nearby_params"`
	Warning           string       `json:"warning,omitempty"`
}

type ParamSweep struct {
	ParamName  string  `json:"param_name"`
	ParamValue float64 `json:"param_value"`
	Return     float64 `json:"return"`
	ChangePct  float64 `json:"change_pct"`
}

type ConcentrationInfo struct {
	MaxPositionWeight    float64            `json:"max_position_weight"`
	ConcentrationIndex   float64            `json:"concentration_index"`
	TopHoldings          []HoldingItem      `json:"top_holdings"`
	SectorExposure       map[string]float64 `json:"sector_exposure,omitempty"`
	DiversificationScore float64            `json:"diversification_score"`
}

type HoldingItem struct {
	StockCode string  `json:"stock_code"`
	StockName string  `json:"stock_name"`
	Weight    float64 `json:"weight"`
}

type CounterExample struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Period      string   `json:"period"`
	Return      *float64 `json:"return,omitempty"`
	Reason      string   `json:"reason"`
	Severity    string   `json:"severity"`
}

type RiskFlag struct {
	Category   string `json:"category"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	Mitigation string `json:"mitigation,omitempty"`
}

type DataLineage struct {
	DataSource          string            `json:"data_source"`
	DataVersion         string            `json:"data_version"`
	DataRange           string            `json:"data_range"`
	DataStart           time.Time         `json:"data_start"`
	DataEnd             time.Time         `json:"data_end"`
	LastUpdated         time.Time         `json:"last_updated"`
	GeneratedBy         string            `json:"generated_by"`
	GeneratedAt         time.Time         `json:"generated_at"`
	SourceHash          string            `json:"source_hash"`
	SnapshotID          string            `json:"snapshot_id"`
	ExperimentID        string            `json:"experiment_id"`
	RunID               string            `json:"run_id"`
	ResultHash          string            `json:"result_hash"`
	ArtifactHashes      map[string]string `json:"artifact_hashes"`
	KlineManifestHashes map[string]string `json:"kline_manifest_hashes"`
	ReviewHistory       []ReviewRecord    `json:"review_history,omitempty"`
}

type ReviewRecord struct {
	Reviewer  string    `json:"reviewer"`
	Action    string    `json:"action"`
	Note      string    `json:"note,omitempty"`
	Rating    int       `json:"rating,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TradeRecord 是真实 CompletedTrade 的展示投影，不生成或补齐任何交易。
type TradeRecord struct {
	TradeID           string    `json:"trade_id"`
	Window            int       `json:"window"`
	Segment           string    `json:"segment"`
	StockCode         string    `json:"stock_code"`
	BuySignalDate     time.Time `json:"buy_signal_date"`
	BuyExecutionDate  time.Time `json:"buy_execution_date"`
	SellSignalDate    time.Time `json:"sell_signal_date"`
	SellExecutionDate time.Time `json:"sell_execution_date"`
	Quantity          int       `json:"quantity"`
	BuyPrice          float64   `json:"buy_price"`
	SellPrice         float64   `json:"sell_price"`
	GrossPnL          float64   `json:"gross_pnl"`
	NetPnL            float64   `json:"net_pnl"`
	TotalCost         float64   `json:"total_cost"`
	Return            *float64  `json:"return,omitempty"`
}
