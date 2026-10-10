package paradigms

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/sjzsdu/tongstock/internal/methods"
)

// 范式 → 方法候选的编译映射。
//
// 红线（fail-closed）：无法在「单日冻结特征」上确定性求值的条件
// （cross_above / cross_below / near / describe）一律返回 blocker 拒绝晋级，
// 绝不静默降级成近似规则——否则会出现「验证通过但选股永不触发」的假象
// （internal/selection/engine.go 对 cross/in_window 产出
// historical_features_unavailable exclusion，晋级进方法的范式不能含此类条件）。
// 编译必需字段缺失（买入条件为空、条件值既非数字也非已知指标等）同样拒绝。

// promoteUniverse 与 methodseed / methodautomation 的验证股票池名保持一致。
const promoteUniverse = "universe_usable"

// knownIndicatorRe 是可直接映射为表达式指标节点的内建指标名。
// 必须与 internal/methods 的内建指标集一致（maN/rsiN/prevhighN/prevlowN/volmaN/
// volatilityN 为参数化内建，其余为 builtinIndicatorSet 成员）。
var knownIndicatorRe = regexp.MustCompile(`^(close|open|high|low|volume|amount|return1|gap_pct|` +
	`ma[0-9]+|rsi[0-9]+|macd_(dif|dea|hist)|kdj_(k|d|j)|boll_(upper|mid|lower)|` +
	`prevhigh[0-9]+|prevlow[0-9]+|volma[0-9]+|volatility[0-9]+)$`)

// betweenSeparatorRe 解析 between 值的区间分隔符（如 "10-20"、"10~20"、"10至20"）。
var betweenSeparatorRe = regexp.MustCompile(`\s*(?:-|~|至|到)\s*`)

// defaultHoldingMaxDays / defaultHoldingMinDays 是 holding_period 缺失或不可解析时
// 的保守默认值；该决定会写进方法描述，绝不静默。
const (
	defaultHoldingMaxDays = 20
	defaultHoldingMinDays = 3
)

