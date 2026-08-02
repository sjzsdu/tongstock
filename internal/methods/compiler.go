package methods

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Candidate 是编译器的输入, 可以来自自然语言结构化结果或已有的
// paradigm JSON。编译器只接受明确字段, 保留歧义而非静默猜测。
type Candidate struct {
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	SourceKind      string   `json:"source_kind,omitempty"` // "natural_lang" / "structured" / "existing_paradigm"
	SourceText      string   `json:"source_text,omitempty"`
	Universe        string   `json:"universe,omitempty"`
	BoardFilter     []string `json:"board_filter,omitempty"`
	MarketState     []string `json:"market_state,omitempty"`
	FeatureDeps     []string `json:"feature_deps,omitempty"`
	MaxPositions    int      `json:"max_positions,omitempty"`
	Entry           any      `json:"entry_rule"` // JSON object → *Expr
	Exit            any      `json:"exit_rule,omitempty"`
	Invalid         any      `json:"invalid_rule,omitempty"`
	PositionMode    string   `json:"position_mode,omitempty"` // "pct_equity" / "fixed_lots" / "kelly_half"
	PositionPct     *float64 `json:"position_pct,omitempty"`  // 0..1
	PositionLots    *int     `json:"position_lots,omitempty"`
	HoldingMaxDays  int      `json:"holding_max_days,omitempty"`
	HoldingMinDays  int      `json:"holding_min_days,omitempty"`
	StopLossPct     *float64 `json:"stop_loss_pct,omitempty"`
	TakeProfitPct   *float64 `json:"take_profit_pct,omitempty"`
	TrailingStopPct *float64 `json:"trailing_stop_pct,omitempty"`
}

