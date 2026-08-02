// Package validation 实现统一验证工厂：接收已编译方法版本，自动在冻结真实数据上
// 执行严格回测、统计检验和 critic 反证，输出 EvidenceBundle 和 promotion blockers。
//
// 设计原则：
//   - 确定性：相同快照 + 方法版本 → 相同结果哈希
//   - Fail closed：缺失真实数据或制品时拒绝产出
//   - 无窥探：训练/验证/样本外严格隔离，purge/embargo 强制执行
//   - 多重检验：根据搜索尝试次数施加数据窥探惩罚
package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ============================================================================
// ValidationJob — 验证任务定义
// ============================================================================

// ValidationJob 是验证工厂的输入：一个已编译方法 + 数据范围 + 执行约束。
// 用户无需提供回测参数；所有未设置字段使用默认严格配置。
type ValidationJob struct {
	// MethodHash 已编译方法的内容哈希 (methods.CompiledMethod.ContentHash)
	MethodHash string `json:"method_hash"`
	// MethodName 方法名称（用于报告）
	MethodName string `json:"method_name"`
	// SnapshotID 含冻结真实 K 线的不可变数据快照 ID
	SnapshotID string `json:"snapshot_id"`
	// StockCode 单股验证时的代码（空=全 universe）
	StockCode string `json:"stock_code,omitempty"`
	// DateStart 数据起始日（空=快照内最早）
	DateStart string `json:"date_start,omitempty"`
	// DateEnd 数据截止日（空=快照内最新）
	DateEnd string `json:"date_end,omitempty"`
	// SplitType 切分类型: "fixed" / "walk_forward"（空=默认 walk_forward）
	SplitType string `json:"split_type,omitempty"`
	// DiscoveryTrials 发现阶段的搜索尝试次数（用于多重检验惩罚，0=不惩罚）
	DiscoveryTrials int `json:"discovery_trials,omitempty"`
	// BenchmarkCode 基准代码（空=同标的买入持有基线）
	BenchmarkCode string `json:"benchmark_code,omitempty"`
	// InitialCash 初始资金（0=1,000,000）
	InitialCash float64 `json:"initial_cash,omitempty"`
}

// Validate 基本字段校验。
func (j *ValidationJob) Validate() error {
	if j.MethodHash == "" {
		return fmt.Errorf("method_hash is required")
	}
	if j.SnapshotID == "" {
		return fmt.Errorf("snapshot_id is required")
	}
	return nil
}