// ToCandidate 把范式编译为方法候选。blockers 非空表示该范式当前不可晋级，
// 调用方必须如实把原因记录进 PromotionBlockers，且不得注册方法。
//
// 映射规则：
//   - Side=buy 才可晋级；Side=sell 返回 blocker。
//   - BuyConds（and 组合）→ Entry 规则树；gt/lt → compare 节点；
//     between → and(gt, lt)。
//   - SellConds.TakeProfit / StopLoss 中的百分比条件 → TakeProfitPct/StopLossPct；
//     其余（价格位、指标条件）→ Exit 规则。
//   - Context.MarketCap → Universe=universe_usable + BoardFilter 市值分桶标记；
//   - Expectation.HoldingPeriod → HoldingMaxDays/HoldingMinDays（不可解析时用
//     保守默认并在描述中显式注明）。
//   - SourceKind 固定为 "existing_paradigm"。
func ToCandidate(p *Paradigm) (*methods.Candidate, []string, error) {
	if p == nil {
		return nil, nil, fmt.Errorf("nil paradigm")
	}
	var blockers []string
	addBlocker := func(format string, args ...any) {
		blockers = append(blockers, fmt.Sprintf(format, args...))
	}

	if p.Side != "buy" {
		addBlocker("side=%q 不是 buy：当前方法库只承接做多范式", p.Side)
	}
	if len(p.BuyConds) == 0 {
		addBlocker("buy_conditions 为空：没有可编译的入场规则")
	}

	// 买入条件 → Entry 规则树（and 组合）。
	var entryChildren []any
	deps := map[string]bool{}
	for i, cond := range p.BuyConds {
		node, dep, err := conditionToExpr(cond)
		if err != nil {
			addBlocker("buy_conditions[%d] (%s %s %s): %v", i, cond.Indicator, cond.Operator, cond.Value, err)
			continue
		}
		entryChildren = append(entryChildren, node)
		if dep != "" {
			deps[dep] = true
		}
	}
	if len(entryChildren) == 0 && len(p.BuyConds) > 0 {
		addBlocker("buy_conditions 全部无法编译：没有可执行入场规则")
	}
	var entry any
	switch len(entryChildren) {
	case 0:
		// 已记录 blocker；entry 留空。
	case 1:
		entry = entryChildren[0]
	default:
		entry = map[string]any{"type": "and", "children": entryChildren}
	}

	// 卖出条件 → 百分比止盈止损 + Exit 规则。
	takeProfitPct, stopLossPct, exitChildren, exitDeps := sellCondsToRules(p.SellConds, addBlocker)
	for dep := range exitDeps {
		deps[dep] = true
	}
	var exit any
	switch len(exitChildren) {
	case 0:
	case 1:
		exit = exitChildren[0]
	default:
		exit = map[string]any{"type": "and", "children": exitChildren}
	}

	if len(blockers) > 0 {
		return nil, blockers, nil
	}

	// 持仓周期 → HoldingMaxDays/MinDays。不可解析不是 blocker（编译器允许
	// 无显式持仓规则），但默认值必须写进描述，保证可审计。
	minDays, maxDays := defaultHoldingMinDays, defaultHoldingMaxDays
	holdingNote := ""
	if parsed, ok := parseHoldingPeriod(p.Expectation.HoldingPeriod); ok {
		minDays, maxDays = parsed.min, parsed.max
	} else if strings.TrimSpace(p.Expectation.HoldingPeriod) != "" {
		holdingNote = fmt.Sprintf("原 holding_period=%q 不可解析，", p.Expectation.HoldingPeriod)
	}

	boardFilter := marketCapBoardFilter(p.Context.MarketCap)
	if len(boardFilter) == 0 && strings.TrimSpace(p.Context.MarketCap) != "" {
		boardFilter = []string{strings.TrimSpace(p.Context.MarketCap)}
	}

	description := buildPromotedDescription(p, holdingNote)
	featureDeps := sortedKeys(deps)

	return &methods.Candidate{
		Name:           p.Name,
		Description:    description,
		SourceKind:     "existing_paradigm",
		SourceText:     p.Rationale,
		Universe:       promoteUniverse,
		BoardFilter:    boardFilter,
		FeatureDeps:    featureDeps,
		MaxPositions:   5,
		PositionMode:   "pct_equity",
		PositionPct:    pctPtr(0.10),
		HoldingMaxDays: maxDays,
		HoldingMinDays: minDays,
		StopLossPct:    stopLossPct,
		TakeProfitPct:  takeProfitPct,
		Entry:          entry,
		Exit:           exit,
	}, nil, nil
}

// conditionToExpr 把单条范式条件编译为表达式节点，返回节点与指标依赖。
// fail-closed：cross/near/describe 或不可解析值返回错误。
func conditionToExpr(cond Condition) (any, string, error) {
	indicator := NormalizeIndicator(cond.Indicator)
	if indicator == "" {
		return nil, "", fmt.Errorf("indicator 为空")
	}
	if !knownIndicatorRe.MatchString(indicator) {
		return nil, "", fmt.Errorf("indicator %q 不是可编译内建指标", indicator)
	}

	switch cond.Operator {
	case "gt", "lt":
		return comparisonToExpr(cond.Operator, indicator, cond.Value)
	case "between":
		lo, hi, err := parseBetweenValue(cond.Value)
		if err != nil {
			return nil, "", err
		}
		gt, err := compareExpr("gt", indicator, constantNode(lo))
		if err != nil {
			return nil, "", err
		}
		lt, err := compareExpr("lt", indicator, constantNode(hi))
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"type": "and", "children": []any{gt, lt}}, indicator, nil
	case "cross_above", "cross_below":
		return nil, "", fmt.Errorf("operator %q 需要历史序列，选股只持单日冻结特征，fail-closed 拒绝", cond.Operator)
	case "near":
		return nil, "", fmt.Errorf("operator %q 无确定性阈值语义，fail-closed 拒绝", cond.Operator)
	case "describe":
		return nil, "", fmt.Errorf("自然语言描述条件无法编译，需要结构化为 gt/lt/before 晋级")
	default:
		return nil, "", fmt.Errorf("未知 operator %q", cond.Operator)
	}
}