// Compile 将候选编译为稳定的 CompiledMethod。返回的方法即使失败也会携带
// 诊断和歧义列表, 用于 AI / UI 解释。
//
// 关键不变式:
//  1. 未知字段 fail-closed (json.Decoder UseNumber + DisallowUnknownFields)。
//  2. 歧义规则生成 NodeAmbiguous 并加入 ambiguities, 不静默猜测。
//  3. 未来函数 lookahead 立即拒绝 (error 级诊断)。
//  4. content_hash 仅由稳定字段决定, 不包含时间戳/源文本。
func Compile(c *Candidate) (*CompiledMethod, []Diagnostic, error) {
	if c == nil {
		return nil, nil, errors.New("nil candidate")
	}
	m := &CompiledMethod{
		Name:            strings.TrimSpace(c.Name),
		Description:     strings.TrimSpace(c.Description),
		CompilerVersion: CompilerVersion,
		SourceKind:      firstNonEmpty(c.SourceKind, "structured"),
		SourceText:      c.SourceText,
		Scope: Scope{
			Universe:     firstNonEmpty(c.Universe, "universe_all"),
			BoardFilter:  append([]string{}, c.BoardFilter...),
			MarketState:  append([]string{}, c.MarketState...),
			FeatureDeps:  append([]string{}, c.FeatureDeps...),
			MaxPositions: c.MaxPositions,
		},
		Position: normalizePos(c),
		Holding: HoldingRule{
			MaxDays:      c.HoldingMaxDays,
			MinDays:      c.HoldingMinDays,
			StopLoss:     c.StopLossPct,
			TakeProfit:   c.TakeProfitPct,
			TrailingStop: c.TrailingStopPct,
		},
		CompiledAt: time.Now(),
	}
	if m.Name == "" {
		return m, append(m.Diagnostics, Diagnostic{
			Level: "error", Code: "NAME_EMPTY", Detail: "方法名称不能为空",
		}), nil
	}

	var diags []Diagnostic
	entryDiags, entryAST, err := buildExpr(c.Entry, "entry_rule")
	diags = append(diags, entryDiags...)
	if err != nil {
		diags = append(diags, Diagnostic{Level: "error", Code: "ENTRY_INVALID", Detail: err.Error(), Path: "entry_rule"})
		m.EntryRule = ambiguousFrom("entry_rule", []string{err.Error()})
	} else {
		m.EntryRule = entryAST
	}

	if c.Exit != nil {
		exitDiags, exitAST, exitErr := buildExpr(c.Exit, "exit_rule")
		diags = append(diags, exitDiags...)
		if exitErr != nil {
			diags = append(diags, Diagnostic{Level: "error", Code: "EXIT_INVALID", Detail: exitErr.Error(), Path: "exit_rule"})
			m.ExitRule = ambiguousFrom("exit_rule", []string{exitErr.Error()})
		} else {
			m.ExitRule = exitAST
		}
	} else {
		// 缺失退出是隐式风险: 输出 warn, 方法仍可执行 (由 持仓周期 或 信号驱动离场)。
		diags = append(diags, Diagnostic{
			Level: "warn", Code: "EXIT_MISSING", Detail: "未显式提供 exit_rule，方法将依赖持仓周期或外部信号离场",
		})
	}

	if c.Invalid != nil {
		invDiags, invAST, invErr := buildExpr(c.Invalid, "invalid_rule")
		diags = append(diags, invDiags...)
		if invErr != nil {
			diags = append(diags, Diagnostic{Level: "error", Code: "INVALID_RULE", Detail: invErr.Error(), Path: "invalid_rule"})
			m.InvalidRule = ambiguousFrom("invalid_rule", []string{invErr.Error()})
		} else {
			m.InvalidRule = invAST
		}
	}

	// 未来函数检测: 任何地方提及 lookahead 或 未来时间锚
	lookDiags := detectFutureFunctions(m)
	diags = append(diags, lookDiags...)

	// 特征依赖合并: AST 里引用的 Indicator 若非内建, 自动加入 FeatureDeps
	m.Scope.FeatureDeps = dedupeStrings(append(m.Scope.FeatureDeps, collectIndicators(m.EntryRule, m.ExitRule, m.InvalidRule)...))

	// 歧义聚合: 所有 NodeAmbiguous 的来源 + reason
	m.Ambiguities = collectAmbiguities(m.EntryRule, m.ExitRule, m.InvalidRule)

	// 持仓/仓位合理性校验
	hd := validateHolding(&m.Holding)
	diags = append(diags, hd...)
	pd := validatePosition(&m.Position)
	diags = append(diags, pd...)

	m.Diagnostics = diags

	hash, err := ComputeContentHash(m)
	if err != nil {
		diags = append(diags, Diagnostic{Level: "error", Code: "HASH", Detail: err.Error()})
		return m, diags, err
	}
	m.ContentHash = hash
	return m, diags, nil
}

// buildExpr 将 any (已解码 JSON 或 map/string) 规范化为 *Expr。
func buildExpr(src any, path string) ([]Diagnostic, *Expr, error) {
	if src == nil {
		return nil, nil, fmt.Errorf("%s: missing expression", path)
	}
	// Candidate 直接传递 map/[]any 对象, 我们重新序列化 + DisallowUnknownFields 解码。
	buf, err := json.Marshal(src)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: marshal source: %w", path, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(buf)))
	dec.DisallowUnknownFields()
	var e Expr
	if err := dec.Decode(&e); err != nil {
		// 未知字段错误优先作为 ambiguous 保留, 不直接 crash
		return []Diagnostic{{
			Level: "ambiguous", Code: "UNKNOWN_FIELDS", Path: path, Detail: err.Error(),
		}}, ambiguousFrom(path, []string{err.Error()}), nil
	}
	diags := validateExpr(&e, path)
	if len(diags) > 0 && hasOnlyAmbiguous(diags) {
		return diags, ambiguousFrom(path, diagReasons(diags)), nil
	}
	// 规范化
	normalizeExpr(&e)
	return diags, &e, nil
}

