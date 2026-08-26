// Package methods 定义统一的投资方法模型 (InvestmentMethod) 与确定性 DSL/AST。
//
// 一个投资方法由三部分组成:
//  1. 应用范围 (适用股票池、市场状态、特征依赖)
//  2. 规则 AST (入场、退出、失效、仓位、持仓周期)
//  3. 证据状态 (由验证工厂负责, 本包不计算有效性)
//
// 编译器 (compiler.go) 将结构化候选或 AI 输出转为稳定 AST:
//   - 类型检查、单位规范化、字段白名单
//   - 显式拒绝未来函数或歧义规则 (不静默猜测)
//   - 生成稳定内容哈希与编译器版本, 保证复现
//
// 执行器 (executor.go) 使用真实 K 线在 AST 上入场/出场判定, 并可输出
// 逐条件解释 (explain.go) 供 AI 或 UI 翻译。
//
// 设计上复用 paradigms 条件执行经验, 但不限制于 paradigms 的狭窄字段。
// 所有未知输入 fail-closed。
package methods

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// CompilerVersion 必须随任何改变 AST 哈希/求值语义的代码变化而递增。
// 只有编译器完全等价才允许复用同一版本号。
const CompilerVersion = "0.2.0"

// Side 方向: 入场 (BUY) 或 出场 (SELL)。
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// Scope 描述方法的应用范围与非价格依赖。
type Scope struct {
	Universe     string   `json:"universe"`                // 股票池标签: "universe_all", "universe_csi800" 等
	BoardFilter  []string `json:"board_filter,omitempty"`  // 主板/创业板/科创板, 空=无限制
	MarketState  []string `json:"market_state,omitempty"`  // "trend_up" "bear" "range" 空=无限制
	FeatureDeps  []string `json:"feature_deps,omitempty"`  // 依赖的非内建特征名 (用于执行前 fail-fast)
	MaxPositions int      `json:"max_positions,omitempty"` // 最大持仓数, 0=不限
}

// NodeType 是 AST 的节点类型枚举, 保证 switch 穷举时不落入静默默认。
type NodeType string

const (
	NodeIndicator NodeType = "indicator" // 单指标: 指标名 + 归一化参数
	NodeConstant  NodeType = "constant"  // 数值常量
	NodeCompare   NodeType = "compare"   // 左 OP 右, OP in {gt,lt,eq,gte,lte}
	NodeAnd       NodeType = "and"
	NodeOr        NodeType = "or"
	NodeNot       NodeType = "not"
	NodeInWindow  NodeType = "in_window" // 时间窗口: {within 最近 N 天, 子条件}
	NodeRank      NodeType = "rank"      // 横截面: 顶部 N% / 底部 N%
	NodeCross     NodeType = "cross"     // 上穿 / 下穿
	NodeAmbiguous NodeType = "ambiguous" // 歧义占位: 编译后保留但拒绝执行
)

// CmpOp 比较运算, 限定在白名单内。
type CmpOp string

const (
	CmpGT  CmpOp = "gt"
	CmpLT  CmpOp = "lt"
	CmpGTE CmpOp = "gte"
	CmpLTE CmpOp = "lte"
	CmpEQ  CmpOp = "eq"
)

// Expr 是 AST 表达式节点。所有未知字段在反序列化时被拒绝 (DisallowUnknownFields)。
type Expr struct {
	Type NodeType `json:"type"`

	// Indicator / Constant
	Indicator string   `json:"indicator,omitempty"`
	Params    []string `json:"params,omitempty"` // e.g. MA params: ["20"]
	Value     *float64 `json:"value,omitempty"`  // Constant literal

	// Compare / Cross
	Left  *Expr  `json:"left,omitempty"`
	Right *Expr  `json:"right,omitempty"`
	Op    CmpOp  `json:"op,omitempty"`
	Cross string `json:"cross,omitempty"` // "above" | "below"

	// Composition
	Children []*Expr `json:"children,omitempty"`

	// InWindow: 子条件在近 N 天内满足。
	WindowDays int    `json:"window_days,omitempty"`
	WindowMode string `json:"window_mode,omitempty"` // "any" | "all"

	// Rank: 横截面排名 (执行时需要提供当日全市场特征快照)
	RankPct  float64 `json:"rank_pct,omitempty"`  // 0..1
	RankSide string  `json:"rank_side,omitempty"` // "top" | "bottom"
	RankBy   string  `json:"rank_by,omitempty"`   // 指标名, 例 "pe" "turnover_rate"

	// Ambiguous: 编译保留的歧义描述, 仅用于诊断输出。
	AmbiguousSource  string   `json:"ambiguous_source,omitempty"`
	AmbiguousReasons []string `json:"ambiguous_reasons,omitempty"`
}

