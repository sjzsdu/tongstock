// Package paradigm defines the research domain model and paradigm lifecycle
// for the TongStock paradigm research system.
//
// The domain model covers: Hypothesis → Experiment → Candidate → Validation →
// Observation → Promoted / Relegated / Retired.
package paradigm

import (
	"errors"
	"fmt"
	"time"
)

// ============================================================================
// Price Adjustment Convention (统一复权/公司行为/价格口径)
// ============================================================================

// PriceAdjustment 约定研究与交易使用的价格口径。任何会输出价格或收益的
// 结构 (DatasetSnapshot / FeatureSet / Signal 等) 必须显式携带此枚举，
// 以保证实验、信号、收益计算口径一致、可复现、可审计。
type PriceAdjustment string

const (
	// PriceRaw 不复权: 保存/使用交易所原始成交价格。
	// 适用于: 实盘下单、原始数据存储。
	PriceRaw PriceAdjustment = "raw"

	// PriceForward 前复权: 以"最新"为基准，对历史价格向下调整。
	// 适用于: 技术指标计算、图形展示、长周期收益回测。
	PriceForward PriceAdjustment = "forward"

	// PriceBackward 后复权: 以"最早"为基准，对后续价格向上调整。
	// 适用于: 全周期累计收益率、长期持有回测。
	PriceBackward PriceAdjustment = "backward"
)

// IsValid 返回口径是否合法。空值视为 raw (不复权)，向后兼容。
func (p PriceAdjustment) IsValid() bool {
	switch p {
	case "", PriceRaw, PriceForward, PriceBackward:
		return true
	}
	return false
}

// Normalize 把空值/非法值归一为 raw。
func (p PriceAdjustment) Normalize() PriceAdjustment {
	if !p.IsValid() {
		return PriceRaw
	}
	if p == "" {
		return PriceRaw
	}
	return p
}

// String 人类可读描述。
func (p PriceAdjustment) String() string {
	switch p.Normalize() {
	case PriceRaw:
		return "不复权"
	case PriceForward:
		return "前复权"
	case PriceBackward:
		return "后复权"
	default:
		return "未知口径"
	}
}

// ============================================================================
// Domain Model
// ============================================================================

// Hypothesis represents a testable research hypothesis about a trading pattern.
// It is the entry point of the research pipeline.
type Hypothesis struct {
	ID        string    `json:"id"`
	Statement string    `json:"statement"` // e.g. "MACD金叉在20日均线向上时具有正向收益"
	Context   string    `json:"context"`   // market context constraint
	Rationale string    `json:"rationale"` // why this hypothesis is worth testing
	Tags      []string  `json:"tags"`      // e.g. ["MACD", "trend-following"]
	CreatedAt time.Time `json:"created_at"`
	Author    string    `json:"author"`
	Status    string    `json:"status"` // draft / tested / accepted / rejected
}

// DataSource 描述单一数据源在快照中的引用信息, 用于 as-of 追溯和版本比对。
type DataSource struct {
	Type            string    `json:"type"`              // kline / quote / finance / news / factor
	Version         string    `json:"version"`           // 数据版本号或 hash
	AsOf            time.Time `json:"as_of"`             // 该数据在决策时点可获得的时间
	SourceUpdatedAt time.Time `json:"source_updated_at"` // 数据本身的更新时间
	Hash            string    `json:"hash,omitempty"`    // 内容哈希, 用于检测静默变更
}

// DatasetSnapshot is a versioned, immutable snapshot of the data used in an
// experiment. It ensures reproducibility by pinning data version, date range,
// and universe (e.g. which stocks were included).
type DatasetSnapshot struct {
	ID              string          `json:"id"`
	Version         string          `json:"version"`          // data version tag
	DateRange       DateRange       `json:"date_range"`       // [start, end]
	Universe        []string        `json:"universe"`         // stock codes in scope
	Market          string          `json:"market"`           // SH / SZ / BJ / ALL
	PriceAdjustment PriceAdjustment `json:"price_adjustment"` // raw / forward / backward
	Description     string          `json:"description"`
	CreatedAt       time.Time       `json:"created_at"`
	// Sources 记录该快照包含的所有数据源明细, 用于血缘追溯.
	Sources []DataSource `json:"sources,omitempty"`
	// Frozen 标记快照已冻结 (不可变), 由系统在创建后自动设置.
	Frozen bool `json:"frozen,omitempty"`
	// ContentHash 是所有冻结数据分片清单的哈希。空值表示旧版仅元数据快照，
	// 不能作为可复现实验的数据输入。
	ContentHash string `json:"content_hash,omitempty"`
	// KlineManifests 记录冻结到快照内的真实 K 线分片。
	KlineManifests []KlineSnapshotManifest `json:"kline_manifests,omitempty"`
}

