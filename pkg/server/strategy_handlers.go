package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/strategy"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"log"
	"net/http"
	"sort"
	"time"
)

func (s *Server) handleOvernightArbitrage(c *gin.Context) {
	var req struct {
		Codes        []string `json:"codes"`
		MinMarketCap float64  `json:"minMarketCap"`
		MaxMarketCap float64  `json:"maxMarketCap"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Normalize codes
	codes := normalizeCodeList(req.Codes)
	if len(codes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "codes is required"})
		return
	}

	params := strategy.DefaultOvernightParams()
	// Override with custom market cap range if provided
	if req.MinMarketCap > 0 {
		params.MinMarketCap = req.MinMarketCap
	}
	if req.MaxMarketCap > 0 {
		params.MaxMarketCap = req.MaxMarketCap
	}

	// Stage 1: Batch fetch quotes and filter by change percentage (3%-5%)
	// This is the cheapest filter that eliminates ~90% of candidates
	type quoteResult struct {
		code  string
		quote *protocol.QuoteItem
		err   error
	}
	quoteChan := make(chan quoteResult, len(codes))
	sem := make(chan struct{}, 16)

	for _, code := range codes {
		go func(code string) {
			sem <- struct{}{}
			defer func() { <-sem }()

			quotes, err := withRetry(s, func() ([]*protocol.QuoteItem, error) {
				return s.svc.GetQuote(code)
			})
			if err != nil {
				quoteChan <- quoteResult{code: code, err: err}
				return
			}
			if len(quotes) == 0 {
				quoteChan <- quoteResult{code: code, err: fmt.Errorf("no quote")}
				return
			}
			quoteChan <- quoteResult{code: code, quote: quotes[0]}
		}(code)
	}

	var stage1Passed []*protocol.QuoteItem
	var stage1Failed []gin.H
	for i := 0; i < len(codes); i++ {
		result := <-quoteChan
		if result.err != nil {
			stage1Failed = append(stage1Failed, gin.H{"code": result.code, "reason": result.err.Error()})
			continue
		}
		// Filter by change percentage
		if !strategy.CheckChangePct(result.quote, params) {
			stage1Failed = append(stage1Failed, gin.H{"code": result.code, "reason": fmt.Sprintf("涨幅%.2f%%不在3%%-5%%区间", strategy.GetChangePct(result.quote))})
			continue
		}
		stage1Passed = append(stage1Passed, result.quote)
	}

	log.Printf("[strategy/overnight] Stage 1: %d passed, %d failed of %d total", len(stage1Passed), len(stage1Failed), len(codes))

	if len(stage1Passed) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"total":             len(codes),
			"stage1_passed":     0,
			"stage1_failed":     len(stage1Failed),
			"stage2_passed":     0,
			"stage3_passed":     0,
			"stage4_passed":     0,
			"final_candidates":  []*strategy.OvernightCandidate{},
			"failed":            stage1Failed,
			"current_time":      time.Now().Format("15:04"),
			"is_overnight_time": strategy.IsOvernightTime(time.Now()),
		})
		return
	}

	// Stage 2: Fetch finance data and filter by market cap (50-200亿)
	type financeResult struct {
		quote   *protocol.QuoteItem
		finance *protocol.FinanceInfo
		err     error
	}
	financeChan := make(chan financeResult, len(stage1Passed))

	for _, quote := range stage1Passed {
		go func(quote *protocol.QuoteItem) {
			sem <- struct{}{}
			defer func() { <-sem }()

			finance, err := withRetry(s, func() (*protocol.FinanceInfo, error) {
				return s.svc.FetchFinance(quote.Code)
			})
			if err != nil {
				financeChan <- financeResult{quote: quote, err: err}
				return
			}
			financeChan <- financeResult{quote: quote, finance: finance}
		}(quote)
	}

	var stage2Passed []struct {
		quote   *protocol.QuoteItem
		finance *protocol.FinanceInfo
	}
	var stage2Failed []gin.H
	for i := 0; i < len(stage1Passed); i++ {
		result := <-financeChan
		if result.err != nil {
			stage2Failed = append(stage2Failed, gin.H{"code": result.quote.Code, "reason": fmt.Sprintf("获取财务数据失败: %v", result.err)})
			continue
		}
		// Filter by market cap and turnover rate
		if !strategy.CheckMarketCap(result.quote, result.finance, params) {
			stage2Failed = append(stage2Failed, gin.H{"code": result.quote.Code, "reason": fmt.Sprintf("流通市值%.2f亿不在50-200亿区间", strategy.GetMarketCap(result.quote, result.finance))})
			continue
		}
		if !strategy.CheckTurnoverRate(result.quote, result.finance, params) {
			stage2Failed = append(stage2Failed, gin.H{"code": result.quote.Code, "reason": fmt.Sprintf("换手率%.2f%%不在5%%-10%%区间", strategy.GetTurnoverRate(result.quote, result.finance))})
			continue
		}
		stage2Passed = append(stage2Passed, struct {
			quote   *protocol.QuoteItem
			finance *protocol.FinanceInfo
		}{result.quote, result.finance})
	}

	log.Printf("[strategy/overnight] Stage 2: %d passed, %d failed", len(stage2Passed), len(stage2Failed))

	if len(stage2Passed) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"total":             len(codes),
			"stage1_passed":     len(stage1Passed),
			"stage1_failed":     len(stage1Failed),
			"stage2_passed":     0,
			"stage3_passed":     0,
			"stage4_passed":     0,
			"final_candidates":  []*strategy.OvernightCandidate{},
			"failed":            append(stage1Failed, stage2Failed...),
			"current_time":      time.Now().Format("15:04"),
			"is_overnight_time": strategy.IsOvernightTime(time.Now()),
		})
		return
	}

	// Stage 3: Fetch daily klines and filter by limit-up history and MA multiple
	type klineResult struct {
		quote       *protocol.QuoteItem
		finance     *protocol.FinanceInfo
		klines      []ta.KlineInput
		maResult    map[string][]float64
		volumeRatio float64
		err         error
	}
	klineChan := make(chan klineResult, len(stage2Passed))

	for _, item := range stage2Passed {
		go func(item struct {
			quote   *protocol.QuoteItem
			finance *protocol.FinanceInfo
		}) {
			sem <- struct{}{}
			defer func() { <-sem }()

			klines, err := withRetry(s, func() ([]*protocol.Kline, error) {
				return s.svc.FetchKlineAll(item.quote.Code, tdx.ParseKlineType("day"))
			})
			if err != nil {
				klineChan <- klineResult{quote: item.quote, finance: item.finance, err: err}
				return
			}

			klines = tdx.FilterValidKlines(klines)
			if len(klines) < 20 {
				klineChan <- klineResult{quote: item.quote, finance: item.finance, err: fmt.Errorf("K线数据不足")}
				return
			}

			var inputs []ta.KlineInput
			for _, k := range klines {
				inputs = append(inputs, ta.KlineInput{
					Time:   k.Time,
					Open:   k.Open,
					High:   k.High,
					Low:    k.Low,
					Close:  k.Close,
					Volume: k.Volume,
					Amount: k.Amount,
				})
			}

			// Calculate indicators
			taParams := param.Resolve(item.quote.Code, param.DetectCategory(item.quote.Code))
			taResult := ta.Calculate(inputs, taParams)

			klineChan <- klineResult{
				quote:       item.quote,
				finance:     item.finance,
				klines:      inputs,
				maResult:    taResult.MA,
				volumeRatio: taResult.VolumeRatio.Ratio,
			}
		}(item)
	}

	var stage3Passed []struct {
		quote       *protocol.QuoteItem
		finance     *protocol.FinanceInfo
		klines      []ta.KlineInput
		maResult    map[string][]float64
		volumeRatio float64
	}
	var stage3Failed []gin.H
	for i := 0; i < len(stage2Passed); i++ {
		result := <-klineChan
		if result.err != nil {
			stage3Failed = append(stage3Failed, gin.H{"code": result.quote.Code, "reason": fmt.Sprintf("获取K线失败: %v", result.err)})
			continue
		}
		// Filter by volume ratio
		if !strategy.CheckVolumeRatio(result.volumeRatio, params) {
			stage3Failed = append(stage3Failed, gin.H{"code": result.quote.Code, "reason": fmt.Sprintf("量比%.2f不大于1", result.volumeRatio)})
			continue
		}
		// Filter by limit-up history
		if !strategy.CheckLimitUpHistory(result.klines, params) {
			stage3Failed = append(stage3Failed, gin.H{"code": result.quote.Code, "reason": "近20日内无涨停记录"})
			continue
		}
		// Filter by MA multiple
		if !strategy.CheckMAMultiple(result.maResult) {
			stage3Failed = append(stage3Failed, gin.H{"code": result.quote.Code, "reason": "MA未形成多头排列"})
			continue
		}
		stage3Passed = append(stage3Passed, struct {
			quote       *protocol.QuoteItem
			finance     *protocol.FinanceInfo
			klines      []ta.KlineInput
			maResult    map[string][]float64
			volumeRatio float64
		}{result.quote, result.finance, result.klines, result.maResult, result.volumeRatio})
	}

	log.Printf("[strategy/overnight] Stage 3: %d passed, %d failed", len(stage3Passed), len(stage3Failed))

	if len(stage3Passed) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"total":             len(codes),
			"stage1_passed":     len(stage1Passed),
			"stage1_failed":     len(stage1Failed),
			"stage2_passed":     len(stage2Passed),
			"stage2_failed":     len(stage2Failed),
			"stage3_passed":     0,
			"stage4_passed":     0,
			"final_candidates":  []*strategy.OvernightCandidate{},
			"failed":            append(append(stage1Failed, stage2Failed...), stage3Failed...),
			"current_time":      time.Now().Format("15:04"),
			"is_overnight_time": strategy.IsOvernightTime(time.Now()),
		})
		return
	}

	// Stage 4: Fetch minute data and filter by VWAP
	type minuteResult struct {
		quote       *protocol.QuoteItem
		finance     *protocol.FinanceInfo
		klines      []ta.KlineInput
		maResult    map[string][]float64
		volumeRatio float64
		minuteData  []protocol.PriceNumber
		err         error
	}
	minuteChan := make(chan minuteResult, len(stage3Passed))

	for _, item := range stage3Passed {
		go func(item struct {
			quote       *protocol.QuoteItem
			finance     *protocol.FinanceInfo
			klines      []ta.KlineInput
			maResult    map[string][]float64
			volumeRatio float64
		}) {
			sem <- struct{}{}
			defer func() { <-sem }()

			minute, err := withRetry(s, func() (*protocol.MinuteResp, error) {
				return s.svc.GetMinute(item.quote.Code)
			})
			if err != nil {
				minuteChan <- minuteResult{quote: item.quote, finance: item.finance, err: err}
				return
			}
			minuteChan <- minuteResult{
				quote:       item.quote,
				finance:     item.finance,
				klines:      item.klines,
				maResult:    item.maResult,
				volumeRatio: item.volumeRatio,
				minuteData:  minute.List,
			}
		}(item)
	}

	var finalCandidates []*strategy.OvernightCandidate
	var stage4Failed []gin.H
	for i := 0; i < len(stage3Passed); i++ {
		result := <-minuteChan
		if result.err != nil {
			stage4Failed = append(stage4Failed, gin.H{"code": result.quote.Code, "reason": fmt.Sprintf("获取分时数据失败: %v", result.err)})
			continue
		}
		// Filter by VWAP
		if !strategy.CheckAboveVWAP(result.minuteData) {
			stage4Failed = append(stage4Failed, gin.H{"code": result.quote.Code, "reason": "股价未全天站在均价线上方"})
			continue
		}
		// Evaluate and add to final candidates
		candidate := strategy.EvaluateCandidate(
			result.quote,
			result.finance,
			result.klines,
			result.maResult,
			result.minuteData,
			result.volumeRatio,
			params,
		)
		if candidate.Passed {
			finalCandidates = append(finalCandidates, candidate)
		}
	}

	log.Printf("[strategy/overnight] Stage 4: %d passed, %d failed. Final candidates: %d", len(finalCandidates), len(stage4Failed), len(finalCandidates))

	// Sort by change percentage descending
	sort.Slice(finalCandidates, func(i, j int) bool {
		return finalCandidates[i].ChangePct > finalCandidates[j].ChangePct
	})

	allFailed := append(append(append(stage1Failed, stage2Failed...), stage3Failed...), stage4Failed...)

	c.JSON(http.StatusOK, gin.H{
		"total":             len(codes),
		"stage1_passed":     len(stage1Passed),
		"stage1_failed":     len(stage1Failed),
		"stage2_passed":     len(stage2Passed),
		"stage2_failed":     len(stage2Failed),
		"stage3_passed":     len(stage3Passed),
		"stage3_failed":     len(stage3Failed),
		"stage4_passed":     len(finalCandidates),
		"stage4_failed":     len(stage4Failed),
		"final_candidates":  finalCandidates,
		"failed":            allFailed,
		"current_time":      time.Now().Format("15:04"),
		"is_overnight_time": strategy.IsOvernightTime(time.Now()),
	})
}

// === Stockinfo Handlers ===

// handleStockinfoList returns stock info list with optional filters