// comparisonToExpr 编译 gt/lt：右值是数字 → 常量节点；是已知指标 → 指标节点；
// 含 % 或两者都不是 → fail-closed。
func comparisonToExpr(op, indicator, rawValue string) (any, string, error) {
	value := strings.Trim(strings.TrimSpace(rawValue), "\"'`")
	if value == "" {
		return nil, "", fmt.Errorf("value 为空")
	}
	if strings.Contains(value, "%") {
		return nil, "", fmt.Errorf("value %q 带百分号、比较基准歧义，fail-closed 拒绝", rawValue)
	}
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		node, err := compareExpr(op, indicator, constantNode(v))
		return node, indicator, err
	}
	normalized := NormalizeIndicator(value)
	if knownIndicatorRe.MatchString(normalized) {
		node, err := compareExpr(op, indicator, map[string]any{"type": "indicator", "indicator": normalized})
		return node, indicator, err
	}
	return nil, "", fmt.Errorf("value %q 既非数字也非已知指标，fail-closed 拒绝", rawValue)
}

func compareExpr(op, indicator string, right any) (any, error) {
	if op != "gt" && op != "lt" {
		return nil, fmt.Errorf("unsupported compare op %q", op)
	}
	return map[string]any{
		"type": "compare", "op": op,
		"left":  map[string]any{"type": "indicator", "indicator": indicator},
		"right": right,
	}, nil
}

func constantNode(v float64) map[string]any {
	return map[string]any{"type": "constant", "value": v}
}

func parseBetweenValue(raw string) (float64, float64, error) {
	value := strings.Trim(strings.TrimSpace(raw), "\"'`")
	value = strings.TrimSuffix(value, "%")
	parts := betweenSeparatorRe.Split(value, -1)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("between 值 %q 不是两数字区间（如 10-20），fail-closed 拒绝", raw)
	}
	lo, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("between 下界 %q 不是数字", parts[0])
	}
	hi, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("between 上界 %q 不是数字", parts[1])
	}
	if lo >= hi {
		return 0, 0, fmt.Errorf("between 区间 %q 下界不小于上界", raw)
	}
	return lo, hi, nil
}

// sellCondsToRules 把卖出条件拆成百分比止盈/止损与 Exit 规则。
func sellCondsToRules(sells SellConditions, addBlocker func(string, ...any)) (*float64, *float64, []any, map[string]bool) {
	deps := map[string]bool{}
	var exitChildren []any
	var takeProfitPct, stopLossPct *float64

	for i, cond := range sells.TakeProfit {
		pct, node, dep, err := sellCondition(cond, true)
		if err != nil {
			addBlocker("sell_conditions.take_profit[%d] (%s %s %s): %v", i, cond.Indicator, cond.Operator, cond.Value, err)
			continue
		}
		if pct != nil {
			takeProfitPct = pct
			continue
		}
		exitChildren = append(exitChildren, node)
		if dep != "" {
			deps[dep] = true
		}
	}
	for i, cond := range sells.StopLoss {
		pct, node, dep, err := sellCondition(cond, false)
		if err != nil {
			addBlocker("sell_conditions.stop_loss[%d] (%s %s %s): %v", i, cond.Indicator, cond.Operator, cond.Value, err)
			continue
		}
		if pct != nil {
			stopLossPct = pct
			continue
		}
		exitChildren = append(exitChildren, node)
		if dep != "" {
			deps[dep] = true
		}
	}
	return takeProfitPct, stopLossPct, exitChildren, deps
}

