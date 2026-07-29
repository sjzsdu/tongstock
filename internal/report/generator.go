package report

import (
	"fmt"
	"math"
	"time"
)

// ============================================================================
// 报告生成器
// ============================================================================

// ReportGenerator 报告生成器。
type ReportGenerator struct {
	minSampleSize int // 最小样本量阈值
}

// NewReportGenerator 创建报告生成器。
func NewReportGenerator(minSampleSize int) *ReportGenerator {
	if minSampleSize <= 0 {
		minSampleSize = 20 // 默认: 小样本阈值
	}
	return &ReportGenerator{
		minSampleSize: minSampleSize,
	}
}

// Generate 生成统一报告。
func (g *ReportGenerator) Generate(
	dailyReturns []float64,
	equityCurve []float64,
	trades []TradeRecord,
	config *ReportConfig,
) *UnifiedReport {
	report := &UnifiedReport{
		GeneratedAt: time.Now(),
		SampleSize:  len(dailyReturns),
	}

	// 空样本处理
	if len(dailyReturns) == 0 {
		report.IsEmpty = true
		report.Warnings = append(report.Warnings, "数据为空, 无法生成报告")
		report.Notes = append(report.Notes, "请检查数据源和交易记录")
		return report
	}

	// 小样本处理
	if len(dailyReturns) < g.minSampleSize {
		report.IsSmallSample = true
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("样本量较小 (%d < %d), 统计指标可能不稳定", len(dailyReturns), g.minSampleSize))
		report.Notes = append(report.Notes, "增加样本量可提高指标可靠性")
	}

	// 计算基础收益指标
	computeReturnMetrics(dailyReturns, equityCurve, report)

	// 分布分析
	dist := ComputeDistribution(dailyReturns, 10)
	if dist != nil {
		report.Distribution = dist
	}

	// 置信区间
	ci := ComputeConfidenceInterval(dailyReturns, 0.95)
	if ci != nil {
		report.ConfidenceInt = ci

		// 小样本置信区间警告
		if report.IsSmallSample {
			report.Notes = append(report.Notes,
				fmt.Sprintf("95%%置信区间: [%.2f%%, %.2f%%]", ci.Lower*100, ci.Upper*100))
		}
	}

	// 风险分析
	risk := ComputeRiskMetrics(equityCurve, dailyReturns)
	if risk != nil {
		report.Risk = risk
	}

	// 最大回撤详细分析
	dd := ComputeDrawdownAnalysis(equityCurve)
	if dd != nil {
		report.Drawdown = dd
	}

	// 稳定性指标
	if len(trades) > 0 {
		totalDays := len(dailyReturns)
		stability := ComputeStabilityMetrics(trades, totalDays)
		if stability != nil {
			report.Stability = stability
		}
	}

	// 添加空样本/小样本警告
	g.addSampleWarnings(report)

	return report
}

// ReportConfig 报告配置。
type ReportConfig struct {
	RiskFreeRate   float64 // 无风险利率 (用于 Sharpe/Sortino)
	TradingDays    int     // 年化交易天数 (默认 252)
	InitialCapital float64 // 初始资金
}

// DefaultReportConfig 默认报告配置。
func DefaultReportConfig() *ReportConfig {
	return &ReportConfig{
		RiskFreeRate:   0.0,     // 默认 0 (可配置)
		TradingDays:    252,     // A 股年交易天数
		InitialCapital: 1000000, // 默认 100 万
	}
}

// ============================================================================
// 内部计算
// ============================================================================