// PosRule 是仓位规则。静态大小 (PctEquity) 或 动态 (Kelly / 固定手数)。
type PosRule struct {
	Mode      string   `json:"mode"`                 // "pct_equity" | "fixed_lots" | "kelly_half"
	PctEquity *float64 `json:"pct_equity,omitempty"` // 0..1 (Mode = pct_equity 时必填)
	FixedLots *int     `json:"fixed_lots,omitempty"`
}

// HoldingRule 是持仓周期规则。明确的硬终止条件优先于到期终止。
type HoldingRule struct {
	MaxDays      int      `json:"max_days,omitempty"`          // 最大持仓日历日, 0=不限制
	MinDays      int      `json:"min_days,omitempty"`          // 最小持仓日 (防止 T+1 外频繁)
	StopLoss     *float64 `json:"stop_loss_pct,omitempty"`     // 亏损百分比止损, e.g. -0.05 = -5%
	TakeProfit   *float64 `json:"take_profit_pct,omitempty"`   // 盈利百分比止盈
	TrailingStop *float64 `json:"trailing_stop_pct,omitempty"` // 回撤百分比移动止盈
}

// Diagnostic 是单条编译诊断。Level ∈ {info,warn,error,ambiguous}。
type Diagnostic struct {
	Level  string `json:"level"`
	Code   string `json:"code"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail"`
}

// CompiledMethod 是编译后稳定的方法: 含 AST、范围、规则、诊断。
// 禁止直接构造；请使用 Compile()。
type CompiledMethod struct {
	Name            string       `json:"name"`
	Description     string       `json:"description,omitempty"`
	CompilerVersion string       `json:"compiler_version"`
	ContentHash     string       `json:"content_hash"`
	SourceKind      string       `json:"source_kind"` // "natural_lang" | "structured" | "existing_paradigm"
	SourceText      string       `json:"source_text,omitempty"`
	Scope           Scope        `json:"scope"`
	EntryRule       *Expr        `json:"entry_rule"`
	ExitRule        *Expr        `json:"exit_rule,omitempty"`
	InvalidRule     *Expr        `json:"invalid_rule,omitempty"`
	Position        PosRule      `json:"position"`
	Holding         HoldingRule  `json:"holding"`
	Diagnostics     []Diagnostic `json:"diagnostics,omitempty"`
	Ambiguities     []string     `json:"ambiguities,omitempty"`
	CompiledAt      time.Time    `json:"compiled_at"`
}

// IsExecutable 当且仅当 CompiledMethod 不含 error 级诊断、入场规则非
// nil、没有歧义占位 Ambiguous 节点时, 方法才可执行。
func (c *CompiledMethod) IsExecutable() bool {
	if c == nil || c.EntryRule == nil {
		return false
	}
	for _, d := range c.Diagnostics {
		if d.Level == "error" {
			return false
		}
	}
	return !hasAmbiguous(c.EntryRule) &&
		(c.ExitRule == nil || !hasAmbiguous(c.ExitRule)) &&
		(c.InvalidRule == nil || !hasAmbiguous(c.InvalidRule))
}

func hasAmbiguous(e *Expr) bool {
	if e == nil {
		return false
	}
	if e.Type == NodeAmbiguous {
		return true
	}
	for _, ch := range e.Children {
		if hasAmbiguous(ch) {
			return true
		}
	}
	return hasAmbiguous(e.Left) || hasAmbiguous(e.Right)
}

// ComputeContentHash 从方法的确定性字段生成稳定哈希 (不包含时间戳)。
// 同一规范输入必然生成相同哈希, 用来去重和版本对齐。
func ComputeContentHash(m *CompiledMethod) (string, error) {
	if m == nil {
		return "", nil
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	// 不包含 CompiledAt / SourceText 等不稳定字段:
	payload := map[string]any{
		"name":            m.Name,
		"description":     m.Description,
		"compilerVersion": CompilerVersion,
		"scope":           m.Scope,
		"entryRule":       m.EntryRule,
		"exitRule":        m.ExitRule,
		"invalidRule":     m.InvalidRule,
		"position":        m.Position,
		"holding":         m.Holding,
	}
	if err := enc.Encode(payload); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)[:16]), nil
}
