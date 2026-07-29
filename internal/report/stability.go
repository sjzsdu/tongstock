package report

import (
	"math"
	"sort"
)

// ============================================================================
// 稳定性分析
// ============================================================================

// TradeRecord 交易记录 (用于稳定性分析)。
type TradeRecord struct {
	StockCode  string  `json:"stock_code"`
	EntryDate  string  `json:"entry_date"`
	ExitDate   string  `json:"exit_date"`
	Return     float64 `json:"return"`
	HoldDays   int     `json:"hold_days"`
	Position   float64 `json:"position"` // 持仓金额
}

// ComputeStabilityMetrics 计算稳定性指标。
func ComputeStabilityMetrics(trades []TradeRecord, totalDays int) *StabilityMetrics {
	if len(trades) == 0 {
		return nil
	}

	// 换手率
	turnover := computeTurnover(trades, totalDays)

	// 平均持有天数
	avgHold := computeAvgHoldDays(trades)

	// 交易频率
	tradeFreq := 0.0
	if totalDays > 0 {
		tradeFreq = float64(totalDays) / float64(len(trades))
	}

	// 股票集中度
	stockConcentration := computeStockConcentration(trades)

	// 日期集中度
	dateConcentration := computeDateConcentration(trades)

	// Top 贡献
	topContrib := computeTopContribution(trades)

	// 容量估算
	capacity := estimateCapacity(trades, totalDays)

	// 夏普稳定性
	sharpeStab := computeSharpeStability(trades, totalDays)

	return &StabilityMetrics{
		TurnoverRate:      turnover,
		AvgHoldDays:       avgHold,
		TradeFrequency:    tradeFreq,
		StockConcentration: stockConcentration,
		DateConcentration:  dateConcentration,
		TopPctContrib:     topContrib,
		CapacityEstimate:  capacity,
		SharpeStability:   sharpeStab,
	}
}

// computeTurnover 计算日均换手率。
func computeTurnover(trades []TradeRecord, totalDays int) float64 {
	if totalDays <= 0 {
		return 0
	}

	totalTurnover := 0.0
	for _, t := range trades {
		if t.Position > 0 {
			totalTurnover += t.Position
		}
	}

	// 日均换手率 = 总成交金额 / (交易天数 * 近似资金)
	// 使用简化公式: 2 * trades / days (双边换手率)
	return 2.0 * float64(len(trades)) / float64(totalDays)
}

// computeAvgHoldDays 计算平均持有天数。
func computeAvgHoldDays(trades []TradeRecord) float64 {
	if len(trades) == 0 {
		return 0
	}

	totalDays := 0
	for _, t := range trades {
		totalDays += t.HoldDays
	}

	return float64(totalDays) / float64(len(trades))
}

// computeStockConcentration 计算股票集中度 (HHI 指数)。
func computeStockConcentration(trades []TradeRecord) float64 {
	if len(trades) == 0 {
		return 0
	}

	// 按股票代码分组, 计算每只股票的收益贡献
	stockReturns := make(map[string]float64)
	totalReturn := 0.0

	for _, t := range trades {
		stockReturns[t.StockCode] += t.Return
		totalReturn += math.Abs(t.Return)
	}

	if totalReturn == 0 {
		return 0
	}

	// HHI = sum(share_i^2)
	hhi := 0.0
	for _, ret := range stockReturns {
		share := math.Abs(ret) / totalReturn
		hhi += share * share
	}

	return hhi
}

// computeDateConcentration 计算日期集中度。
func computeDateConcentration(trades []TradeRecord) float64 {
	if len(trades) == 0 {
		return 0
	}

	// 按退出日期分组
	dateReturns := make(map[string]float64)
	totalReturn := 0.0

	for _, t := range trades {
		dateReturns[t.ExitDate] += t.Return
		totalReturn += math.Abs(t.Return)
	}

	if totalReturn == 0 {
		return 0
	}

	// HHI
	hhi := 0.0
	for _, ret := range dateReturns {
		share := math.Abs(ret) / totalReturn
		hhi += share * share
	}

	return hhi
}

// computeTopContribution 计算 Top 贡献百分比。
func computeTopContribution(trades []TradeRecord) float64 {
	if len(trades) == 0 {
		return 0
	}

	// 按收益排序
	returns := make([]float64, len(trades))
	for i, t := range trades {
		returns[i] = t.Return
	}
	sort.Float64s(returns)

	totalAbsReturn := 0.0
	for _, r := range returns {
		totalAbsReturn += math.Abs(r)
	}

	if totalAbsReturn == 0 {
		return 0
	}

	// Top 20% 的贡献
	topCount := int(math.Ceil(float64(len(returns)) * 0.2))
	topReturn := 0.0
	for i := len(returns) - topCount; i < len(returns); i++ {
		if i >= 0 {
			topReturn += math.Abs(returns[i])
		}
	}

	return topReturn / totalAbsReturn * 100
}

// estimateCapacity 估算策略容量 (简化版)。
func estimateCapacity(trades []TradeRecord, totalDays int) float64 {
	if len(trades) == 0 || totalDays <= 0 {
		return 0
	}

	// 计算平均单笔持仓规模
	totalPosition := 0.0
	for _, t := range trades {
		totalPosition += t.Position
	}
	avgPosition := totalPosition / float64(len(trades))

	// 基于市场深度估算容量
	// 简化假设: 策略容量 ≈ 平均持仓 * 20 (流动性假设)
	return avgPosition * 20 / 10000 // 转换为万元
}

// computeSharpeStability 计算夏普稳定性 (滚动夏普的变异系数)。
func computeSharpeStability(trades []TradeRecord, totalDays int) float64 {
	if len(trades) < 10 {
		return 0
	}

	// 将交易按周分组, 计算每周的夏普比率
	weeklyReturns := make(map[int][]float64)
	for _, t := range trades {
		// 简化: 按交易序号分组
		week := len(weeklyReturns) / 7
		weeklyReturns[week] = append(weeklyReturns[week], t.Return)
	}

	if len(weeklyReturns) < 4 {
		return 0
	}

	// 计算每周夏普
	sharpes := make([]float64, 0)
	for _, weekReturns := range weeklyReturns {
		if len(weekReturns) >= 2 {
			mean := 0.0
			for _, r := range weekReturns {
				mean += r
			}
			mean /= float64(len(weekReturns))

			std := 0.0
			for _, r := range weekReturns {
				diff := r - mean
				std += diff * diff
			}
			std = math.Sqrt(std / float64(len(weekReturns)))

			if std > 0 {
				sharpes = append(sharpes, mean/std)
			}
		}
	}

	if len(sharpes) < 2 {
		return 0
	}

	// 夏普变异系数
	meanSharpe := 0.0
	for _, s := range sharpes {
		meanSharpe += s
	}
	meanSharpe /= float64(len(sharpes))

	if math.Abs(meanSharpe) < 0.01 {
		return 0
	}

	stdSharpe := 0.0
	for _, s := range sharpes {
		diff := s - meanSharpe
		stdSharpe += diff * diff
	}
	stdSharpe = math.Sqrt(stdSharpe / float64(len(sharpes)))

	return stdSharpe / math.Abs(meanSharpe)
}
