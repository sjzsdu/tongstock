package signal

import (
	"fmt"
	"math"

	"github.com/sjzsdu/tongstock/pkg/ta"
)

// TradeCycle represents a complete trading cycle (buy → sell)
type TradeCycle struct {
	Code          string
	BuyDate       string // 买入日期
	BuyPrice      float64
	BuySignal     string // 买入信号来源 (MACD/KDJ/MA)
	SellDate      string // 卖出日期
	SellPrice     float64
	SellSignal    string // 卖出信号来源
	HoldDays      int    // 持有天数
	ReturnPct     float64 // 收益率百分比
	MaxProfit     float64 // 期间最大涨幅
	MaxLoss       float64 // 期间最大跌幅
}

// detectCyclesFromCrosses 从交叉信号中识别完整的买卖周期
// findKlineIndexByDate 根据日期字符串查找K线索引
func findKlineIndexByDate(klines []ta.KlineInput, dateStr string) int {
	for i, k := range klines {
		if k.Time.Format("2006-01-02") == dateStr {
			return i
		}
	}
	return -1
}

func detectCyclesFromCrosses(code string, klines []ta.KlineInput, crosses []int, indicator string) []TradeCycle {
	var cycles []TradeCycle
	
	// crosses: 1 = 金叉(买入), -1 = 死叉(卖出)
	type pendingBuyInfo struct {
		date   string
		price  float64
		signal string
	}
	var pendingBuy *pendingBuyInfo
	
	for i, c := range crosses {
		if c == 0 {
			continue
		}
		
		date := klines[i].Time.Format("2006-01-02")
		price := klines[i].Close
		
		if c == 1 {
			// 金叉 - 记录潜在的买入点（只在没有待处理买入时记录）
			if pendingBuy == nil {
				pendingBuy = &pendingBuyInfo{date: date, price: price, signal: indicator}
			}
		} else if c == -1 && pendingBuy != nil {
			// 死叉 - 完成一个完整周期
			buyIdx := findKlineIndexByDate(klines, pendingBuy.date)
			holdDays := 0
			if buyIdx >= 0 {
				holdDays = i - buyIdx
			}
			cycle := TradeCycle{
				Code:       code,
				BuyDate:    pendingBuy.date,
				BuyPrice:   pendingBuy.price,
				BuySignal:  pendingBuy.signal,
				SellDate:   date,
				SellPrice:  price,
				SellSignal: indicator,
				HoldDays:   holdDays,
			}
			
			// 计算收益率
			if pendingBuy.price > 0 {
				cycle.ReturnPct = (price - pendingBuy.price) / pendingBuy.price * 100
			}
			
			// 计算期间最大涨跌幅
			cycle.MaxProfit, cycle.MaxLoss = calculateMaxProfitLoss(klines, pendingBuy.date, date, pendingBuy.price)
			
			cycles = append(cycles, cycle)
			pendingBuy = nil
		}
	}
	
	return cycles
}

// calculateMaxProfitLoss 计算期间最大涨跌幅
func calculateMaxProfitLoss(klines []ta.KlineInput, startDate, endDate string, buyPrice float64) (maxProfit, maxLoss float64) {
	startIdx := -1
	endIdx := -1
	
	for i, k := range klines {
		dateStr := k.Time.Format("2006-01-02")
		if startIdx == -1 && dateStr >= startDate {
			startIdx = i
		}
		if dateStr <= endDate {
			endIdx = i
		}
	}
	
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return 0, 0
	}
	
	for i := startIdx; i <= endIdx; i++ {
		change := (klines[i].Close - buyPrice) / buyPrice * 100
		if change > maxProfit {
			maxProfit = change
		}
		if change < maxLoss {
			maxLoss = change
		}
	}
	
	return math.Round(maxProfit*100) / 100, math.Round(maxLoss*100) / 100
}

// DetectAllCycles 检测所有完整的交易周期（使用全量历史数据）
func DetectAllCycles(code string, klines []ta.KlineInput, result *ta.IndicatorResult) []TradeCycle {
	var allCycles []TradeCycle
	
	// MACD 金叉死叉周期
	if result.MACD != nil && len(result.MACD.DIF) > 0 {
		crosses := detectLineCross(result.MACD.DIF, result.MACD.DEA)
		cycles := detectCyclesFromCrosses(code, klines, crosses, "MACD")
		allCycles = append(allCycles, cycles...)
	}
	
	// KDJ 金叉死叉周期
	if result.KDJ != nil && len(result.KDJ.K) > 0 {
		crosses := detectLineCross(result.KDJ.K, result.KDJ.D)
		cycles := detectCyclesFromCrosses(code, klines, crosses, "KDJ")
		allCycles = append(allCycles, cycles...)
	}
	
	// MA 金叉死叉周期 (5日上穿10日买入，下穿卖出)
	if result.MA != nil && result.MA["5"] != nil && result.MA["10"] != nil {
		crosses := detectLineCross(result.MA["5"], result.MA["10"])
		cycles := detectCyclesFromCrosses(code, klines, crosses, "MA(5,10)")
		allCycles = append(allCycles, cycles...)
	}
	
	// 对周期按买入日期排序
	// (简单冒泡排序，实际可以用sort包)
	for i := 0; i < len(allCycles)-1; i++ {
		for j := i + 1; j < len(allCycles); j++ {
			if allCycles[j].BuyDate < allCycles[i].BuyDate {
				allCycles[i], allCycles[j] = allCycles[j], allCycles[i]
			}
		}
	}
	
	return allCycles
}

// CycleSummary 返回周期统计信息
func CycleSummary(cycles []TradeCycle) string {
	if len(cycles) == 0 {
		return "无完整交易周期"
	}
	
	totalReturn := 0.0
	winCount := 0
	
	for _, c := range cycles {
		totalReturn += c.ReturnPct
		if c.ReturnPct > 0 {
			winCount++
		}
	}
	
	avgReturn := totalReturn / float64(len(cycles))
	winRate := float64(winCount) / float64(len(cycles)) * 100
	
	return fmt.Sprintf("周期数: %d | 胜率: %.1f%% | 平均收益: %.2f%%", 
		len(cycles), winRate, avgReturn)
}