// computeReturnMetrics 计算收益指标。
func computeReturnMetrics(dailyReturns []float64, equityCurve []float64, report *UnifiedReport) {
	// 总收益
	if len(equityCurve) >= 2 && equityCurve[0] > 0 {
		report.TotalReturn = (equityCurve[len(equityCurve)-1] - equityCurve[0]) / equityCurve[0]
	}

	// 年化收益
	if len(dailyReturns) > 0 {
		stats := ComputeBasicStats(dailyReturns)
		tradingDays := 252.0
		if stats.Mean != 0 {
			report.AnnualReturn = math.Pow(1+stats.Mean, tradingDays) - 1
		}
		report.AvgReturn = stats.Mean

		// 胜率
		wins := 0
		for _, r := range dailyReturns {
			if r > 0 {
				wins++
			}
		}
		report.WinRate = float64(wins) / float64(len(dailyReturns)) * 100

		// 盈亏比
		profits := make([]float64, 0)
		losses := make([]float64, 0)
		for _, r := range dailyReturns {
			if r > 0 {
				profits = append(profits, r)
			} else if r < 0 {
				losses = append(losses, r)
			}
		}
		avgProfit := mean(profits)
		avgLoss := math.Abs(mean(losses))
		if avgLoss > 0 {
			report.ProfitFactor = avgProfit / avgLoss
		}

		// Sharpe Ratio
		if stats.StdDev > 0 {
			excessReturn := stats.Mean
			report.SharpeRatio = excessReturn / stats.StdDev * math.Sqrt(252)
		}

		// Sortino Ratio
		downsideDev := computeDownsideDeviation(dailyReturns)
		if downsideDev > 0 {
			report.SortinoRatio = stats.Mean / downsideDev * math.Sqrt(252)
		}
	}
}

// ComputeDrawdownAnalysis 计算最大回撤详细分析。
func ComputeDrawdownAnalysis(equityCurve []float64) *DrawdownAnalysis {
	if len(equityCurve) < 2 {
		return nil
	}

	// 计算回撤序列
	dds := computeDrawdownSeries(equityCurve)

	// 最大回撤
	maxDD := 0.0
	maxDDStart := 0
	maxDDEnd := 0

	for i, dd := range dds {
		if dd > maxDD {
			maxDD = dd
			maxDDEnd = i
			// 回溯找到回撤起点 (峰值位置)
			peakVal := equityCurve[maxDDEnd] / (1 - dd)
			for j := maxDDEnd; j >= 0; j-- {
				if equityCurve[j] >= peakVal {
					maxDDStart = j
					break
				}
				if j == 0 {
					maxDDStart = 0
				}
			}
		}
	}

	// 平均回撤
	sumDD := 0.0
	for _, dd := range dds {
		sumDD += dd
	}
	avgDD := sumDD / float64(len(dds))

	// 当前回撤
	currentDD := dds[len(dds)-1]

	// 回撤频率: 回撤超过 1% 的次数
	ddCount := 0
	lastDD := 0.0
	for _, dd := range dds {
		if dd > 0.01 && lastDD <= 0.01 {
			ddCount++
		}
		lastDD = dd
	}
	ddFreq := float64(ddCount) / float64(len(dds)) * 252 // 年化

	// 恢复天数
	recoveryDays := computeRecoveryDays(dds)

	daysBetween := maxDDEnd - maxDDStart

	return &DrawdownAnalysis{
		MaxDrawdown:     maxDD,
		MaxDDDuration:   daysBetween,
		AvgDrawdown:     avgDD,
		DrawdownFreq:    ddFreq,
		RecoveryDays:    recoveryDays,
		CurrentDrawdown: currentDD,
		CurrentDDBDays:  len(dds),
	}
}

// computeRecoveryDays 计算每次回撤的恢复天数。
func computeRecoveryDays(dds []float64) []int {
	var recoveryDays []int
	daysInDrawdown := 0
	inDrawdown := false

	for _, dd := range dds {
		if dd > 0 {
			inDrawdown = true
			daysInDrawdown++
		} else if inDrawdown {
			recoveryDays = append(recoveryDays, daysInDrawdown)
			daysInDrawdown = 0
			inDrawdown = false
		}
	}

	return recoveryDays
}

// addSampleWarnings 添加样本相关警告。
func (g *ReportGenerator) addSampleWarnings(report *UnifiedReport) {
	if report.IsEmpty {
		return
	}

	// 空样本/小样本的百分比指标说明
	if report.IsSmallSample {
		report.Notes = append(report.Notes,
			"小样本警告: 百分比指标 (如胜率) 在样本量 < 20 时可能不稳定")
	}

	// 检查是否有误导性的百分比
	if report.SampleSize > 0 && report.SampleSize < 10 {
		// 样本非常小, 胜率等指标意义有限
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("样本量极少 (%d), 指标仅供参考, 可能存在误导", report.SampleSize))

		if report.SampleSize <= 5 {
			report.Notes = append(report.Notes,
				fmt.Sprintf("样本量 ≤ 5: 建议增加至少 %d 倍样本量以获得可靠估计", g.minSampleSize/report.SampleSize))
		}
	}
}