// validateExpr 对 Expr 树做类型/值域检查。
func validateExpr(e *Expr, path string) []Diagnostic {
	if e == nil {
		return []Diagnostic{{Level: "error", Code: "NIL_EXPR", Path: path, Detail: "nil expression"}}
	}
	var out []Diagnostic
	switch e.Type {
	case NodeIndicator:
		if e.Indicator == "" {
			out = append(out, Diagnostic{Level: "error", Code: "INDICATOR_EMPTY", Path: path, Detail: "indicator missing name"})
		}
	case NodeConstant:
		if e.Value == nil {
			out = append(out, Diagnostic{Level: "error", Code: "CONSTANT_EMPTY", Path: path, Detail: "constant missing value"})
		}
	case NodeCompare:
		if e.Left == nil || e.Right == nil {
			out = append(out, Diagnostic{Level: "error", Code: "COMPARE_MISSING_OP", Path: path, Detail: "compare requires left and right"})
		}
		if !knownCmp(e.Op) {
			out = append(out, Diagnostic{Level: "ambiguous", Code: "OP_UNKNOWN", Path: path, Detail: fmt.Sprintf("unknown op %q", e.Op)})
		}
		out = append(out, validateExpr(e.Left, path+".left")...)
		out = append(out, validateExpr(e.Right, path+".right")...)
	case NodeAnd, NodeOr:
		if len(e.Children) < 2 {
			out = append(out, Diagnostic{Level: "ambiguous", Code: "COMPOSITION_CHILDREN", Path: path, Detail: "and/or needs 2+ children"})
		}
		for i, ch := range e.Children {
			out = append(out, validateExpr(ch, fmt.Sprintf("%s[%d]", path, i))...)
		}
	case NodeNot:
		if len(e.Children) != 1 {
			out = append(out, Diagnostic{Level: "ambiguous", Code: "NOT_CHILDREN", Path: path, Detail: "not needs exactly one child"})
		}
		for i, ch := range e.Children {
			out = append(out, validateExpr(ch, fmt.Sprintf("%s[%d]", path, i))...)
		}
	case NodeInWindow:
		if e.WindowDays <= 0 {
			out = append(out, Diagnostic{Level: "ambiguous", Code: "WINDOW_ZERO", Path: path, Detail: "window_days must be positive"})
		}
		if e.WindowMode != "any" && e.WindowMode != "all" {
			out = append(out, Diagnostic{Level: "ambiguous", Code: "WINDOW_MODE", Path: path, Detail: fmt.Sprintf("unknown window_mode %q", e.WindowMode)})
		}
		for i, ch := range e.Children {
			out = append(out, validateExpr(ch, fmt.Sprintf("%s[%d]", path, i))...)
		}
	case NodeRank:
		if e.RankPct <= 0 || e.RankPct > 1 {
			out = append(out, Diagnostic{Level: "error", Code: "RANK_PCT", Path: path, Detail: "rank_pct must be in (0,1]"})
		}
		if e.RankSide != "top" && e.RankSide != "bottom" {
			out = append(out, Diagnostic{Level: "ambiguous", Code: "RANK_SIDE", Path: path})
		}
	case NodeCross:
		if e.Cross != "above" && e.Cross != "below" {
			out = append(out, Diagnostic{Level: "ambiguous", Code: "CROSS_SIDE", Path: path})
		}
		out = append(out, validateExpr(e.Left, path+".left")...)
		out = append(out, validateExpr(e.Right, path+".right")...)
	case NodeAmbiguous:
		// 已经是歧义节点, 不继续 deep 报错
	default:
		out = append(out, Diagnostic{Level: "error", Code: "EXPR_TYPE", Path: path, Detail: fmt.Sprintf("unknown node type %q", e.Type)})
	}
	return out
}

func knownCmp(o CmpOp) bool {
	switch o {
	case CmpGT, CmpLT, CmpGTE, CmpLTE, CmpEQ:
		return true
	}
	return false
}

func hasOnlyAmbiguous(ds []Diagnostic) bool {
	if len(ds) == 0 {
		return false
	}
	for _, d := range ds {
		if d.Level != "ambiguous" {
			return false
		}
	}
	return true
}

