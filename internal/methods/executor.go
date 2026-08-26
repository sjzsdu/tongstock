package methods

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Bar 是执行器输入的单行 K 线 (和衍生指标。真实值。不包含未来数据字段；
// 未来数据会被 detectFutureFunctions 在编译期拒绝，这里保持对真实数据的绝对信任。
type Bar struct {
	Date   string // YYYY-MM-DD 格式，执行期日
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64

	// Indicators 保存内建或外部衍生指标, 命名需与 Compile 时期的 indicator 归一化名字对齐:
	// 例: ma5 / rsi14 / macd_dif / macd_dea / boll_upper。
	Indicators map[string]float64
}

// Result 是一次执行结果，包含逐条件解释、布尔结论、可复现的执行轨迹。
type Result struct {
	Matched bool        `json:"matched"`
	Date    string      `json:"date"`
	Trace   []TraceStep `json:"trace"`
	Value   float64     `json:"value,omitempty"`
	Explain string      `json:"explain,omitempty"`
}

// TraceStep 保存子条件的执行轨迹:
//  1. Path:   条件路径, 例 entry.left
//  2. Name:   表达式文本形式
//  3. Passed: 是否满足
//  4. Detail: 真实数值摘要, 例 "close=10.20 gt ma20=10.05"
type TraceStep struct {
	Path   string `json:"path"`
	Expr   string `json:"expr,omitempty"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// Entry 在当前 bar 上执行 method 的入场规则 (buy)。
// 失败原因会写进 Result.Trace 方便 explain。
func (c *CompiledMethod) Entry(bar Bar, all []Bar) (*Result, error) {
	if c == nil {
		return nil, errors.New("nil method")
	}
	if !c.IsExecutable() {
		return &Result{Matched: false, Explain: "method 不满足可执行条件，请先 resolve ambiguities 或修复 error 级诊断"}, nil
	}
	env := &evalEnv{bar: bar, all: all}
	res := &Result{Date: bar.Date}
	passed, trace := evalExpr(c.EntryRule, env, "entry")
	res.Matched = passed
	res.Trace = trace
	return res, nil
}

// Exit 在当前持仓情况下检查出场条件。当 exit_rule 为 nil 时仅检查持有周期硬终止。
// positionEntry 是入场时的价格/日期信息 (用于 max_days/stop_loss 检查)。
func (c *CompiledMethod) Exit(bar Bar, all []Bar, positionEntry *PositionState) (*Result, error) {
	if c == nil {
		return nil, errors.New("nil method")
	}
	env := &evalEnv{bar: bar, all: all}
	res := &Result{Date: bar.Date}
	if c.ExitRule != nil && c.IsExecutable() {
		passed, trace := evalExpr(c.ExitRule, env, "exit")
		res.Matched = passed
		res.Trace = trace
	}
	// 硬终止条件叠加: max_days / stop_loss / take_profit / trailing_stop
	hard := c.Holding
	if positionEntry != nil {
		if hard.MaxDays > 0 && positionEntry.EntryDate != "" {
			if elapsed, daysErr := daysSince(positionEntry.EntryDate, bar.Date); daysErr == nil && elapsed >= hard.MaxDays {
				res.Matched = true
				res.Trace = append(res.Trace, TraceStep{
					Path: "holding.max_days", Passed: true,
					Detail: fmt.Sprintf("持仓 %d 天超过 max_days=%d", elapsed, hard.MaxDays),
				})
			}
		}
		if hard.StopLoss != nil && positionEntry.EntryPrice > 0 {
			ret := (bar.Close - positionEntry.EntryPrice) / positionEntry.EntryPrice
			if ret <= *hard.StopLoss {
				res.Matched = true
				res.Trace = append(res.Trace, TraceStep{
					Path: "holding.stop_loss", Passed: true,
					Detail: fmt.Sprintf("回撤 %.2f%% <= stop_loss=%.2f%%", ret*100, *hard.StopLoss*100),
				})
			}
		}
		if hard.TakeProfit != nil && positionEntry.EntryPrice > 0 {
			ret := (bar.Close - positionEntry.EntryPrice) / positionEntry.EntryPrice
			if ret >= *hard.TakeProfit {
				res.Matched = true
				res.Trace = append(res.Trace, TraceStep{
					Path: "holding.take_profit", Passed: true,
					Detail: fmt.Sprintf("盈利 %.2f%% >= take_profit=%.2f%%", ret*100, *hard.TakeProfit*100),
				})
			}
		}
	}
	return res, nil
}

// PositionState 是执行器需要的最小持仓状态 (仅含价格/入场日期，避免执行层访问总账)。
type PositionState struct {
	EntryPrice float64
	EntryDate  string
}

// evalEnv 是单次表达式求值环境。
type evalEnv struct {
	bar Bar
	all []Bar
}

// evalExpr 返回 (passed, trace)。
func evalExpr(e *Expr, env *evalEnv, path string) (bool, []TraceStep) {
	if e == nil {
		return false, []TraceStep{{Path: path, Expr: "nil", Passed: false, Detail: "expression missing"}}
	}
	switch e.Type {
	case NodeIndicator:
		v, ok := indicatorValue(env, e.Indicator, e.Params)
		step := TraceStep{Path: path, Expr: fmt.Sprintf("indicator(%s)", e.Indicator), Passed: ok}
		if ok {
			step.Detail = fmt.Sprintf("%s=%.4f", e.Indicator, v)
		} else {
			step.Detail = fmt.Sprintf("%s missing from bar indicators", e.Indicator)
		}
		return ok, []TraceStep{step}
	case NodeConstant:
		v := 0.0
		if e.Value != nil {
			v = *e.Value
		}
		return true, []TraceStep{{Path: path, Expr: "constant", Passed: true, Detail: fmt.Sprintf("%.4f", v)}}
	case NodeCompare:
		lv, lOK := evalValue(e.Left, env)
		rv, rOK := evalValue(e.Right, env)
		ok := lOK && rOK
		passed := false
		if ok {
			switch e.Op {
			case CmpGT:
				passed = lv > rv
			case CmpLT:
				passed = lv < rv
			case CmpGTE:
				passed = lv >= rv
			case CmpLTE:
				passed = lv <= rv
			case CmpEQ:
				passed = almostEq(lv, rv)
			}
		}
		step := TraceStep{
			Path:   path,
			Expr:   fmt.Sprintf("(%s %s %s)", shortName(e.Left), e.Op, shortName(e.Right)),
			Passed: passed && ok,
			Detail: fmt.Sprintf("left=%.4f op=%s right=%.4f ok=%v", lv, e.Op, rv, ok),
		}
		trace := []TraceStep{step}
		_, lT := evalExpr(e.Left, env, path+".left")
		_, rT := evalExpr(e.Right, env, path+".right")
		trace = append(trace, lT...)
		trace = append(trace, rT...)
		return passed && ok, trace
	case NodeAnd:
		passed := len(e.Children) > 0
		trace := []TraceStep{}
		for i, ch := range e.Children {
			cp, ct := evalExpr(ch, env, fmt.Sprintf("%s[%d]", path, i))
			trace = append(trace, ct...)
			if !cp {
				passed = false
			}
		}
		return passed, append([]TraceStep{{Path: path, Expr: "and", Passed: passed}}, trace...)
	case NodeOr:
		passed := false
		trace := []TraceStep{}
		for i, ch := range e.Children {
			cp, ct := evalExpr(ch, env, fmt.Sprintf("%s[%d]", path, i))
			trace = append(trace, ct...)
			if cp {
				passed = true
			}
		}
		return passed, append([]TraceStep{{Path: path, Expr: "or", Passed: passed}}, trace...)
	case NodeNot:
		if len(e.Children) != 1 {
			return false, []TraceStep{{Path: path, Passed: false, Detail: "not needs 1 child"}}
		}
		childPassed, ct := evalExpr(e.Children[0], env, fmt.Sprintf("%s[0]", path))
		return !childPassed, append([]TraceStep{{Path: path, Expr: "not", Passed: !childPassed}}, ct...)
	case NodeCross:
		if len(env.all) < 2 {
			return false, []TraceStep{{Path: path, Passed: false, Detail: "cross requires at least 2 bars"}}
		}
		prev := env.all[len(env.all)-2]
		prevEnv := &evalEnv{bar: prev, all: env.all[:len(env.all)-1]}
		lv, _ := evalValue(e.Left, env)
		rv, _ := evalValue(e.Right, env)
		lvp, _ := evalValue(e.Left, prevEnv)
		rvp, _ := evalValue(e.Right, prevEnv)
		var passed bool
		switch e.Cross {
		case "above":
			passed = lvp <= rvp && lv > rv
		case "below":
			passed = lvp >= rvp && lv < rv
		default:
			return false, []TraceStep{{Path: path, Passed: false, Detail: fmt.Sprintf("unknown cross %q", e.Cross)}}
		}
		return passed, []TraceStep{{
			Path:   path,
			Expr:   fmt.Sprintf("cross %s %s %s", shortName(e.Left), e.Cross, shortName(e.Right)),
			Passed: passed,
			Detail: fmt.Sprintf("prev %.4f %.4f → now %.4f %.4f", lvp, rvp, lv, rv),
		}}
	case NodeInWindow:
		mode := firstNonEmpty(e.WindowMode, "any")
		window := e.WindowDays
		if window > len(env.all) {
			window = len(env.all)
		}
		start := len(env.all) - window
		if start < 0 {
			start = 0
		}
		matchedCount := 0
		totalChecked := 0
		var trace []TraceStep
		for i := start; i < len(env.all); i++ {
			sub := &evalEnv{bar: env.all[i], all: env.all[:i+1]}
			for _, ch := range e.Children {
				cp, ct := evalExpr(ch, sub, fmt.Sprintf("%s.d%d]", path, i))
				trace = append(trace, ct...)
				totalChecked++
				if cp {
					matchedCount++
				}
			}
		}
		passed := false
		switch mode {
		case "any":
			passed = matchedCount > 0
		case "all":
			passed = totalChecked > 0 && matchedCount == totalChecked
		}
		return passed, append([]TraceStep{{
			Path:   path,
			Expr:   fmt.Sprintf("in_window %d %s", e.WindowDays, mode),
			Passed: passed,
			Detail: fmt.Sprintf("matched %d / %d checked", matchedCount, totalChecked),
		}}, trace...)
	case NodeAmbiguous:
		return false, []TraceStep{{
			Path: path, Expr: "AMBIGUOUS(" + e.AmbiguousSource + ")", Passed: false,
			Detail: strings.Join(e.AmbiguousReasons, "; "),
		}}
	case NodeRank:
		// 横截面排名需要外部特征快照，不在当前执行范围。
		// 设计为 fail-closed: 缺失横截面数据 → false + 清晰 trace。
		return false, []TraceStep{{
			Path:   path,
			Expr:   fmt.Sprintf("rank %s %s %.1f%%", e.RankSide, e.RankBy, e.RankPct*100),
			Passed: false,
			Detail: "横截面执行需要外部横截面快照, 当前单股票仅作 not available, fail closed",
		}}
	}
	return false, []TraceStep{{Path: path, Passed: false, Detail: fmt.Sprintf("unknown node %q", e.Type)}}
}

// evalValue 将 expr 的数值结果: indicator/constant/comparison 或组合 (and/or 视为 0/1)。
func evalValue(e *Expr, env *evalEnv) (float64, bool) {
	if e == nil {
		return 0, false
	}
	switch e.Type {
	case NodeIndicator:
		return indicatorValue(env, e.Indicator, e.Params)
	case NodeConstant:
		if e.Value == nil {
			return 0, false
		}
		return *e.Value, true
	case NodeCompare:
		p, _ := evalExpr(e, env, "_")
		if p {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func indicatorValue(env *evalEnv, name string, params []string) (float64, bool) {
	switch strings.ToLower(name) {
	case "open":
		return env.bar.Open, true
	case "high":
		return env.bar.High, true
	case "low":
		return env.bar.Low, true
	case "close":
		return env.bar.Close, true
	case "volume":
		return env.bar.Volume, true
	case "amount":
		return env.bar.Amount, true
	}
	// A frozen FeatureSnapshot is the authoritative point-in-time value in
	// daily decision engines. Prefer it over recomputing a rolling indicator
	// from an incomplete one-bar execution window.
	if env.bar.Indicators != nil {
		if v, ok := env.bar.Indicators[name]; ok {
			return v, true
		}
	}
	// 参数化 MA: 名字是 "ma" 加 params[0] = N → 返回 N 日均收盘价均线。
	if strings.HasPrefix(name, "ma") {
		if n, err := parseIntSuffix(name, 2); err == nil {
			return rollingMeanClose(env, n)
		}
		if len(params) > 0 {
			if n, err := parseInt(params[0]); err == nil && n > 0 {
				return rollingMeanClose(env, n)
			}
		}
	}
	if strings.HasPrefix(name, "rsi") {
		if n, err := parseIntSuffix(name, 3); err == nil && n > 0 {
			return rsi(env, n)
		}
	}
	if strings.HasPrefix(name, "prevhigh") {
		if n, err := parseIntSuffix(name, len("prevhigh")); err == nil && n > 0 {
			return rollingPreviousCloseExtreme(env, n, true)
		}
	}
	if strings.HasPrefix(name, "prevlow") {
		if n, err := parseIntSuffix(name, len("prevlow")); err == nil && n > 0 {
			return rollingPreviousCloseExtreme(env, n, false)
		}
	}
	if strings.HasPrefix(name, "volma") {
		if n, err := parseIntSuffix(name, len("volma")); err == nil && n > 0 {
			return rollingMeanVolume(env, n)
		}
	}
	if strings.HasPrefix(name, "volatility") {
		if n, err := parseIntSuffix(name, len("volatility")); err == nil && n > 1 {
			return rollingVolatility(env, n)
		}
	}
	if name == "return1" && len(env.all) >= 2 {
		previous := env.all[len(env.all)-2].Close
		if previous > 0 {
			return (env.bar.Close - previous) / previous, true
		}
	}
	if name == "gap_pct" && len(env.all) >= 2 {
		previous := env.all[len(env.all)-2].Close
		if previous > 0 {
			return (env.bar.Open - previous) / previous, true
		}
	}
	return 0, false
}

func rollingMeanClose(env *evalEnv, n int) (float64, bool) {
	if n <= 0 || len(env.all) < n {
		return 0, false
	}
	sum := 0.0
	for i := len(env.all) - n; i < len(env.all); i++ {
		sum += env.all[i].Close
	}
	return sum / float64(n), true
}

func rollingMeanVolume(env *evalEnv, n int) (float64, bool) {
	if n <= 0 || len(env.all) < n {
		return 0, false
	}
	var sum float64
	for i := len(env.all) - n; i < len(env.all); i++ {
		sum += env.all[i].Volume
	}
	return sum / float64(n), true
}

func rollingPreviousCloseExtreme(env *evalEnv, n int, maximum bool) (float64, bool) {
	if n <= 0 || len(env.all) < n+1 {
		return 0, false
	}
	start, end := len(env.all)-n-1, len(env.all)-1
	value := env.all[start].Close
	for i := start + 1; i < end; i++ {
		if maximum && env.all[i].Close > value {
			value = env.all[i].Close
		}
		if !maximum && env.all[i].Close < value {
			value = env.all[i].Close
		}
	}
	return value, true
}

func rollingVolatility(env *evalEnv, n int) (float64, bool) {
	if n <= 1 || len(env.all) < n+1 {
		return 0, false
	}
	returns := make([]float64, 0, n)
	for i := len(env.all) - n; i < len(env.all); i++ {
		previous := env.all[i-1].Close
		if previous <= 0 {
			return 0, false
		}
		returns = append(returns, (env.all[i].Close-previous)/previous)
	}
	var mean float64
	for _, value := range returns {
		mean += value
	}
	mean /= float64(len(returns))
	var variance float64
	for _, value := range returns {
		delta := value - mean
		variance += delta * delta
	}
	return math.Sqrt(variance / float64(len(returns)-1)), true
}

func rsi(env *evalEnv, n int) (float64, bool) {
	if n <= 0 || len(env.all) < n+1 {
		return 0, false
	}
	start := len(env.all) - n - 1
	var gains, losses float64
	count := 0
	for i := start + 1; i < len(env.all); i++ {
		diff := env.all[i].Close - env.all[i-1].Close
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
		count++
	}
	if count == 0 || losses == 0 {
		if gains == 0 {
			return 50.0, true
		}
		return 100.0, true
	}
	avgGain := gains / float64(count)
	avgLoss := losses / float64(count)
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs), true
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
func parseIntSuffix(s string, prefixLen int) (int, error) {
	if len(s) <= prefixLen {
		return 0, fmt.Errorf("not a suffix int: %q", s)
	}
	return parseInt(s[prefixLen:])
}

// daysSince 返回两个 YYYY-MM-DD 格式之间的天数差异。
func daysSince(a, b string) (int, error) {
	// 这里只做日差的简易近似, 避免 time.Parse 失败时零值
	layout := "2006-01-02"
	ta, err := timeParse(layout, a)
	if err != nil {
		return 0, err
	}
	tb, err := timeParse(layout, b)
	if err != nil {
		return 0, err
	}
	return int(tb.Sub(ta).Hours() / 24), nil
}

// timeParse / almostEq 等辅助: 防止 compiler 中的 sort 占位修复:
func timeParse(layout, value string) (time.Time, error) { return time.Parse(layout, value) }
func almostEq(a, b float64) bool                        { return math.Abs(a-b) < 1e-9 }

func shortName(e *Expr) string {
	if e == nil {
		return "nil"
	}
	switch e.Type {
	case NodeIndicator:
		return e.Indicator
	case NodeConstant:
		if e.Value != nil {
			return fmt.Sprintf("%.2f", *e.Value)
		}
		return "0"
	case NodeCompare:
		return fmt.Sprintf("compare(%s %s %s)", shortName(e.Left), e.Op, shortName(e.Right))
	}
	return string(e.Type)
}