// KlineSnapshotManifest 描述快照内一只证券的一种 K 线数据分片。
type KlineSnapshotManifest struct {
	Code        string `json:"code"`
	KType       uint8  `json:"ktype"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	RowCount    int    `json:"row_count"`
	ContentHash string `json:"content_hash"`
}

// SnapshotKlineBar 是不可变快照内保存的原始 K 线记录。
type SnapshotKlineBar struct {
	Code   string    `json:"code"`
	KType  uint8     `json:"ktype"`
	Date   time.Time `json:"date"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
	Amount float64   `json:"amount"`
}

// DateRange is an inclusive date range.
type DateRange struct {
	Start string `json:"start"` // YYYY-MM-DD
	End   string `json:"end"`   // YYYY-MM-DD
}

// FeatureSet defines which features (indicators, signals) are computed for an
// experiment. It is versioned so that experiments can reference the exact
// feature specification used.
type FeatureSet struct {
	ID              string          `json:"id"`
	Version         string          `json:"version"`          // feature set version tag
	Indicators      []string        `json:"indicators"`       // e.g. ["MACD(12,26,9)", "RSI(14)", "MA(20)"]
	Signals         []string        `json:"signals"`          // e.g. ["golden_cross", "oversold"]
	Params          string          `json:"params"`           // JSON-serialized parameter map
	PriceAdjustment PriceAdjustment `json:"price_adjustment"` // 特征计算使用的价格口径
	Description     string          `json:"description"`
	CreatedAt       time.Time       `json:"created_at"`
}

// Freeze 冻结快照, 使其不可变. 冻结后 Source 列表不可再修改.
func (d *DatasetSnapshot) Freeze() {
	d.Frozen = true
}

// ValidateAsOf 校验所有数据源的 as-of 时间均不晚于 referenceDate.
// 用于确保实验在 T 日运行时, 所有数据都已在 T 日或之前可获得.
func (d *DatasetSnapshot) ValidateAsOf(referenceDate time.Time) error {
	for _, src := range d.Sources {
		if src.AsOf.After(referenceDate) {
			return fmt.Errorf("data source %q as_of (%s) is after reference date (%s), future data leak",
				src.Type, src.AsOf.Format("2006-01-02"), referenceDate.Format("2006-01-02"))
		}
	}
	return nil
}

// HasSource 检查是否包含指定类型的数据源.
func (d *DatasetSnapshot) HasSource(sourceType string) bool {
	for _, src := range d.Sources {
		if src.Type == sourceType {
			return true
		}
	}
	return false
}

// SourceVersions 返回所有数据源的版本摘要, 用于血缘追溯展示.
func (d *DatasetSnapshot) SourceVersions() map[string]string {
	m := make(map[string]string, len(d.Sources))
	for _, src := range d.Sources {
		m[src.Type] = src.Version
	}
	return m
}

// Experiment runs a hypothesis against a dataset and feature set.
// It produces one or more Candidate paradigms.
type Experiment struct {
	ID              string     `json:"id"`
	HypothesisID    string     `json:"hypothesis_id"`
	DatasetSnapshot string     `json:"dataset_snapshot_id"`
	FeatureSetID    string     `json:"feature_set_id"`
	RuleSet         string     `json:"rule_set"`       // JSON-serialized rule definition
	HoldingPeriod   string     `json:"holding_period"` // short / medium / long
	CostPerTrade    float64    `json:"cost_per_trade"` // as fraction
	Status          string     `json:"status"`         // running / completed / failed
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	// BoundSnapshotIDs 记录实验实际绑定的所有不可变快照 ID (含 DatasetSnapshot 和 FeatureSet 版本),
	// 用于确保重跑时不会静默读取更新后的数据.
	BoundSnapshotIDs []string `json:"bound_snapshot_ids,omitempty"`
}

