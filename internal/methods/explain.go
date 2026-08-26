package methods

import (
	"fmt"
	"strconv"
	"strings"
)

// ExplainDiagnostics 将编译诊断翻译为 AI/终端可读解释。
// 按 severity 分块输出: errors → ambiguities → warnings → info。
func ExplainDiagnostics(m *CompiledMethod) string {
	if m == nil {
		return ""
	}
	var errs, ambiguous, warns, info []string
	for _, d := range m.Diagnostics {
		msg := fmt.Sprintf("[%s:%s] %s %s", d.Level, d.Code, d.Path, d.Detail)
		switch d.Level {
		case "error":
			errs = append(errs, msg)
		case "ambiguous":
			ambiguous = append(ambiguous, msg)
		case "warn":
			warns = append(warns, msg)
		default:
			info = append(info, msg)
		}
	}
	for i, a := range m.Ambiguities {
		ambiguous = append(ambiguous, fmt.Sprintf("[ambiguous:%d] %s", i, a))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "方法 %s (compiler=%s hash=%s)\n", m.Name, m.CompilerVersion, m.ContentHash)
	if !m.IsExecutable() {
		fmt.Fprint(&b, "状态: 不可执行（请先修复以下错误或歧义）\n")
	} else {
		fmt.Fprint(&b, "状态: 可执行\n")
	}
	writeBlock(&b, "错误（阻断执行）", errs)
	writeBlock(&b, "歧义（请显式补充说明或生成多个变体）", ambiguous)
	writeBlock(&b, "警告", warns)
	writeBlock(&b, "信息", info)
	return b.String()
}

func writeBlock(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s (%d)\n", title, len(items))
	for _, it := range items {
		fmt.Fprintf(b, "  - %s\n", it)
	}
}

// ExplainTrace 将执行 Trace 翻译为 human-readable 文本，用于 AI 或 UI。
func ExplainTrace(trace []TraceStep) string {
	var b strings.Builder
	for _, s := range trace {
		mark := "✓"
		if !s.Passed {
			mark = "✗"
		}
		expr := s.Expr
		if expr == "" {
			expr = s.Path
		}
		fmt.Fprintf(&b, "%s %s: %s\n", mark, expr, s.Detail)
	}
	return b.String()
}

// SuggestFix 基于诊断给出候选修复建议 (例如缺失退出 → 添加持仓 max_days 作为安全退出)。
// 仅供 AI / UI 作为候选建议, 不自动应用。
func SuggestFix(m *CompiledMethod) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, d := range m.Diagnostics {
		switch d.Code {
		case "EXIT_MISSING":
			out = append(out, "建议补充 exit_rule（例如 close < ma60 触发止损），或设置 holding.max_days = 30 作为到期强制离场")
		case "INDICATOR_EMPTY":
			out = append(out, "请在 Condition 中补充指标名，例如 indicator=close 或 indicator=rsi14")
		case "UNKNOWN_FIELDS":
			out = append(out, "存在未知 JSON 字段；请检查字段名拼写或在方法文档中补充该字段至白名单")
		case "FUTURE_FUNCTION":
			out = append(out, "禁止使用未来函数（future_*/lookahead/next_*）；请使用 lag_* 或历史滚动窗口替代")
		case "OP_UNKNOWN":
			out = append(out, "未知比较操作符，仅允许 {gt,lt,gte,lte,eq} 与 {cross above, cross below}")
		case "NAME_EMPTY":
			out = append(out, "请给方法命名，例如『20日均线突破』『低波成长组合』")
		case "PCT_RANGE", "POS_MODE":
			out = append(out, "pct_equity 取值需 ∈ (0, 1]；kelly_half 为动态仓位无需静态值")
		}
	}
	for i, a := range m.Ambiguities {
		_ = i
		out = append(out, "歧义: "+a+" → 建议提供更精确的条件边界，或显式声明『以上条件全满足/任一满足』")
	}
	if len(out) == 0 {
		out = append(out, "暂无已知修复建议；可通过编译诊断 + 执行 trace 排查")
	}
	return out
}

// FormatVersion 返回人类可读版本号（带编译器和 hash 前缀）。
func FormatVersion(m *CompiledMethod) string {
	if m == nil {
		return ""
	}
	h := m.ContentHash
	if len(h) > 8 {
		h = h[:8]
	}
	return "v" + CompilerVersion + "-" + h
}

// Atoi / 辅助
var _ = strconv.Atoi