func diagReasons(ds []Diagnostic) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, fmt.Sprintf("%s: %s", d.Code, d.Detail))
	}
	return out
}

// normalizeExpr 规范化 AST 的枚举值大小写与常量。
func normalizeExpr(e *Expr) {
	if e == nil {
		return
	}
	e.Indicator = strings.ToLower(strings.TrimSpace(e.Indicator))
	e.Op = CmpOp(strings.ToLower(string(e.Op)))
	e.Cross = strings.ToLower(strings.TrimSpace(e.Cross))
	e.WindowMode = strings.ToLower(strings.TrimSpace(e.WindowMode))
	e.RankSide = strings.ToLower(strings.TrimSpace(e.RankSide))
	e.RankBy = strings.ToLower(strings.TrimSpace(e.RankBy))
	for _, ch := range e.Children {
		normalizeExpr(ch)
	}
	normalizeExpr(e.Left)
	normalizeExpr(e.Right)
}

// detectFutureFunctions 拒绝引用未来值的任何模式。
func detectFutureFunctions(m *CompiledMethod) []Diagnostic {
	var out []Diagnostic
	var walk func(e *Expr, path string)
	walk = func(e *Expr, path string) {
		if e == nil {
			return
		}
		if e.Type == NodeIndicator {
			ind := strings.ToLower(e.Indicator)
			if strings.HasPrefix(ind, "future") || strings.Contains(ind, "lookahead") || strings.Contains(ind, "next_") {
				out = append(out, Diagnostic{
					Level: "error", Code: "FUTURE_FUNCTION", Path: path,
					Detail: fmt.Sprintf("indicator %q references future data — fail closed", e.Indicator),
				})
			}
		}
		for i, ch := range e.Children {
			walk(ch, fmt.Sprintf("%s[%d]", path, i))
		}
		walk(e.Left, path+".left")
		walk(e.Right, path+".right")
	}
	walk(m.EntryRule, "entry")
	walk(m.ExitRule, "exit")
	walk(m.InvalidRule, "invalid")
	return out
}