// Candidate is a paradigm that has passed initial screening but has not yet
// undergone rigorous validation. It is created by an Experiment.
type Candidate struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experiment_id"`
	HypothesisID string    `json:"hypothesis_id"`
	RuleSet      string    `json:"rule_set"`     // JSON-serialized rule definition
	SampleSize   int       `json:"sample_size"`  // number of trades in initial screening
	GrossReturn  float64   `json:"gross_return"` // initial screening return (%)
	CreatedAt    time.Time `json:"created_at"`
}

// ParadigmVersion is a snapshot of a paradigm at a specific point in time.
// Every state change produces a new version, enabling full traceability.
type ParadigmVersion struct {
	ID         string    `json:"id"`
	ParadigmID string    `json:"paradigm_id"`
	Version    int       `json:"version"`   // monotonically increasing
	State      State     `json:"state"`     // current lifecycle state
	RuleSet    string    `json:"rule_set"`  // JSON-serialized rule definition
	Metrics    string    `json:"metrics"`   // JSON-serialized Metrics snapshot
	ParentID   string    `json:"parent_id"` // previous version ID (for chain)
	Reason     string    `json:"reason"`    // why this version was created
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  string    `json:"created_by"` // user or agent
}

// ValidationReport is the result of validating a paradigm against admission
// standards. It is linked to a specific ParadigmVersion and DatasetSnapshot.
type ValidationReport struct {
	ID                string    `json:"id"`
	ParadigmVersionID string    `json:"paradigm_version_id"`
	DatasetSnapshotID string    `json:"dataset_snapshot_id"`
	AdmissionResult   string    `json:"admission_result"` // JSON-serialized AdmissionResult
	Passed            bool      `json:"passed"`
	Level             string    `json:"level"` // platinum / gold / silver / bronze
	CreatedAt         time.Time `json:"created_at"`
}

// Signal represents a trading signal emitted by a paradigm during observation
// or forward-running. It is time-stamped and links back to the paradigm version
// that emitted it.
type Signal struct {
	ID                string          `json:"id"`
	ParadigmVersionID string          `json:"paradigm_version_id"`
	StockCode         string          `json:"stock_code"`
	Direction         string          `json:"direction"` // buy / sell
	TriggeredAt       time.Time       `json:"triggered_at"`
	Price             float64         `json:"price"` // 以 PriceAdjustment 口径计价的价格
	PriceAdjustment   PriceAdjustment `json:"price_adjustment"`
	Confidence        float64         `json:"confidence"` // 0-1
	CreatedAt         time.Time       `json:"created_at"`
}

// ForwardRun is a paper-trading or out-of-sample forward test of a paradigm.
// It tracks signals and P&L over time without real capital at risk.
type ForwardRun struct {
	ID                string    `json:"id"`
	ParadigmVersionID string    `json:"paradigm_version_id"`
	StartDate         string    `json:"start_date"`
	EndDate           string    `json:"end_date,omitempty"`
	Status            string    `json:"status"`       // active / completed / stopped
	CurrentPnl        float64   `json:"current_pnl"`  // running P&L (%)
	SignalCount       int       `json:"signal_count"` // total signals emitted
	CreatedAt         time.Time `json:"created_at"`
}

// Review is a human review of a paradigm at a specific stage.
// Reviews are required for promotion and can trigger relegation.
type Review struct {
	ID                string    `json:"id"`
	ParadigmVersionID string    `json:"paradigm_version_id"`
	Reviewer          string    `json:"reviewer"`
	Action            string    `json:"action"` // approve / reject / request_changes
	Rating            int       `json:"rating"` // 1-5
	Comment           string    `json:"comment"`
	RedFlags          []string  `json:"red_flags"` // e.g. ["survivorship_bias", "data_leakage"]
	CreatedAt         time.Time `json:"created_at"`
}

// ============================================================================
// Lifecycle State Machine
// ============================================================================

// State represents a paradigm lifecycle state.
type State string

const (
	StateHypothesis  State = "hypothesis"  // 研究假设
	StateExperiment  State = "experiment"  // 实验进行中
	StateCandidate   State = "candidate"   // 候选范式
	StateValidation  State = "validation"  // 验证中
	StateObservation State = "observation" // 前向观察
	StatePromoted    State = "promoted"    // 已晋级
	StateRelegated   State = "relegated"   // 降级
	StateRetired     State = "retired"     // 已淘汰
)

