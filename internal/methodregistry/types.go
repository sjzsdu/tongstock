// Package methodregistry owns the product-facing investment method lifecycle.
// Experiments and validations are evidence inputs, never the aggregate root.
package methodregistry

import (
	"context"
	"errors"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

var ErrNotFound = errors.New("investment method not found")

type Status string

const (
	StatusDraft     Status = "draft"
	StatusCandidate Status = "candidate"
	StatusVerified  Status = "verified"
	StatusObserving Status = "observing"
	StatusDegraded  Status = "degraded"
	StatusRetired   Status = "retired"
	StatusRejected  Status = "rejected"
)

type EvidenceSummary struct {
	ResultHash          string   `json:"result_hash"`
	SnapshotID          string   `json:"snapshot_id"`
	JobHash             string   `json:"job_hash"`
	Confidence          string   `json:"confidence"`
	ConfidenceReason    string   `json:"confidence_reason,omitempty"`
	Passable            bool     `json:"passable"`
	OOSTrades           int      `json:"oos_trades"`
	OOSReturn           float64  `json:"oos_return"`
	OOSWinRate          float64  `json:"oos_win_rate"`
	OutcomeHitRate      *float64 `json:"outcome_hit_rate,omitempty"`
	OutcomeObservations int      `json:"outcome_observations,omitempty"`
	OOSMaxDrawdown      float64  `json:"oos_max_drawdown"`
	// SharpeRatio / SortinoRatio 来自样本外回测的已计算指标；旧数据为 nil，
	// 前端必须把缺失如实展示为「待验证」，不得编默认值。
	SharpeRatio  *float64 `json:"sharpe_ratio,omitempty"`
	SortinoRatio *float64 `json:"sortino_ratio,omitempty"`
	// UniverseSize 是样本外验证实际覆盖的股票数；0 = 历史数据未记录。
	UniverseSize int `json:"universe_size,omitempty"`
	// ValidationStart / ValidationEnd 是样本外验证窗口（保留区间）的日期边界；
	// 空 = 历史数据未记录，前端不得推测。
	ValidationStart string `json:"validation_start,omitempty"`
	ValidationEnd   string `json:"validation_end,omitempty"`
}

type MethodVersion struct {
	ID               string                  `json:"id"`
	Version          int                     `json:"version"`
	MethodHash       string                  `json:"method_hash"`
	CompilerVersion  string                  `json:"compiler_version"`
	SourceResearchID string                  `json:"source_research_id,omitempty"`
	ValidationJobID  string                  `json:"validation_job_id,omitempty"`
	Method           *methods.CompiledMethod `json:"method"`
	Evidence         *EvidenceSummary        `json:"evidence,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
}

type Method struct {
	ID               string          `json:"id"`
	FamilyID         string          `json:"family_id"`
	VariantID        string          `json:"variant_id"`
	Name             string          `json:"name"`
	Status           Status          `json:"status"`
	Market           string          `json:"market"`
	Universe         string          `json:"universe"`
	HoldingMinDays   int             `json:"holding_min_days"`
	HoldingMaxDays   int             `json:"holding_max_days"`
	TriggerFrequency string          `json:"trigger_frequency"`
	EntrySummary     string          `json:"entry_summary"`
	ExitSummary      string          `json:"exit_summary"`
	Invalidations    []string        `json:"invalidations,omitempty"`
	CurrentVersion   int             `json:"current_version"`
	Versions         []MethodVersion `json:"versions"`
	Health           *HealthState    `json:"health,omitempty"`
	Annotations      []Annotation    `json:"annotations,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type Annotation struct {
	Text      string    `json:"text"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}
type HealthState struct {
	Score              float64   `json:"score"`
	ForwardSamples     int       `json:"forward_samples"`
	Drift              bool      `json:"drift"`
	Decay              bool      `json:"decay"`
	ExecutionDeviation bool      `json:"execution_deviation"`
	CriticalAlerts     int       `json:"critical_alerts"`
	ConsecutiveSevere  int       `json:"consecutive_severe"`
	EvidenceHash       string    `json:"evidence_hash"`
	AsOf               time.Time `json:"as_of"`
}
type AuditEvent struct {
	ID           string    `json:"id"`
	MethodID     string    `json:"method_id"`
	From         Status    `json:"from"`
	To           Status    `json:"to"`
	Action       string    `json:"action"`
	Reason       string    `json:"reason"`
	Actor        string    `json:"actor"`
	EvidenceHash string    `json:"evidence_hash,omitempty"`
	Automatic    bool      `json:"automatic"`
	CreatedAt    time.Time `json:"created_at"`
}

type Registration struct {
	FamilyID  string
	VariantID string
	// Name 可选：覆盖方法显示名（如自动研究给模板名附上持有期与来源前缀）。
	// 空 = 使用 compiled method 自带名称。
	Name                string
	SourceResearchID    string
	ValidationJobID     string
	OutcomeHitRate      *float64
	OutcomeObservations int
	Market              string
	TriggerFrequency    string
	EntrySummary        string
	ExitSummary         string
	Invalidations       []string
	Method              *methods.CompiledMethod
	Evidence            Evidence
}
type Evidence interface{ RegistryEvidence() EvidenceInput }
type EvidenceInput struct {
	ResultHash, ComputedHash, SnapshotID, JobHash, MethodHash, StockCode, Confidence string
	ConfidenceReason                                                                 string
	Passable                                                                         bool
	HasHardBlocker                                                                   bool
	OOSTrades                                                                        int
	OOSReturn, OOSWinRate, OOSMaxDrawdown                                            float64
	OutcomeHitRate                                                                   *float64
	OutcomeObservations                                                              int
	SharpeRatio, SortinoRatio                                                        *float64
	UniverseSize                                                                     int
	ValidationStart, ValidationEnd                                                   string
}

type Query struct {
	Status []Status
	// IDs 按方法 ID 批量过滤；nil/空 = 不过滤（保持向后兼容）。
	IDs            []string
	Market         string
	Universe       string
	HoldingMinDays *int
	HoldingMaxDays *int
	FamilyID       string
	Limit          int
}
type Card struct {
	ID               string              `json:"id"`
	FamilyID         string              `json:"family_id"`
	VariantID        string              `json:"variant_id"`
	Name             string              `json:"name"`
	Status           Status              `json:"status"`
	Market           string              `json:"market"`
	Universe         string              `json:"universe"`
	Scope            methods.Scope       `json:"scope,omitempty"`
	Outcome          methods.OutcomeRule `json:"outcome,omitempty"`
	Rules            *CardRules          `json:"rules,omitempty"`
	TriggerFrequency string              `json:"trigger_frequency"`
	HoldingPeriod    string              `json:"holding_period"`
	EntrySummary     string              `json:"entry_summary"`
	ExitSummary      string              `json:"exit_summary"`
	Invalidations    []string            `json:"invalidations,omitempty"`
	// SourceResearchID 是产生当前版本的自动研究批次；旧证据缺验证窗口/池大小时，
	// 前端可用它去批次记录回查（数据只存在于 batch result 上）。
	SourceResearchID string           `json:"source_research_id,omitempty"`
	Evidence         *EvidenceSummary `json:"evidence,omitempty"`
	Health           *HealthState     `json:"health,omitempty"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// CardRules 把编译产物里的可执行规则原样暴露给前端，让人能读到方法
// 真正怎么判定入场/退出/失效。nil = 该版本没有编译产物（历史数据）。
type CardRules struct {
	EntryRule   *methods.Expr        `json:"entry_rule,omitempty"`
	ExitRule    *methods.Expr        `json:"exit_rule,omitempty"`
	InvalidRule *methods.Expr        `json:"invalid_rule,omitempty"`
	Position    *methods.PosRule     `json:"position,omitempty"`
	Holding     *methods.HoldingRule `json:"holding,omitempty"`
}

type Repository interface {
	Save(context.Context, *Method, AuditEvent) error
	Get(context.Context, string) (*Method, error)
	Query(context.Context, Query) ([]*Method, error)
	ListAudit(context.Context, string) ([]AuditEvent, error)
}
