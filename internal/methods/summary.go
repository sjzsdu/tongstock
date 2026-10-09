package methods

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ExitSummary renders a compiled method's structured exit/holding rules as a
// short human-readable summary. It replaces the earlier circular wording
// ("按退出规则，最长持有 N 个交易日") that repeated the label back at the
// reader and ignored stop loss / take profit / trailing stop.
func ExitSummary(m *CompiledMethod) string {
	if m == nil {
		return "未设定"
	}
	var parts []string
	h := m.Holding
	if h.StopLoss != nil {
		parts = append(parts, fmt.Sprintf("亏损 %s%% 止损", pctText(math.Abs(*h.StopLoss))))
	}
	if h.TakeProfit != nil {
		parts = append(parts, fmt.Sprintf("盈利 %s%% 止盈", pctText(*h.TakeProfit)))
	}
	if h.TrailingStop != nil {
		parts = append(parts, fmt.Sprintf("从最高点回撤 %s%% 移动止盈", pctText(*h.TrailingStop)))
	}
	if h.MaxDays > 0 {
		parts = append(parts, fmt.Sprintf("最长持有 %d 个交易日", h.MaxDays))
	}
	if h.MinDays > 0 {
		parts = append(parts, fmt.Sprintf("至少持有 %d 个交易日", h.MinDays))
	}
	if m.ExitRule != nil {
		parts = append(parts, "触发方法退出条件时卖出")
	}
	if len(parts) == 0 {
		return "未设定"
	}
	return strings.Join(parts, "；")
}

func pctText(v float64) string {
	return strconv.FormatFloat(v*100, 'f', -1, 64)
}