// ============================================================================
// 报告格式化输出
// ============================================================================

// SummaryText 生成报告摘要文本 (供 AI 消费)。
func (r *UnifiedReport) SummaryText() string {
	if r.IsEmpty {
		return "⚠️ 数据为空, 无法生成分析报告。"
	}

	summary := ""
	summary += fmt.Sprintf("📊 统一分析报告 (样本量: %d)\n", r.SampleSize)
	summary += fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 收益指标
	summary += fmt.Sprintf("\n💰 收益指标:\n")
	summary += fmt.Sprintf("  • 总收益: %.2f%%\n", r.TotalReturn*100)
	if r.AnnualReturn != 0 {
		summary += fmt.Sprintf("  • 年化收益: %.2f%%\n", r.AnnualReturn*100)
	}
	summary += fmt.Sprintf("  • 平均日收益: %.4f%%\n", r.AvgReturn*100)
	summary += fmt.Sprintf("  • 胜率: %.2f%%\n", r.WinRate)
	if r.ProfitFactor != 0 {
		summary += fmt.Sprintf("  • 盈亏比: %.2f\n", r.ProfitFactor)
	}
	if r.SharpeRatio != 0 {
		summary += fmt.Sprintf("  • 夏普比率: %.2f\n", r.SharpeRatio)
	}
	if r.SortinoRatio != 0 {
		summary += fmt.Sprintf("  • 索提诺比率: %.2f\n", r.SortinoRatio)
	}

	// 风险指标
	if r.Risk != nil {
		summary += fmt.Sprintf("\n⚠️  风险指标:\n")
		summary += fmt.Sprintf("  • 最大回撤: %.2f%%\n", r.Risk.MaxDrawdown*100)
		summary += fmt.Sprintf("  • 95%% VaR: %.4f\n", r.Risk.VaR_95)
		summary += fmt.Sprintf("  • 99%% VaR: %.4f\n", r.Risk.VaR_99)
		summary += fmt.Sprintf("  • 95%% CVaR: %.4f\n", r.Risk.CVaR_95)
		summary += fmt.Sprintf("  • Ulcer Index: %.4f\n", r.Risk.UlcerIndex)
	}

	// 置信区间
	if r.ConfidenceInt != nil {
		summary += fmt.Sprintf("\n📈 置信区间 (%.0f%%):\n", r.ConfidenceInt.Level*100)
		summary += fmt.Sprintf("  • 日收益: [%.4f%%, %.4f%%]\n", r.ConfidenceInt.Lower*100, r.ConfidenceInt.Upper*100)
	}

	// 分布分析
	if r.Distribution != nil {
		summary += fmt.Sprintf("\n📊 分布分析:\n")
		summary += fmt.Sprintf("  • 偏度: %.2f\n", r.Distribution.Stats.Skewness)
		summary += fmt.Sprintf("  • 峰度: %.2f\n", r.Distribution.Stats.Kurtosis)
		summary += fmt.Sprintf("  • 正态分布假设: %v\n", r.Distribution.IsNormal)
	}

	// 稳定性指标
	if r.Stability != nil {
		summary += fmt.Sprintf("\n🔄 稳定性指标:\n")
		summary += fmt.Sprintf("  • 换手率: %.2f\n", r.Stability.TurnoverRate)
		summary += fmt.Sprintf("  • 平均持有: %.1f 天\n", r.Stability.AvgHoldDays)
		summary += fmt.Sprintf("  • 容量估算: %.0f 万元\n", r.Stability.CapacityEstimate)
		summary += fmt.Sprintf("  • 股票集中度 (HHI): %.4f\n", r.Stability.StockConcentration)
	}

	// 警告信息
	if len(r.Warnings) > 0 {
		summary += fmt.Sprintf("\n⚠️  警告:\n")
		for _, w := range r.Warnings {
			summary += fmt.Sprintf("  • %s\n", w)
		}
	}

	return summary
}

// ============================================================================
// 辅助函数
// ============================================================================

// mean 平均值。
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range values {
		s += v
	}
	return s / float64(len(values))
}
