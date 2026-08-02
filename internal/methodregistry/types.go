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
	ResultHash     string  `json:"result_hash"`
	SnapshotID     string  `json:"snapshot_id"`
	JobHash        string  `json:"job_hash"`
	Confidence     string  `json:"confidence"`
	Passable       bool    `json:"passable"`
	OOSTrades      int     `json:"oos_trades"`
	OOSReturn      float64 `json:"oos_return"`
	OOSWinRate     float64 `json:"oos_win_rate"`
	OOSMaxDrawdown float64 `json:"oos_max_drawdown"`
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
	FamilyID         string
	VariantID        string
	SourceResearchID string
	ValidationJobID  string
	Market           string
	TriggerFrequency string
	EntrySummary     string
	ExitSummary      string
	Invalidations    []string
	Method           *methods.CompiledMethod
	Evidence         Evidence
}
type Evidence interface{ RegistryEvidence() EvidenceInput }
type EvidenceInput struct {
	ResultHash, ComputedHash, SnapshotID, JobHash, MethodHash, StockCode, Confidence string
	Passable                                                                         bool
	HasHardBlocker                                                                   bool
	OOSTrades                                                                        int
	OOSReturn, OOSWinRate, OOSMaxDrawdown                                            float64
}

type Query struct {
	Status         []Status
	Market         string
	Universe       string
	HoldingMinDays *int
	HoldingMaxDays *int
	FamilyID       string
	Limit          int
}
type Card struct {
	ID               string           `json:"id"`
	FamilyID         string           `json:"family_id"`
	VariantID        string           `json:"variant_id"`
	Name             string           `json:"name"`
	Status           Status           `json:"status"`
	Market           string           `json:"market"`
	Universe         string           `json:"universe"`
	TriggerFrequency string           `json:"trigger_frequency"`
	HoldingPeriod    string           `json:"holding_period"`
	EntrySummary     string           `json:"entry_summary"`
	ExitSummary      string           `json:"exit_summary"`
	Invalidations    []string         `json:"invalidations,omitempty"`
	Evidence         *EvidenceSummary `json:"evidence,omitempty"`
	Health           *HealthState     `json:"health,omitempty"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type Repository interface {
	Save(context.Context, *Method, AuditEvent) error
	Get(context.Context, string) (*Method, error)
	Query(context.Context, Query) ([]*Method, error)
	ListAudit(context.Context, string) ([]AuditEvent, error)
}
