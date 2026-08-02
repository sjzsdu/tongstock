package positiondecision

import (
	"context"
	"errors"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/pkg/trading"
)

const EngineVersion = "position-decision-v1"

var ErrNotFound = errors.New("position decision not found")

type Link struct {
	TradeID         int64  `json:"trade_id"`
	Quantity        int    `json:"quantity"`
	SelectionRunID  string `json:"selection_run_id,omitempty"`
	MethodID        string `json:"method_id,omitempty"`
	MethodVersionID string `json:"method_version_id,omitempty"`
	BuyReason       string `json:"buy_reason,omitempty"`
}
type Fact struct {
	Kind   string `json:"kind"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}
type Decision struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Action          string   `json:"action"`
	Priority        string   `json:"priority"`
	Deadline        string   `json:"deadline"`
	Inferred        bool     `json:"inferred"`
	Executable      bool     `json:"executable"`
	Constraint      string   `json:"constraint,omitempty"`
	Quantity        int      `json:"quantity,omitempty"`
	Cost            float64  `json:"cost"`
	CurrentPrice    float64  `json:"current_price"`
	ReturnPct       float64  `json:"return_pct"`
	PriceTime       string   `json:"price_time"`
	SnapshotID      string   `json:"snapshot_id"`
	SelectionRunID  string   `json:"selection_run_id,omitempty"`
	MethodID        string   `json:"method_id,omitempty"`
	MethodVersionID string   `json:"method_version_id,omitempty"`
	Facts           []Fact   `json:"facts"`
	CounterEvidence []string `json:"counter_evidence,omitempty"`
	Explanation     string   `json:"explanation"`
}
type Run struct {
	ID                string     `json:"id"`
	RunHash           string     `json:"run_hash"`
	EngineVersion     string     `json:"engine_version"`
	SnapshotID        string     `json:"snapshot_id"`
	FeatureSnapshotID string     `json:"feature_snapshot_id"`
	SnapshotDate      string     `json:"snapshot_date"`
	Decisions         []Decision `json:"decisions"`
	CreatedAt         time.Time  `json:"created_at"`
}
type Request struct {
	MarketSnapshotID  string
	FeatureSnapshotID string
}

type SnapshotRepository interface {
	LoadMarketSnapshot(string, bool) (*marketsnapshot.MarketSnapshot, error)
	LoadFeatureSnapshot(string, bool) (*marketsnapshot.FeatureSnapshot, error)
	ListFeatureSnapshots(string) ([]*marketsnapshot.FeatureSnapshot, error)
}
type TradeRepository interface {
	GetAllPositions() ([]trading.Trade, error)
}
type MethodRepository interface {
	Get(context.Context, string) (*methodregistry.Method, error)
}
type Repository interface {
	GetLink(context.Context, int64) (Link, error)
	SaveLink(context.Context, Link) error
	Save(context.Context, *Run) error
	Get(context.Context, string) (*Run, error)
	List(context.Context, string, int) ([]*Run, error)
}
