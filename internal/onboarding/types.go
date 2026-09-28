// Package onboarding 是「首次引导」编排服务：把「同步行情 → 冻结快照 → 物化特征 →
// 运行选股」这条真实链路一次走完，让第一次使用的用户直接落到今日决策页，
// 而不是在入口页自己摸索每一步。
//
// 它不产生任何业务事实：每一步都调用既有的确定性引擎（快照构建器、特征引擎、
// 选股引擎）。数据缺失或未达门槛时 fail closed 并给出下一步，绝不伪造候选。
package onboarding

import "time"

// 步骤状态。每一步都必须回答「现在发生了什么」。
const (
	StepDone    = "done"    // 已完成
	StepSkipped = "skipped" // 已就绪，无需重复执行
	StepBlocked = "blocked" // 被真实条件阻断，附下一步
	StepFailed  = "failed"  // 执行失败
)

// 整体状态。
const (
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
	StatusFailed    = "failed"
)

// Step 是引导流程里的一步。
type Step struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// RunOptions 控制一次一键走通。
type RunOptions struct {
	// Date 目标交易日（YYYY-MM-DD）。空 = 库内真实最新交易日。
	Date string
	// Universe 股票池名称。空 = universe_usable。
	Universe string
	// CoverageThreshold 快照 ready 覆盖阈值 (0,1]。0 = 使用构建器默认。
	CoverageThreshold float64
	// MaxGappedCodes 允许的最多有缺口代码数。0 = 使用构建器默认。
	MaxGappedCodes int
	// Sync 是否在走通前触发真实行情同步。
	Sync bool
	// Force 是否忽略已有快照强制重建。
	Force bool
}

// Result 是一次一键走通的结果。
type Result struct {
	Status            string    `json:"status"`
	TradeDate         string    `json:"trade_date,omitempty"`
	SnapshotID        string    `json:"snapshot_id,omitempty"`
	FeatureSnapshotID string    `json:"feature_snapshot_id,omitempty"`
	SelectionRunID    string    `json:"selection_run_id,omitempty"`
	CandidateCount    int       `json:"candidate_count"`
	BuyCount          int       `json:"buy_count"`
	ScannedStocks     int       `json:"scanned_stocks"`
	EligibleMethods   int       `json:"eligible_methods"`
	Steps             []Step    `json:"steps"`
	BlockedReason     string    `json:"blocked_reason,omitempty"`
	FinishedAt        time.Time `json:"finished_at"`
}
