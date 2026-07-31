package paradigms

import "time"

// Paradigm represents a trading pattern with contextual conditions
type Paradigm struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Side        string            `json:"side"` // buy / sell
	Context     Context           `json:"context"`
	StockCode   string            `json:"stock_code,omitempty"`
	StockName   string            `json:"stock_name,omitempty"`
	BuyConds    []Condition       `json:"buy_conditions"`
	SellConds   SellConditions    `json:"sell_conditions"`
	Confirm     []string          `json:"confirmations,omitempty"`
	Invalid     []string          `json:"invalidations,omitempty"`
	Expectation Expectation       `json:"expectation"`
	Rationale   string            `json:"rationale,omitempty"`
	AgentText   string            `json:"agent_text,omitempty"`
	Source      ParadigmSource    `json:"source,omitempty"`
	Validation  ValidationSummary `json:"validation,omitempty"`
	// Evidence holds the admission check result for paradigm promotion.
	// Populated when the paradigm undergoes validation.
	Evidence  *ParadigmEvidence `json:"evidence,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Tags      []string          `json:"tags,omitempty"`
	// Review fields
	ReviewStatus string   `json:"review_status,omitempty"` // pending / reviewed / verified / promoted / degraded / suspended / rejected
	ReviewNote   string   `json:"review_note,omitempty"`
	ReviewRating int      `json:"review_rating,omitempty"` // 1-5
	ActualReturn *float64 `json:"actual_return,omitempty"` // actual return after the paradigm was created
	// 生命周期审计
	Transitions []StateTransition `json:"transitions,omitempty"`
}

type ParadigmSource struct {
	AgentVersion string `json:"agent_version,omitempty"`
	Model        string `json:"model,omitempty"`
	KlineType    string `json:"kline_type,omitempty"`
	Days         int    `json:"days,omitempty"`
	GeneratedAt  string `json:"generated_at,omitempty"`
	DataWindow   string `json:"data_window,omitempty"`
	CacheKey     string `json:"cache_key,omitempty"`
}

type ValidationSummary struct {
	Valid              bool     `json:"valid"`
	Errors             []string `json:"errors,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	AutoEvaluable      int      `json:"auto_evaluable"`
	TotalConditions    int      `json:"total_conditions"`
	AutoEvaluableRatio float64  `json:"auto_evaluable_ratio"`
	DataCompleteness   float64  `json:"data_completeness"`
	ReliabilityLabel   string   `json:"reliability_label,omitempty"`
}

type Context struct {
	MarketCap           string `json:"market_cap"`           // small / mid / large / mega
	ShareholderDominant string `json:"shareholder_dominant"` // retail / hot_money / foreign / institutional / state / mixed
	Activity            string `json:"activity,omitempty"`   // active / normal / quiet
	Trend               string `json:"trend,omitempty"`      // uptrend / downtrend / range / volatile
}

type Condition struct {
	Indicator string `json:"indicator"` // e.g. "MACD.DIF", "RSI6", "MA20"
	Operator  string `json:"operator"`  // cross_above, cross_below, gt, lt, between, near
	Value     string `json:"value"`     // threshold value or range
}

type SellConditions struct {
	TakeProfit []Condition `json:"take_profit,omitempty"`
	StopLoss   []Condition `json:"stop_loss,omitempty"`
}

type Expectation struct {
	HoldingPeriod  string  `json:"holding_period"`
	ExpectedReturn string  `json:"expected_return"`
	RiskReward     string  `json:"risk_reward_ratio"`
	WinRate        float64 `json:"win_rate,omitempty"`
	SampleSize     int     `json:"sample_size,omitempty"`
	Confidence     float64 `json:"confidence"` // 0-1
}

// ParadigmEvidence wraps an admission-check result for JSON serialization within a Paradigm.
type ParadigmEvidence struct {
	Eligible    bool     `json:"eligible"`
	Level       string   `json:"level"`
	Score       float64  `json:"score"`
	Reasons     []string `json:"reasons,omitempty"`
	MustFix     []string `json:"must_fix,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// EvaluatedItem is a condition with its current status
type EvaluatedItem struct {
	Text   string `json:"text"`
	Status string `json:"status"` // met / not_met / unknown
	Reason string `json:"reason,omitempty"`
}