// collectIndicators 收集 AST 中所有非内建指标名, 用来计算特征依赖。
func collectIndicators(exprs ...*Expr) []string {
	seen := map[string]bool{}
	var walk func(*Expr)
	walk = func(e *Expr) {
		if e == nil {
			return
		}
		if e.Type == NodeIndicator && e.Indicator != "" {
			if !isBuiltinIndicator(e.Indicator) {
				seen[e.Indicator] = true
			}
		}
		for _, ch := range e.Children {
			walk(ch)
		}
		walk(e.Left)
		walk(e.Right)
	}
	for _, e := range exprs {
		walk(e)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var builtinIndicatorSet = map[string]bool{
	"close": true, "open": true, "high": true, "low": true,
	"volume": true, "amount": true, "return1": true, "gap_pct": true,
	"ma5": true, "ma10": true, "ma20": true, "ma60": true, "ma120": true, "ma250": true,
	"macd_dif": true, "macd_dea": true, "macd_hist": true,
	"kdj_k": true, "kdj_d": true, "kdj_j": true,
	"boll_upper": true, "boll_mid": true, "boll_lower": true,
	"rsi6": true, "rsi14": true, "rsi24": true,
}

func isBuiltinIndicator(name string) bool {
	// 支持参数化内建指标。
	if n := name; len(n) > 2 {
		switch {
		case strings.HasPrefix(n, "ma"):
			if _, err := strconv.Atoi(n[2:]); err == nil {
				return true
			}
		case strings.HasPrefix(n, "rsi"):
			if _, err := strconv.Atoi(n[3:]); err == nil {
				return true
			}
		case strings.HasPrefix(n, "prevhigh"):
			if _, err := strconv.Atoi(n[len("prevhigh"):]); err == nil {
				return true
			}
		case strings.HasPrefix(n, "prevlow"):
			if _, err := strconv.Atoi(n[len("prevlow"):]); err == nil {
				return true
			}
		case strings.HasPrefix(n, "volma"):
			if _, err := strconv.Atoi(n[len("volma"):]); err == nil {
				return true
			}
		case strings.HasPrefix(n, "volatility"):
			if _, err := strconv.Atoi(n[len("volatility"):]); err == nil {
				return true
			}
		}
	}
	return builtinIndicatorSet[strings.ToLower(name)]
}

// collectAmbiguities 聚合树中所有 Ambiguous 节点的原因。
func collectAmbiguities(exprs ...*Expr) []string {
	var out []string
	var walk func(*Expr)
	walk = func(e *Expr) {
		if e == nil {
			return
		}
		if e.Type == NodeAmbiguous {
			for _, r := range e.AmbiguousReasons {
				out = append(out, fmt.Sprintf("%s: %s", e.AmbiguousSource, r))
			}
		}
		for _, ch := range e.Children {
			walk(ch)
		}
		walk(e.Left)
		walk(e.Right)
	}
	for _, e := range exprs {
		walk(e)
	}
	return dedupeStrings(out)
}

func ambiguousFrom(source string, reasons []string) *Expr {
	return &Expr{Type: NodeAmbiguous, AmbiguousSource: source, AmbiguousReasons: reasons}
}

func validateHolding(h *HoldingRule) []Diagnostic {
	var out []Diagnostic
	if h.MaxDays < 0 || h.MinDays < 0 {
		out = append(out, Diagnostic{Level: "error", Code: "DAYS_NEGATIVE", Detail: "holding days cannot be negative"})
	}
	if h.MinDays > h.MaxDays && h.MaxDays > 0 {
		out = append(out, Diagnostic{Level: "error", Code: "HOLDING_RANGE", Detail: "min_days > max_days"})
	}
	for _, pct := range []*float64{h.StopLoss, h.TakeProfit, h.TrailingStop} {
		if pct != nil {
			if math.IsNaN(*pct) || math.IsInf(*pct, 0) {
				out = append(out, Diagnostic{Level: "error", Code: "PCT_NAN", Detail: "holding pct is NaN/Inf"})
			}
		}
	}
	if h.StopLoss != nil && *h.StopLoss > 0 {
		out = append(out, Diagnostic{Level: "warn", Code: "STOPLOSS_SIGN", Detail: "stop_loss_pct should usually be negative (e.g. -0.05 = -5%)"})
	}
	return out
}

func validatePosition(p *PosRule) []Diagnostic {
	var out []Diagnostic
	switch p.Mode {
	case "pct_equity":
		if p.PctEquity == nil {
			out = append(out, Diagnostic{Level: "error", Code: "PCT_MISSING", Detail: "pct_equity mode requires pct_equity value"})
		} else if *p.PctEquity <= 0 || *p.PctEquity > 1 {
			out = append(out, Diagnostic{Level: "error", Code: "PCT_RANGE", Detail: "pct_equity must be in (0,1]"})
		}
	case "fixed_lots":
		if p.FixedLots == nil {
			out = append(out, Diagnostic{Level: "error", Code: "LOTS_MISSING", Detail: "fixed_lots requires lots value"})
		} else if *p.FixedLots <= 0 {
			out = append(out, Diagnostic{Level: "error", Code: "LOTS_RANGE", Detail: "fixed_lots must be > 0"})
		}
	case "kelly_half":
		// 动态, 无需静态字段
	default:
		out = append(out, Diagnostic{Level: "ambiguous", Code: "POS_MODE", Detail: fmt.Sprintf("unknown position mode %q", p.Mode)})
	}
	return out
}

func normalizePos(c *Candidate) PosRule {
	p := PosRule{Mode: firstNonEmpty(c.PositionMode, "pct_equity")}
	switch p.Mode {
	case "pct_equity":
		p.PctEquity = c.PositionPct
		if p.PctEquity == nil {
			v := 0.1 // 保守默认 10%
			p.PctEquity = &v
		}
	case "fixed_lots":
		p.FixedLots = c.PositionLots
	}
	return p
}

func dedupeStrings(s []string) []string {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