// JobHash 计算任务配置的确定性哈希（不含运行时状态）。
func (j *ValidationJob) JobHash() string {
	payload := struct {
		MethodHash      string  `json:"mh"`
		SnapshotID      string  `json:"sid"`
		StockCode       string  `json:"sc,omitempty"`
		DateStart       string  `json:"ds,omitempty"`
		DateEnd         string  `json:"de,omitempty"`
		SplitType       string  `json:"st,omitempty"`
		DiscoveryTrials int     `json:"dt"`
		BenchmarkCode   string  `json:"bc,omitempty"`
		InitialCash     float64 `json:"ic,omitempty"`
	}{
		MethodHash: j.MethodHash, SnapshotID: j.SnapshotID, StockCode: j.StockCode,
		DateStart: j.DateStart, DateEnd: j.DateEnd, SplitType: j.SplitType,
		DiscoveryTrials: j.DiscoveryTrials, BenchmarkCode: j.BenchmarkCode,
		InitialCash: j.InitialCash,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ============================================================================
// EvidenceBundle — 验证工厂的统一输出
// ============================================================================

// ConfidenceLevel 置信等级。
type ConfidenceLevel string

const (
	ConfidenceRejected     ConfidenceLevel = "rejected"     // 拒绝：硬门槛未过
	ConfidenceWeak         ConfidenceLevel = "weak"         // 弱：部分指标达标但不充分
	ConfidenceModerate     ConfidenceLevel = "moderate"     // 中等：样本外有效但稳定性不足
	ConfidenceStrong       ConfidenceLevel = "strong"       // 强：多维度证据一致
	ConfidenceInsufficient ConfidenceLevel = "insufficient" // 数据不足
)

// PromotionBlocker 阻止方法晋级的硬门槛。
type PromotionBlocker struct {
	Code        string `json:"code"`
	Dimension   string `json:"dimension"` // sample_size / oos_performance / cost / stability / data_leak / multiple_testing / concentration
	Severity    string `json:"severity"`  // hard / soft
	Description string `json:"description"`
}

// TradeRecord 交易级记录（从 backtest.CompletedTrade 映射）。
type TradeRecord struct {
	Code              string  `json:"code"`
	BuySignalDate     string  `json:"buy_signal_date"`
	BuyExecutionDate  string  `json:"buy_execution_date"`
	SellSignalDate    string  `json:"sell_signal_date"`
	SellExecutionDate string  `json:"sell_execution_date"`
	Quantity          int     `json:"quantity"`
	BuyPrice          float64 `json:"buy_price"`
	SellPrice         float64 `json:"sell_price"`
	GrossPnL          float64 `json:"gross_pnl"`
	NetPnL            float64 `json:"net_pnl"`
	TotalCost         float64 `json:"total_cost"`
	ReturnPct         float64 `json:"return_pct"`
}

// PerformanceStats 交易级和资金曲线级统计指标。
type PerformanceStats struct {
	// 交易级
	TotalTrades  int     `json:"total_trades"`
	WinRate      float64 `json:"win_rate"`
	AvgWin       float64 `json:"avg_win"`
	AvgLoss      float64 `json:"avg_loss"`
	ProfitFactor float64 `json:"profit_factor"` // 总盈利/总亏损
	AvgHoldDays  float64 `json:"avg_hold_days"`

	// 资金曲线级
	TotalReturn  float64 `json:"total_return"`
	AnnualReturn float64 `json:"annual_return"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	SharpeRatio  float64 `json:"sharpe_ratio"`
	SortinoRatio float64 `json:"sortino_ratio"`
	CalmarRatio  float64 `json:"calmar_ratio"`

	// 基准超额
	BenchmarkReturn float64 `json:"benchmark_return"`
	ExcessReturn    float64 `json:"excess_return"`
	Alpha           float64 `json:"alpha"`
	Beta            float64 `json:"beta"`

	// 成本
	TotalCost float64 `json:"total_cost"`
	CostRatio float64 `json:"cost_ratio"` // 总成本/总交易额

	// 置信区间 (bootstrap)
	ReturnCI95Low  float64 `json:"return_ci95_low"`
	ReturnCI95High float64 `json:"return_ci95_high"`
}

// SegmentResult 单个切分段（训练/验证/样本外）的回测结果。
type SegmentResult struct {
	Segment string           `json:"segment"` // train/valid/test
	Start   string           `json:"start"`
	End     string           `json:"end"`
	Stats   PerformanceStats `json:"stats"`
	Trades  []TradeRecord    `json:"trades,omitempty"`
}

// EvidenceBundle 是验证工厂的统一输出，包含所有证据。
type EvidenceBundle struct {
	JobHash     string    `json:"job_hash"`
	MethodHash  string    `json:"method_hash"`
	MethodName  string    `json:"method_name"`
	SnapshotID  string    `json:"snapshot_id"`
	GeneratedAt time.Time `json:"generated_at"`

	// 各段回测结果
	Segments []SegmentResult `json:"segments"`

	// 汇总统计（样本外为主）
	OosStats PerformanceStats `json:"oos_stats"`

	// 多重检验
	DiscoveryTrials int     `json:"discovery_trials"`
	BonferroniAlpha float64 `json:"bonferroni_alpha,omitempty"` // 0.05 / trials
	AdjustedPValue  float64 `json:"adjusted_p_value,omitempty"`

	// Critic 反证
	CriticIssues []CriticIssue `json:"critic_issues,omitempty"`

	// 最终判定
	Confidence ConfidenceLevel    `json:"confidence"`
	Blockers   []PromotionBlocker `json:"blockers,omitempty"`
	Passable   bool               `json:"passable"` // true = 可晋级到方法库

	// 可复现性
	ResultHash string `json:"result_hash"`
}

// CriticIssue 来自 ai_critic 的单项反证。
type CriticIssue struct {
	Dimension string `json:"dimension"`
	Severity  string `json:"severity"` // hard / soft
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// ComputeResultHash 计算结果的内容哈希（确定性）。
func (e *EvidenceBundle) ComputeResultHash() string {
	sortedSegments := make([]SegmentResult, len(e.Segments))
	copy(sortedSegments, e.Segments)
	sort.Slice(sortedSegments, func(i, j int) bool {
		return sortedSegments[i].Segment < sortedSegments[j].Segment
	})
	payload := struct {
		JobHash        string             `json:"jh"`
		MethodHash     string             `json:"mh"`
		Snapshot       string             `json:"sid"`
		Segments       []SegmentResult    `json:"segs"`
		Oos            PerformanceStats   `json:"oos"`
		Trials         int                `json:"trials"`
		AdjustedPValue float64            `json:"adjusted_p_value"`
		CriticIssues   []CriticIssue      `json:"critic_issues"`
		Blockers       []PromotionBlocker `json:"blockers"`
		Confidence     string             `json:"conf"`
		Passable       bool               `json:"passable"`
	}{
		JobHash: e.JobHash, MethodHash: e.MethodHash, Snapshot: e.SnapshotID,
		Segments: sortedSegments, Oos: e.OosStats, Trials: e.DiscoveryTrials,
		AdjustedPValue: e.AdjustedPValue, CriticIssues: e.CriticIssues,
		Blockers: e.Blockers, Confidence: string(e.Confidence), Passable: e.Passable,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