// ValidTransitions defines the allowed state transitions.
var ValidTransitions = map[State][]State{
	StateHypothesis:  {StateExperiment},
	StateExperiment:  {StateCandidate, StateRetired},
	StateCandidate:   {StateValidation, StateRetired},
	StateValidation:  {StateObservation, StateCandidate, StateRetired},
	StateObservation: {StatePromoted, StateRelegated, StateRetired},
	StatePromoted:    {StateRelegated, StateRetired},
	StateRelegated:   {StateObservation, StateRetired},
	StateRetired:     {}, // terminal state
}

// StateDescriptions provides human-readable descriptions for each state.
var StateDescriptions = map[State]string{
	StateHypothesis:  "研究假设：提出可测试的交易模式假设",
	StateExperiment:  "实验进行中：在历史数据上运行规则集",
	StateCandidate:   "候选范式：通过初步筛选，等待验证",
	StateValidation:  "验证中：进行正式证据评估",
	StateObservation: "前向观察：模拟前向运行，监控漂移",
	StatePromoted:    "已晋级：通过全部验证，可用于有限实盘",
	StateRelegated:   "降级：性能下降，退回观察",
	StateRetired:     "已淘汰：不再使用",
}

// ============================================================================
// State Machine Operations
// ============================================================================

// Transition represents a state transition event.
type Transition struct {
	From      State     `json:"from"`
	To        State     `json:"to"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
	By        string    `json:"by"`
}

// Lifecycle tracks the full state machine history of a paradigm.
type Lifecycle struct {
	ParadigmID string       `json:"paradigm_id"`
	Current    State        `json:"current_state"`
	History    []Transition `json:"history"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// CanTransition checks whether a transition from -> to is valid.
func CanTransition(from, to State) bool {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// TransitionTo attempts to transition the lifecycle to a new state.
// Returns an error if the transition is invalid.
func (l *Lifecycle) TransitionTo(to State, reason, by string) error {
	if !CanTransition(l.Current, to) {
		return fmt.Errorf("invalid transition: %s → %s (allowed: %v)", l.Current, to, ValidTransitions[l.Current])
	}

	t := Transition{
		From:      l.Current,
		To:        to,
		Reason:    reason,
		Timestamp: time.Now(),
		By:        by,
	}

	l.Current = to
	l.History = append(l.History, t)
	l.UpdatedAt = t.Timestamp
	return nil
}

// NewLifecycle creates a new lifecycle starting at the hypothesis state.
func NewLifecycle(paradigmID string) *Lifecycle {
	now := time.Now()
	return &Lifecycle{
		ParadigmID: paradigmID,
		Current:    StateHypothesis,
		History:    []Transition{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// IsTerminal returns true if the current state is terminal (retired).
func (l *Lifecycle) IsTerminal() bool {
	return l.Current == StateRetired
}

// IsActive returns true if the paradigm is in an active (non-terminal) state
// that can generate signals.
func (l *Lifecycle) IsActive() bool {
	switch l.Current {
	case StateObservation, StatePromoted:
		return true
	default:
		return false
	}
}

// ============================================================================
// Traceability
// ============================================================================

// TraceabilityChain links a paradigm back to its origin.
// Every derived result (experiment, candidate, validation report, forward run)
// can reference its parent objects for full auditability.
type TraceabilityChain struct {
	ParadigmVersionID  string `json:"paradigm_version_id"`
	ValidationReportID string `json:"validation_report_id,omitempty"`
	ExperimentID       string `json:"experiment_id,omitempty"`
	HypothesisID       string `json:"hypothesis_id,omitempty"`
	DatasetSnapshotID  string `json:"dataset_snapshot_id,omitempty"`
	FeatureSetID       string `json:"feature_set_id,omitempty"`
}

// Validate checks that the traceability chain is complete enough for the
// given state. For example, a promoted paradigm must have a validation report.
func (t *TraceabilityChain) Validate(state State) error {
	if t.ParadigmVersionID == "" {
		return errors.New("paradigm_version_id is required")
	}
	switch state {
	case StateExperiment:
		if t.HypothesisID == "" {
			return errors.New("hypothesis_id is required for experiment state")
		}
	case StateValidation:
		if t.DatasetSnapshotID == "" {
			return errors.New("dataset_snapshot_id is required for validation state")
		}
	case StatePromoted:
		if t.ValidationReportID == "" {
			return errors.New("validation_report_id is required for promoted state")
		}
	}
	return nil
}
