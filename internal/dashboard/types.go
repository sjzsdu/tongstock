// Package dashboard 是「今日状态」读模型：把分散在市场快照、方法库、选股运行、
// 持仓判断里的真实事实聚合成一屏可读的状态，并给出空榜单的真实原因与下一步。
//
// 它不产生任何新的业务事实，只读取已存在的确定性结果，因此不会引入
// 演示数据、随机分数或伪造的推荐。
package dashboard

import (
	"time"

	"github.com/sjzsdu/tongstock/internal/selection"
)

// 空榜单原因。每一种都必须回答两个问题：为什么空、现在该点哪。
const (
	// ReasonDataNotSynced 行情数据尚未同步到最新交易日。
	ReasonDataNotSynced = "data_not_synced"
	// ReasonNoVerifiedMethods 方法库里还没有通过真实回测验证的方法。
	ReasonNoVerifiedMethods = "no_verified_methods"
	// ReasonSelectionNotRun 数据与方法都就绪，但还没有跑过选股。
	ReasonSelectionNotRun = "selection_not_run"
	// ReasonNoCandidates 已扫描但没有方法通过证据门槛，这是正常结果。
	ReasonNoCandidates = "no_candidates"
	// ReasonHasCandidates 有候选，不需要空状态。
	ReasonHasCandidates = "has_candidates"
)

// 下一步动作类型。前端据此决定点击行为。
const (
	ActionSync           = "sync"            // 触发一键走通（同步行情）
	ActionSeedMethods    = "seed_methods"    // 载入内置示例方法并验证
	ActionRunSelection   = "run_selection"   // 直接运行今日选股
	ActionViewExclusions = "view_exclusions" // 查看排除原因
	ActionResearch       = "research"        // 去方法研究
)

// Action 是一个可点击的「下一步」。
type Action struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	To      string `json:"to,omitempty"`
	Primary bool   `json:"primary"`
}

// EmptyState 描述当前首屏为什么没有候选，以及下一步。
type EmptyState struct {
	Reason  string   `json:"reason"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Actions []Action `json:"actions"`
}

// DataStatus 是行情数据新鲜度。
type DataStatus struct {
	LatestKlineDate    string  `json:"latest_kline_date,omitempty"`
	LatestSnapshotDate string  `json:"latest_snapshot_date,omitempty"`
	LatestSnapshotID   string  `json:"latest_snapshot_id,omitempty"`
	SnapshotStatus     string  `json:"snapshot_status,omitempty"`
	SnapshotFrozen     bool    `json:"snapshot_frozen"`
	CoveragePct        float64 `json:"coverage_pct"`
	ReadyCodes         int     `json:"ready_codes"`
	ExpectedCodes      int     `json:"expected_codes"`
	Fresh              bool    `json:"fresh"`
	Detail             string  `json:"detail"`
}

// MethodStatus 是方法库状态。
type MethodStatus struct {
	Total     int            `json:"total"`
	Verified  int            `json:"verified"`
	Observing int            `json:"observing"`
	Candidate int            `json:"candidate"`
	Rejected  int            `json:"rejected"`
	Degraded  int            `json:"degraded"`
	Retired   int            `json:"retired"`
	ByStatus  map[string]int `json:"by_status"`
}

// SelectionStatus 是最近一次选股运行的摘要（含工作日志口径）。
type SelectionStatus struct {
	RunID             string                `json:"run_id"`
	SnapshotID        string                `json:"snapshot_id"`
	FeatureSnapshotID string                `json:"feature_snapshot_id"`
	SnapshotDate      string                `json:"snapshot_date"`
	Status            string                `json:"status"`
	CreatedAt         time.Time             `json:"created_at"`
	ScannedStocks     int                   `json:"scanned_stocks"`
	EligibleMethods   int                   `json:"eligible_methods"`
	CandidateCount    int                   `json:"candidate_count"`
	BuyCount          int                   `json:"buy_count"`
	ActionCounts      map[string]int        `json:"action_counts"`
	ExclusionCounts   map[string]int        `json:"exclusion_counts"`
	SampleExclusions  []selection.Exclusion `json:"sample_exclusions"`
	Candidates        []selection.Candidate `json:"candidates"`
}

// WorkLogLine 是工作日志里的一行。
type WorkLogLine struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  int    `json:"value"`
	Detail string `json:"detail,omitempty"`
	Tone   string `json:"tone"` // neutral / positive / warning
}

// WorkLog 把一次运行翻译成「系统真的在干活」的实况，消除「是不是坏了」的焦虑。
type WorkLog struct {
	Available    bool          `json:"available"`
	SnapshotDate string        `json:"snapshot_date,omitempty"`
	Lines        []WorkLogLine `json:"lines"`
}

// PositionStatus 是持仓状态。
type PositionStatus struct {
	HoldingCount   int    `json:"holding_count"`
	UrgentActions  int    `json:"urgent_actions"`
	LatestRunDate  string `json:"latest_run_date,omitempty"`
	HasDecisionRun bool   `json:"has_decision_run"`
}

// Signal 是健康面板上的一个信号灯。
type Signal struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status"` // ok / attention / blocked
	Detail string `json:"detail,omitempty"`
}

// Health 是首屏一行式系统健康总览。
type Health struct {
	Overall string   `json:"overall"` // ok / attention / blocked
	Signals []Signal `json:"signals"`
}

// Today 是 GET /api/dashboard/today 的响应。
type Today struct {
	AsOf      time.Time        `json:"as_of"`
	Data      DataStatus       `json:"data"`
	Methods   MethodStatus     `json:"methods"`
	Selection *SelectionStatus `json:"selection,omitempty"`
	Positions PositionStatus   `json:"positions"`
	Empty     EmptyState       `json:"empty_state"`
	WorkLog   WorkLog          `json:"work_log"`
	Health    Health           `json:"health"`
}
