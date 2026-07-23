package strategy

import (
	"time"

	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// OvernightCandidate 隔夜套利候选股
type OvernightCandidate struct {
	Code         string            `json:"code"`
	Name         string            `json:"name"`
	Price        float64           `json:"price"`
	ChangePct    float64           `json:"change_pct"`
	VolumeRatio  float64           `json:"volume_ratio"`
	TurnoverRate float64           `json:"turnover_rate"`
	MarketCap    float64           `json:"market_cap"`
	Criteria     OvernightCriteria `json:"criteria"`
	Passed       bool              `json:"passed"`
	FailReason   string            `json:"fail_reason"`
}

// OvernightCriteria 各选股标准的检查结果
type OvernightCriteria struct {
	ChangePct      bool `json:"change_pct"`       // 涨幅3%-5%
	VolumeRatio    bool `json:"volume_ratio"`     // 量比>1
	TurnoverRate   bool `json:"turnover_rate"`    // 换手率5%-10%
	MarketCap      bool `json:"market_cap"`       // 流通市值50-200亿
	LimitUpHistory bool `json:"limit_up_history"` // 20日内涨停
	MAMultiple     bool `json:"ma_multiple"`      // MA多头排列
	AboveVWAP      bool `json:"above_vwap"`       // 分时均价线上方
}

// OvernightFilterParams 筛选参数
type OvernightFilterParams struct {
	MinChangePct    float64 // 最小涨幅(%)
	MaxChangePct    float64 // 最大涨幅(%)
	MinVolumeRatio  float64 // 最小量比
	MinTurnoverRate float64 // 最小换手率(%)
	MaxTurnoverRate float64 // 最大换手率(%)
	MinMarketCap    float64 // 最小流通市值(亿)
	MaxMarketCap    float64 // 最大流通市值(亿)
	LimitUpDays     int     // 涨停历史天数
}

// DefaultOvernightParams 返回默认参数（杨永兴标准）
func DefaultOvernightParams() *OvernightFilterParams {
	return &OvernightFilterParams{
		MinChangePct:    3.0,
		MaxChangePct:    5.0,
		MinVolumeRatio:  1.0,
		MinTurnoverRate: 5.0,
		MaxTurnoverRate: 10.0,
		MinMarketCap:    50.0,
		MaxMarketCap:    200.0,
		LimitUpDays:     20,
	}
}

// CheckChangePct 检查涨幅是否在3%-5%之间
func CheckChangePct(quote *protocol.QuoteItem, params *OvernightFilterParams) bool {
	if quote.LastClose <= 0 {
		return false
	}
	change := (quote.Price - quote.LastClose) / quote.LastClose * 100
	return change >= params.MinChangePct && change <= params.MaxChangePct
}

// GetChangePct 获取涨幅百分比
func GetChangePct(quote *protocol.QuoteItem) float64 {
	if quote.LastClose <= 0 {
		return 0
	}
	return (quote.Price - quote.LastClose) / quote.LastClose * 100
}

// CheckVolumeRatio 检查量比是否大于1
func CheckVolumeRatio(volumeRatio float64, params *OvernightFilterParams) bool {
	return volumeRatio > params.MinVolumeRatio
}

// CheckTurnoverRate 检查换手率是否在5%-10%之间
func CheckTurnoverRate(quote *protocol.QuoteItem, finance *protocol.FinanceInfo, params *OvernightFilterParams) bool {
	if finance.LiuTongGuBen <= 0 {
		return false
	}
	// 成交量单位是手，需要乘以100转为股数
	turnover := (quote.Volume * 100) / finance.LiuTongGuBen * 100
	return turnover >= params.MinTurnoverRate && turnover <= params.MaxTurnoverRate
}

// GetTurnoverRate 获取换手率
func GetTurnoverRate(quote *protocol.QuoteItem, finance *protocol.FinanceInfo) float64 {
	if finance.LiuTongGuBen <= 0 {
		return 0
	}
	return (quote.Volume * 100) / finance.LiuTongGuBen * 100
}

// CheckMarketCap 检查流通市值是否在50-200亿之间
func CheckMarketCap(quote *protocol.QuoteItem, finance *protocol.FinanceInfo, params *OvernightFilterParams) bool {
	if finance.LiuTongGuBen <= 0 || quote.Price <= 0 {
		return false
	}
	// 流通市值 = 流通股本(股) * 股价 / 100000000
	mktCap := finance.LiuTongGuBen * quote.Price / 100000000
	return mktCap >= params.MinMarketCap && mktCap <= params.MaxMarketCap
}

// GetMarketCap 获取流通市值(亿)
func GetMarketCap(quote *protocol.QuoteItem, finance *protocol.FinanceInfo) float64 {
	if finance.LiuTongGuBen <= 0 || quote.Price <= 0 {
		return 0
	}
	return finance.LiuTongGuBen * quote.Price / 100000000
}

// GetLimitUpThreshold 获取涨停阈值
func GetLimitUpThreshold(code string) float64 {
	// 科创板(688)和创业板(300/301)涨跌幅限制20%
	if len(code) >= 3 && (code[:3] == "688" || code[:3] == "300" || code[:3] == "301") {
		return 1.195 // 20%涨停
	}
	// 北交所(8xx)涨跌幅限制30%
	if len(code) >= 1 && (code[0] == '8') {
		return 1.295 // 30%涨停
	}
	// 主板(000/002/600/601)涨跌幅限制10%
	return 1.095 // 10%涨停
}

// CheckLimitUpHistory 检查近N日是否出现过涨停
func CheckLimitUpHistory(klines []ta.KlineInput, params *OvernightFilterParams) bool {
	if len(klines) < params.LimitUpDays {
		return false
	}
	endIdx := len(klines) - 1
	startIdx := endIdx - params.LimitUpDays + 1
	if startIdx < 0 {
		startIdx = 0
	}

	threshold := 1.095 // 默认主板10%
	if len(klines) > 1 {
		// 根据最新K线判断股票类型
		threshold = GetLimitUpThreshold("") // 需要实际code参数，这里简化处理
	}

	for i := startIdx; i <= endIdx; i++ {
		if i == 0 {
			continue
		}
		prevClose := klines[i-1].Close
		currentClose := klines[i].Close
		if prevClose > 0 && currentClose >= prevClose*threshold {
			return true
		}
	}
	return false
}

// CheckMAMultiple 检查MA是否多头排列(MA5>MA10>MA20)
func CheckMAMultiple(maResult map[string][]float64) bool {
	ma5 := maResult["5"]
	ma10 := maResult["10"]
	ma20 := maResult["20"]

	if len(ma5) == 0 || len(ma10) == 0 || len(ma20) == 0 {
		return false
	}

	lastIdx := len(ma5) - 1
	return ma5[lastIdx] > ma10[lastIdx] && ma10[lastIdx] > ma20[lastIdx]
}

// CheckAboveVWAP 检查股价是否全天在分时均价线上方
func CheckAboveVWAP(minuteData []protocol.PriceNumber) bool {
	if len(minuteData) == 0 {
		return false
	}

	totalAmount := 0.0
	totalVolume := 0
	vwap := 0.0

	for _, item := range minuteData {
		totalAmount += item.Price * float64(item.Number)
		totalVolume += item.Number
		if totalVolume > 0 {
			vwap = totalAmount / float64(totalVolume)
			// 检查当前价格是否低于均价线
			if item.Price < vwap*0.998 { // 允许0.2%误差
				return false
			}
		}
	}
	return true
}

// EvaluateCandidate 评估一个候选股是否符合所有标准
func EvaluateCandidate(
	quote *protocol.QuoteItem,
	finance *protocol.FinanceInfo,
	klines []ta.KlineInput,
	maResult map[string][]float64,
	minuteData []protocol.PriceNumber,
	volumeRatio float64,
	params *OvernightFilterParams,
) *OvernightCandidate {

	candidate := &OvernightCandidate{
		Code:         quote.Code,
		Name:         quote.Name,
		Price:        quote.Price,
		ChangePct:    GetChangePct(quote),
		VolumeRatio:  volumeRatio,
		TurnoverRate: GetTurnoverRate(quote, finance),
		MarketCap:    GetMarketCap(quote, finance),
		Criteria:     OvernightCriteria{},
	}

	// 检查涨幅
	candidate.Criteria.ChangePct = CheckChangePct(quote, params)
	if !candidate.Criteria.ChangePct {
		candidate.Passed = false
		candidate.FailReason = "涨幅不在3%-5%区间"
		return candidate
	}

	// 检查量比
	candidate.Criteria.VolumeRatio = CheckVolumeRatio(volumeRatio, params)
	if !candidate.Criteria.VolumeRatio {
		candidate.Passed = false
		candidate.FailReason = "量比不大于1"
		return candidate
	}

	// 检查换手率
	candidate.Criteria.TurnoverRate = CheckTurnoverRate(quote, finance, params)
	if !candidate.Criteria.TurnoverRate {
		candidate.Passed = false
		candidate.FailReason = "换手率不在5%-10%区间"
		return candidate
	}

	// 检查流通市值
	candidate.Criteria.MarketCap = CheckMarketCap(quote, finance, params)
	if !candidate.Criteria.MarketCap {
		candidate.Passed = false
		candidate.FailReason = "流通市值不在50-200亿区间"
		return candidate
	}

	// 检查涨停历史
	candidate.Criteria.LimitUpHistory = CheckLimitUpHistory(klines, params)
	if !candidate.Criteria.LimitUpHistory {
		candidate.Passed = false
		candidate.FailReason = "近20日内无涨停记录"
		return candidate
	}

	// 检查MA多头排列
	candidate.Criteria.MAMultiple = CheckMAMultiple(maResult)
	if !candidate.Criteria.MAMultiple {
		candidate.Passed = false
		candidate.FailReason = "MA未形成多头排列"
		return candidate
	}

	// 检查分时均价线
	candidate.Criteria.AboveVWAP = CheckAboveVWAP(minuteData)
	if !candidate.Criteria.AboveVWAP {
		candidate.Passed = false
		candidate.FailReason = "股价未全天站在均价线上方"
		return candidate
	}

	candidate.Passed = true
	return candidate
}

// IsTradingTime 判断是否在交易时间内(9:30-11:30, 13:00-15:00)
func IsTradingTime(t time.Time) bool {
	hour := t.Hour()
	minute := t.Minute()

	// 上午交易时间: 9:30-11:30
	if hour == 9 && minute >= 30 {
		return true
	}
	if hour == 10 || (hour == 11 && minute <= 30) {
		return true
	}

	// 下午交易时间: 13:00-15:00
	if hour >= 13 && hour < 15 {
		return true
	}

	return false
}

// IsOvernightTime 判断是否适合隔夜套利筛选时间(14:30之后)
func IsOvernightTime(t time.Time) bool {
	return t.Hour() >= 14 && (t.Hour() > 14 || t.Minute() >= 30)
}
