// Compare 模块: 对比理论回测与实际前向表现, 量化执行差距。
package ledger

import (
	"fmt"
	"time"
)

// ComparisonReport 对比报告: 理论回测 vs 实际前向
type ComparisonReport struct {
	ParadigmVersionID string    `json:"paradigm_version_id"`
	RunID             string    `json:"run_id"`
	CompareDate       time.Time `json:"compare_date"`

	// 理论回测 (从范式验证阶段)
	Theoretical TheoreticalMetrics `json:"theoretical"`
	// 实际前向 (Paper Trading)
	Actual ActualMetrics `json:"actual"`
	// 差距分析
	Gap GapAnalysis `json:"gap"`
}

// TheoreticalMetrics 理论回测指标
type TheoreticalMetrics struct {
	TotalReturn   float64 `json:"total_return"`
	AnnualizedRet float64 `json:"annualized_ret"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	SharpeRatio   float64 `json:"sharpe_ratio"`
	WinRate       float64 `json:"win_rate"`
	SignalCount   int     `json:"signal_count"`
	// 交易全部按理想价格成交 (无滑点, 无约束)
	IdealPnL float64 `json:"ideal_pnl"`
}

// ActualMetrics 实际前向指标 (受 A 股约束)
type ActualMetrics struct {
	TotalReturn   float64 `json:"total_return"`
	AnnualizedRet float64 `json:"annualized_ret"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	SharpeRatio   float64 `json:"sharpe_ratio"`
	WinRate       float64 `json:"win_rate"`
	SignalCount   int     `json:"signal_count"`
	FilledCount   int     `json:"filled_count"`
	RejectedCount int     `json:"rejected_count"`
	// 实际成交价格, 受滑点和约束影响
	ActualPnL float64 `json:"actual_pnl"`
	// 因约束损失的机会
	MissedTrades int `json:"missed_trades"`
}

// GapAnalysis 差距分析
type GapAnalysis struct {
	ReturnGap        float64 `json:"return_gap"`        // 理论收益 - 实际收益
	ReturnGapPct     float64 `json:"return_gap_pct"`    // 收益差距百分比
	DrawdownGap      float64 `json:"drawdown_gap"`      // 理论回撤 - 实际回撤
	SharpeGap        float64 `json:"sharpe_gap"`        // 夏普比率差距
	WinRateGap       float64 `json:"win_rate_gap"`      // 胜率差距
	ExecLoss         float64 `json:"exec_loss"`         // 因执行约束损失的收益
	ConstraintImpact float64 `json:"constraint_impact"` // 约束影响: 0=无影响, 1=完全无效
	// 关键差异原因
	KeyInsights []string `json:"key_insights"`
}

// NewComparisonReport 对比理论回测与实际前向
func NewComparisonReport(
	paradigmVersionID string,
	runID string,
	theoretical TheoreticalMetrics,
	run *ForwardRun,
	entries []SignalEntry,
) *ComparisonReport {
	actual := calcActualMetrics(run, entries)
	gap := calcGap(theoretical, actual)

	return &ComparisonReport{
		ParadigmVersionID: paradigmVersionID,
		RunID:             runID,
		CompareDate:       time.Now(),
		Theoretical:       theoretical,
		Actual:            actual,
		Gap:               gap,
	}
}

// calcActualMetrics 从前向运行和信号计算实际指标
func calcActualMetrics(run *ForwardRun, entries []SignalEntry) ActualMetrics {
	var filled, rejected, totalPnL float64
	var missedTrades int
	var wins, losses int

	for _, e := range entries {
		if e.Execution == nil {
			continue
		}
		switch e.Execution.Status {
		case "filled", "partial":
			filled++
			totalPnL += e.Execution.PnL
			if e.Execution.PnL > 0 {
				wins++
			} else if e.Execution.PnL < 0 {
				losses++
			}
		case "rejected":
			rejected++
			missedTrades++
		}
	}

	actual := ActualMetrics{
		TotalReturn:   run.TotalReturn,
		AnnualizedRet: run.TotalReturn, // 简化, 实际需要按年化计算
		MaxDrawdown:   run.MaxDrawdown,
		SharpeRatio:   run.SharpeRatio,
		WinRate:       run.WinRate,
		SignalCount:   run.SignalCount,
		FilledCount:   int(filled),
		RejectedCount: int(rejected),
		ActualPnL:     totalPnL,
		MissedTrades:  missedTrades,
	}

	if actual.WinRate == 0 && wins+losses > 0 {
		actual.WinRate = float64(wins) / float64(wins+losses)
	}

	return actual
}

// calcGap 计算理论 vs 实际的差距
func calcGap(theoretical TheoreticalMetrics, actual ActualMetrics) GapAnalysis {
	gap := GapAnalysis{
		ReturnGap:   theoretical.TotalReturn - actual.TotalReturn,
		DrawdownGap: actual.MaxDrawdown - theoretical.MaxDrawdown, // 实际回撤可能更大
		SharpeGap:   theoretical.SharpeRatio - actual.SharpeRatio,
		WinRateGap:  theoretical.WinRate - actual.WinRate,
		KeyInsights: make([]string, 0),
	}

	// 计算约束影响
	if theoretical.TotalReturn > 0 {
		gap.ReturnGapPct = gap.ReturnGap / theoretical.TotalReturn
	}
	if actual.FilledCount+actual.RejectedCount > 0 {
		gap.ExecLoss = float64(actual.RejectedCount) / float64(actual.FilledCount+actual.RejectedCount)
		gap.ConstraintImpact = gap.ExecLoss
	}

	// 生成关键洞察
	if gap.ReturnGapPct > 0.2 {
		gap.KeyInsights = append(gap.KeyInsights,
			fmt.Sprintf("收益差距 %.1f%% 显著, 可能受交易约束影响", gap.ReturnGapPct*100))
	}
	if actual.RejectedCount > 0 {
		gap.KeyInsights = append(gap.KeyInsights,
			fmt.Sprintf("%d 笔信号被 A 股约束拒绝 (T+1/涨跌停/停牌)", actual.RejectedCount))
	}
	if actual.MaxDrawdown > theoretical.MaxDrawdown*1.5 {
		gap.KeyInsights = append(gap.KeyInsights,
			"实际回撤显著大于理论回撤, 风险控制可能不足")
	}
	if gap.WinRateGap > 0.1 {
		gap.KeyInsights = append(gap.KeyInsights,
			"实际胜率低于理论胜率, 信号质量可能受市场状态影响")
	}
	if len(gap.KeyInsights) == 0 {
		gap.KeyInsights = append(gap.KeyInsights, "前向表现与理论回测一致, 范式有效性得到验证")
	}

	return gap
}

// ValidateGapThreshold 检查差距是否在可接受范围内
func (r *ComparisonReport) ValidateGapThreshold(maxReturnGapPct, maxDrawdownGap float64) (bool, []string) {
	var warnings []string
	ok := true

	if r.Gap.ReturnGapPct > maxReturnGapPct {
		ok = false
		warnings = append(warnings,
			fmt.Sprintf("收益差距 %.1f%% 超过阈值 %.1f%%", r.Gap.ReturnGapPct*100, maxReturnGapPct*100))
	}
	if r.Gap.DrawdownGap > maxDrawdownGap {
		ok = false
		warnings = append(warnings,
			fmt.Sprintf("回撤差距 %.1f%% 超过阈值 %.1f%%", r.Gap.DrawdownGap*100, maxDrawdownGap*100))
	}

	return ok, warnings
}