// sellCondition 编译单条卖出条件。百分比条件（如 "10%" / "-5%"）返回 pct；
// 其余可编译条件返回 Exit 表达式节点。
func sellCondition(cond Condition, takeProfit bool) (*float64, any, string, error) {
	indicator := NormalizeIndicator(cond.Indicator)
	if indicator == "" {
		return nil, nil, "", fmt.Errorf("indicator 为空")
	}
	if !knownIndicatorRe.MatchString(indicator) {
		return nil, nil, "", fmt.Errorf("indicator %q 不是可编译内建指标", indicator)
	}

	switch cond.Operator {
	case "cross_above", "cross_below":
		return nil, nil, "", fmt.Errorf("operator %q 需要历史序列，fail-closed 拒绝", cond.Operator)
	case "near":
		return nil, nil, "", fmt.Errorf("operator %q 无确定性阈值语义，fail-closed 拒绝", cond.Operator)
	case "describe":
		return nil, nil, "", fmt.Errorf("自然语言描述条件无法编译")
	}

	value := strings.Trim(strings.TrimSpace(cond.Value), "\"'`")
	if strings.HasSuffix(value, "%") {
		pctValue, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil {
			return nil, nil, "", fmt.Errorf("百分比值 %q 不是数字", cond.Value)
		}
		pct := pctValue / 100
		if !takeProfit && pct > 0 {
			pct = -pct // 止损百分比按惯例为负（-0.05 = -5%）
		}
		return &pct, nil, "", nil
	}

	op := cond.Operator
	if op != "gt" && op != "lt" && op != "between" {
		return nil, nil, "", fmt.Errorf("未知 operator %q", cond.Operator)
	}
	node, dep, err := conditionToExpr(Condition{Indicator: cond.Indicator, Operator: op, Value: cond.Value})
	if err != nil {
		return nil, nil, "", err
	}
	return nil, node, dep, nil
}

type holdingPeriod struct{ min, max int }

var holdingNumberRe = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

// parseHoldingPeriod 解析 "3-5天" / "10个交易日" / "20日" 之类的持仓周期。
func parseHoldingPeriod(raw string) (holdingPeriod, bool) {
	matches := holdingNumberRe.FindAllString(raw, 2)
	switch len(matches) {
	case 2:
		a, err1 := strconv.ParseFloat(matches[0], 64)
		b, err2 := strconv.ParseFloat(matches[1], 64)
		if err1 != nil || err2 != nil {
			return holdingPeriod{}, false
		}
		lo, hi := a, b
		if lo > hi {
			lo, hi = hi, lo
		}
		if hi <= 0 || hi > 250 {
			return holdingPeriod{}, false
		}
		if lo < 1 {
			lo = 1
		}
		return holdingPeriod{min: int(lo), max: int(hi)}, true
	case 1:
		n, err := strconv.ParseFloat(matches[0], 64)
		if err != nil || n <= 0 || n > 250 {
			return holdingPeriod{}, false
		}
		return holdingPeriod{min: 1, max: int(n)}, true
	default:
		return holdingPeriod{}, false
	}
}

// marketCapBoardFilter 把市值画像映射为 BoardFilter 分桶标记。
// BoardFilter 目前是编译器透传的范围标记，不参与数据过滤；
// 映射只用于记录范式上下文，不改变验证股票池。
func marketCapBoardFilter(marketCap string) []string {
	mc := strings.ToLower(strings.TrimSpace(marketCap))
	switch mc {
	case "small", "mid", "large", "mega":
		return []string{"market_cap:" + mc}
	default:
		if mc == "" {
			return nil
		}
		return []string{"market_cap:" + mc}
	}
}

func buildPromotedDescription(p *Paradigm, holdingNote string) string {
	var b strings.Builder
	if p.Rationale != "" {
		b.WriteString(p.Rationale)
	}
	if holdingNote != "" {
		if b.Len() > 0 {
			b.WriteString("；")
		}
		b.WriteString(fmt.Sprintf("%s默认持仓 %d-%d 个交易日", holdingNote, defaultHoldingMinDays, defaultHoldingMaxDays))
	}
	if len(p.Confirm) > 0 {
		if b.Len() > 0 {
			b.WriteString("；确认项：" + strings.Join(p.Confirm, "、"))
		}
	}
	return b.String()
}

func pctPtr(v float64) *float64 { return &v }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// 排序保证 FeatureDeps 稳定，ContentHash 可复现。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
