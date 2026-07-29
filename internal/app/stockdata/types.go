package stockdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// Adjustment 统一的价格口径枚举，贯穿研究、信号与收益计算。
// - Raw: 不复权（原始成交价格）
// - Forward: 前复权（历史价格向最新对齐，适合指标/图形）
// - Backward: 后复权（未来价格向最早对齐，适合累计收益）
type Adjustment string

const (
	AdjustmentRaw      Adjustment = "raw"
	AdjustmentForward  Adjustment = "forward"
	AdjustmentBackward Adjustment = "backward"
)

// Normalize 把空值/非法值归一为 raw。
func (a Adjustment) Normalize() Adjustment {
	switch a {
	case "", AdjustmentRaw:
		return AdjustmentRaw
	case AdjustmentForward:
		return AdjustmentForward
	case AdjustmentBackward:
		return AdjustmentBackward
	default:
		return AdjustmentRaw
	}
}

// String 人类可读描述。
func (a Adjustment) String() string {
	switch a.Normalize() {
	case AdjustmentRaw:
		return "不复权"
	case AdjustmentForward:
		return "前复权"
	case AdjustmentBackward:
		return "后复权"
	default:
		return "未知口径"
	}
}

type DataType string

const (
	DataKline   DataType = "kline"
	DataQuote   DataType = "quote"
	DataFinance DataType = "finance"
)

type ConsistencyMode string

const (
	RequireFresh ConsistencyMode = "require_fresh"
	AllowStale   ConsistencyMode = "allow_stale"
	CacheOnly    ConsistencyMode = "cache_only"
)

type DataSpec struct {
	Type        DataType
	Market      string
	Code        string
	Granularity string
	KType       uint8
	Adjustment  Adjustment
	Start       time.Time
	End         time.Time
}

type DataRequest struct {
	Spec         DataSpec
	Mode         ConsistencyMode
	ForceRefresh bool
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type Coverage struct {
	Exists          bool
	Start           time.Time
	End             time.Time
	Points          []time.Time
	SourceUpdatedAt time.Time
	LastSyncAt      time.Time
	Status          string
	Quality         string
}

type FreshnessDecision struct {
	Fresh         bool
	Reason        string
	MissingRanges []TimeRange
	AsOf          time.Time
}

type SyncRequest struct {
	Spec  DataSpec
	Range TimeRange
}

type SyncMetadata struct {
	SourceUpdatedAt time.Time
	Quality         string
}

type Dataset struct {
	Klines  []*protocol.Kline
	Quote   *protocol.QuoteItem
	Finance *protocol.FinanceInfo
}

type ResultMetadata struct {
	AsOf            time.Time   `json:"as_of"`
	Freshness       string      `json:"freshness"`
	Reason          string      `json:"reason"`
	SourceUpdatedAt time.Time   `json:"source_updated_at,omitempty"`
	SyncStatus      string      `json:"sync_status"`
	SyncedRanges    []TimeRange `json:"synced_ranges,omitempty"`
}

type DataResult struct {
	Klines   []*protocol.Kline     `json:"klines,omitempty"`
	Quote    *protocol.QuoteItem   `json:"quote,omitempty"`
	Finance  *protocol.FinanceInfo `json:"finance,omitempty"`
	Metadata ResultMetadata        `json:"metadata"`
}

type Provider interface {
	Sync(ctx context.Context, request SyncRequest) (Dataset, SyncMetadata, error)
}

type Repository interface {
	InspectCoverage(ctx context.Context, spec DataSpec) (Coverage, error)
	Query(ctx context.Context, spec DataSpec) (Dataset, error)
	SaveSynced(ctx context.Context, spec DataSpec, dataset Dataset, metadata SyncMetadata) error
}

type Clock interface {
	Now() time.Time
}

type TradingCalendar interface {
	IsTradingDay(ctx context.Context, market string, day time.Time) (bool, error)
}

type FreshnessPolicy interface {
	Evaluate(ctx context.Context, now time.Time, spec DataSpec, coverage Coverage) (FreshnessDecision, error)
}

type ErrorCode string

const (
	ErrInvalidRequest  ErrorCode = "validation_failed"
	ErrCacheMiss       ErrorCode = "cache_miss"
	ErrUpstream        ErrorCode = "upstream_unavailable"
	ErrUpstreamTimeout ErrorCode = "upstream_timeout"
	ErrPersistence     ErrorCode = "persistence_failed"
	ErrStaleData       ErrorCode = "stale_data"
)

type Error struct {
	Code ErrorCode
	Op   string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func CodeOf(err error) ErrorCode {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ErrPersistence
}
