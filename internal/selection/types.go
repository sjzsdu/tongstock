// Package selection turns trusted methods and immutable market data into
// deterministic, auditable daily buy candidates. It never asks an LLM to
// create scores or fill missing facts.
package selection

import (
	"context"
	"errors"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
)

var ErrNotFound = errors.New("daily selection run not found")

const (
	EngineVersion          = "selection-v1"
	ActionBuy              = "buy"
	ActionWatch            = "watch"
	ActionAvoid            = "avoid"
	ActionInsufficientData = "insufficient_data"
)

type Request struct {
	MarketSnapshotID  string `json:"market_snapshot_id"`
	FeatureSnapshotID string `json:"feature_snapshot_id,omitempty"`
}

type Trigger struct {
	MethodID        string                          `json:"method_id"`
	MethodVersionID string                          `json:"method_version_id"`
	MethodName      string                          `json:"method_name"`
	FamilyID        string                          `json:"family_id"`
	Evidence        *methodregistry.EvidenceSummary `json:"evidence"`
	Facts           []TriggerFact                   `json:"facts"`
	Score           float64                         `json:"score"`
}

type TriggerFact struct {
	Path   string `json:"path"`
	Rule   string `json:"rule,omitempty"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type ExitPlan struct {
	MaxHoldingDays int      `json:"max_holding_days,omitempty"`
	StopLossPct    *float64 `json:"stop_loss_pct,omitempty"`
	TakeProfitPct  *float64 `json:"take_profit_pct,omitempty"`
	HasRule        bool     `json:"has_rule"`
	Complete       bool     `json:"complete"`
}

type Candidate struct {
	Rank              int       `json:"rank"`
	Code              string    `json:"code"`
	Action            string    `json:"action"`
	Score             float64   `json:"score"`
	DataDate          string    `json:"data_date"`
	SnapshotID        string    `json:"snapshot_id"`
	FeatureSnapshotID string    `json:"feature_snapshot_id"`
	Triggers          []Trigger `json:"triggers"`
	PositionCapPct    float64   `json:"position_cap_pct"`
	BuyWindow         string    `json:"buy_window"`
	Exit              ExitPlan  `json:"exit"`
	Invalidations     []string  `json:"invalidations,omitempty"`
	Risks             []string  `json:"risks,omitempty"`
	Explanation       string    `json:"explanation"`
}

type Exclusion struct {
	MethodID   string `json:"method_id,omitempty"`
	Code       string `json:"code,omitempty"`
	ReasonCode string `json:"reason_code"`
	Detail     string `json:"detail"`
}

type Run struct {
	ID                string         `json:"id"`
	RunHash           string         `json:"run_hash"`
	EngineVersion     string         `json:"engine_version"`
	SnapshotID        string         `json:"snapshot_id"`
	FeatureSnapshotID string         `json:"feature_snapshot_id"`
	SnapshotDate      string         `json:"snapshot_date"`
	Status            string         `json:"status"`
	EligibleMethods   int            `json:"eligible_methods"`
	ScannedStocks     int            `json:"scanned_stocks"`
	CandidateCount    int            `json:"candidate_count"`
	BuyCount          int            `json:"buy_count"`
	ActionCounts      map[string]int `json:"action_counts"`
	Candidates        []Candidate    `json:"candidates"`
	Exclusions        []Exclusion    `json:"exclusions"`
	CreatedAt         time.Time      `json:"created_at"`
}

type SnapshotRepository interface {
	LoadMarketSnapshot(string, bool) (*marketsnapshot.MarketSnapshot, error)
	LoadFeatureSnapshot(string, bool) (*marketsnapshot.FeatureSnapshot, error)
	ListFeatureSnapshots(string) ([]*marketsnapshot.FeatureSnapshot, error)
}

type MethodRepository interface {
	Query(context.Context, methodregistry.Query) ([]*methodregistry.Method, error)
}

type Repository interface {
	Save(context.Context, *Run) error
	Get(context.Context, string, string) (*Run, error)
	List(context.Context, string, string, int) ([]*Run, error)
}
