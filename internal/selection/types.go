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
	// selection-v2: the scope resolver no longer rejects methods on the legacy
	// universe label, so cached v1 runs (full universe_mismatch exclusions)
	// must never be re-served from the run cache.
	// selection-v3: the missing-features gate now covers every AST-referenced
	// indicator (including builtins like gap_pct); cached v2 runs could hold
	// silent all-miss results with zero exclusions.
	// selection-v4: one method can contribute at most BuyCapPerMethod buy
	// candidates per run; cached v3 runs were produced without that cap and
	// would be re-served for identical inputs, silently ignoring the limit.
	//
	// This constant is an input of the run idempotency hash (see
	// runHashForVersion), so bumping it is what retires old cached runs:
	// a v3 record is stored under a different run_hash and can never be hit
	// again. Do not remove it from the hash input.
	EngineVersion          = "selection-v4"
	ActionBuy              = "buy"
	ActionWatch            = "watch"
	ActionAvoid            = "avoid"
	ActionInsufficientData = "insufficient_data"
)

type Request struct {
	MarketSnapshotID  string `json:"market_snapshot_id"`
	FeatureSnapshotID string `json:"feature_snapshot_id,omitempty"`
	// MethodIDs 限定本次选股只跑这些方法；nil/空 = 全部 eligible 方法（向后兼容）。
	MethodIDs []string `json:"method_ids,omitempty"`
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
	// KeptByMethodIDs 记录把它计入各自方法内前 N 名（名额内）的方法 ID。
	// 只有非空时才可能落在 buy；与 TruncatedByMethodIDs 一起让「为什么是 buy」
	// 可复核：同一个候选可能被一个方法保留、被另一个方法截断。
	KeptByMethodIDs []string `json:"kept_by_method_ids,omitempty"`
	// TruncatedByMethodIDs 记录触发了它、却因为方法内名额已满而把它排在外面的
	// 方法 ID（不影响 Score，也不影响其它方法的判定）。
	TruncatedByMethodIDs []string `json:"truncated_by_method_ids,omitempty"`
	// TruncatedFromBuy 表示该候选本已达到 buy 门槛（score + 完整退出计划），
	// 只因所有触发它的方法名额都被更靠前的候选占满而降级为 watch。
	TruncatedFromBuy bool   `json:"truncated_from_buy,omitempty"`
	Explanation      string `json:"explanation"`
}

type Exclusion struct {
	MethodID   string `json:"method_id,omitempty"`
	Code       string `json:"code,omitempty"`
	ReasonCode string `json:"reason_code"`
	Detail     string `json:"detail"`
}

type Run struct {
	ID                string `json:"id"`
	RunHash           string `json:"run_hash"`
	EngineVersion     string `json:"engine_version"`
	SnapshotID        string `json:"snapshot_id"`
	FeatureSnapshotID string `json:"feature_snapshot_id"`
	SnapshotDate      string `json:"snapshot_date"`
	Status            string `json:"status"`
	EligibleMethods   int    `json:"eligible_methods"`
	ScannedStocks     int    `json:"scanned_stocks"`
	CandidateCount    int    `json:"candidate_count"`
	BuyCount          int    `json:"buy_count"`
	// BuyCapPerMethod 是本次 run 生效的「每个方法最多贡献几个 buy 候选」上限。
	// 旧 run（该字段出现之前）反序列化为 0，历史产物因此保持可读。
	BuyCapPerMethod int `json:"buy_cap_per_method,omitempty"`
	// BuyCountByMethod 是逐方法的名额使用情况：键为方法 ID，值为该方法保住的
	// buy 候选数（每项 ≤ BuyCapPerMethod）。没有 buy 的 run 不带该字段。
	BuyCountByMethod map[string]int `json:"buy_count_by_method,omitempty"`
	// TruncatedFromBuy 统计本次因为方法内名额已满而从 buy 降级为 watch 的候选数。
	TruncatedFromBuy int            `json:"truncated_from_buy,omitempty"`
	ActionCounts     map[string]int `json:"action_counts"`
	Candidates       []Candidate    `json:"candidates"`
	Exclusions       []Exclusion    `json:"exclusions"`
	// RequestedMethodIDs 记录本次请求明确指定的方法（空 = 全量运行），
	// 让「结果与所选方法」在持久化层面可追溯。
	RequestedMethodIDs []string  `json:"requested_method_ids,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type SnapshotRepository interface {
	LoadMarketSnapshot(string, bool) (*marketsnapshot.MarketSnapshot, error)
	LoadFeatureSnapshot(string, bool) (*marketsnapshot.FeatureSnapshot, error)
	ListFeatureSnapshots(string) ([]*marketsnapshot.FeatureSnapshot, error)
}

// FeatureEnsurer 保证给定市场快照存在覆盖最新物化特征集的特征快照：
// 库内只有旧特征集快照（如升级前缺 gap_pct 的 22 特征集）时就地重建，
// 避免方法整批 insufficient_data。由 onboarding/物化器注入（可选）。
type FeatureEnsurer interface {
	EnsureFeatureSnapshot(ctx context.Context, market *marketsnapshot.MarketSnapshot) (*marketsnapshot.FeatureSnapshot, error)
}

type MethodRepository interface {
	Query(context.Context, methodregistry.Query) ([]*methodregistry.Method, error)
}

type Repository interface {
	Save(context.Context, *Run) error
	Get(context.Context, string, string) (*Run, error)
	List(context.Context, string, string, int) ([]*Run, error)
}
