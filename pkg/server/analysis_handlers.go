package server

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (s *Server) handleIndicator(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	daysStr := c.Query("days")
	days := 0
	if daysStr != "" {
		days, _ = strconv.Atoi(daysStr)
	}

	// Get klines
	klines, err := withRetry(s, func() ([]*protocol.Kline, error) {
		return s.svc.FetchKlineAll(code, ktype)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线数据失败: %v", err)})
		return
	}

	// Filter out corrupted klines (keep valid data for display)
	klines = tdx.FilterValidKlines(klines)
	if len(klines) == 0 {
		c.JSON(http.StatusOK, gin.H{"error": "该股票暂无可展示的数据", "klines": []gin.H{}})
		return
	}

	// Get quote for name
	quotes, err := withRetry(s, func() ([]*protocol.QuoteItem, error) {
		return s.svc.GetQuote(code)
	})
	name := ""
	if err == nil && len(quotes) > 0 {
		name = quotes[0].Name
	}

	// Build inputs
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

	// Get params
	params := param.Resolve(code, param.DetectCategory(code))

	// Calculate indicators
	result := ta.Calculate(inputs, params)

	// Detect signals
	signals := signal.Detect(code, inputs, result, signal.DefaultDetectOptions())

	// Limit days if specified
	var limitedKlines []gin.H
	startIdx := 0
	if days > 0 && len(inputs) > days {
		startIdx = len(inputs) - days
	}

	for i := startIdx; i < len(inputs); i++ {
		k := inputs[i]
		limitedKlines = append(limitedKlines, gin.H{
			"Time":   formatKlineAPITime(k.Time, ktype),
			"Open":   k.Open,
			"High":   k.High,
			"Low":    k.Low,
			"Close":  k.Close,
			"Volume": k.Volume,
			"Amount": k.Amount,
		})
	}

	// Build response
	response := gin.H{
		"code":   code,
		"name":   name,
		"klines": limitedKlines,
		"ma":     result.MA,
		"macd": gin.H{
			"DIF":  result.MACD.DIF,
			"DEA":  result.MACD.DEA,
			"Hist": result.MACD.Hist,
			"HIST": result.MACD.Hist,
		},
		"kdj": gin.H{
			"K": result.KDJ.K,
			"D": result.KDJ.D,
			"J": result.KDJ.J,
		},
		"boll": gin.H{
			"Upper":  result.BOLL.Upper,
			"Middle": result.BOLL.Middle,
			"Lower":  result.BOLL.Lower,
		},
		"rsi":         result.RSI,
		"volumeRatio": result.VolumeRatio.Ratio,
		"signals":     buildSignalsResponse(signals),
	}

	if len(inputs) > 0 {
		response["last"] = gin.H{
			"Open":   inputs[len(inputs)-1].Open,
			"High":   inputs[len(inputs)-1].High,
			"Low":    inputs[len(inputs)-1].Low,
			"Close":  inputs[len(inputs)-1].Close,
			"Volume": inputs[len(inputs)-1].Volume,
		}
	}

	c.JSON(http.StatusOK, response)
}

func buildSignalsResponse(signals []signal.Signal) []gin.H {
	var result []gin.H
	for _, s := range signals {
		result = append(result, gin.H{
			"Code":      s.Code,
			"Date":      s.Date.Format("2006-01-02"),
			"Type":      string(s.Type),
			"Indicator": s.Indicator,
			"Details":   s.Details,
			"Strength":  s.Strength,
		})
	}
	return result
}

// handleScreen handles batch screening requests
func (s *Server) handleScreen(c *gin.Context) {
	codesStr := c.Query("codes")
	if codesStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "codes is required"})
		return
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	codeNameMap := s.getCodeNameMapServer()

	signalFilters := strings.Split(c.Query("signals"), ",")
	var signalFilterSet map[string]bool
	if c.Query("signals") != "" {
		signalFilterSet = make(map[string]bool)
		for _, s := range signalFilters {
			signalFilterSet[strings.TrimSpace(s)] = true
		}
	}

	// Parse, trim, deduplicate, and cap batch size
	const maxCodes = 500
	seen := make(map[string]bool)
	var codes []string
	capped := false
	for _, raw := range strings.Split(codesStr, ",") {
		code := strings.TrimSpace(raw)
		if code == "" || seen[code] {
			continue
		}
		if len(codes) >= maxCodes {
			capped = true
			break
		}
		seen[code] = true
		codes = append(codes, code)
	}

	// Track per-code status for transparent reporting
	type codeStatus struct {
		Code   string `json:"code"`
		Name   string `json:"name,omitempty"`
		Status string `json:"status"` // "failed" or "skipped"
		Reason string `json:"reason"`
	}
	type screenOutput struct {
		result  *gin.H
		failed  *codeStatus
		skipped *codeStatus
	}

	// Bounded concurrent processing
	const concurrency = 8
	sem := make(chan struct{}, concurrency)
	outputs := make([]screenOutput, len(codes))
	var wg sync.WaitGroup

	for i, code := range codes {
		wg.Add(1)
		go func(idx int, code string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			out := screenOutput{}

			// Get klines
			klines, err := withRetry(s, func() ([]*protocol.Kline, error) {
				return s.svc.FetchKlineAll(code, ktype)
			})
			if err != nil {
				out.failed = &codeStatus{Code: code, Status: "failed", Reason: fmt.Sprintf("获取K线失败: %v", err)}
				outputs[idx] = out
				return
			}

			// Validate klines
			if corrupted := findCorruptedKlines(klines); len(corrupted) > 0 {
				out.failed = &codeStatus{Code: code, Status: "failed", Reason: fmt.Sprintf("检测到 %d 条异常K线数据", len(corrupted))}
				outputs[idx] = out
				return
			}

			// Get name from code-name map first, fallback to quote
			name := codeNameMap[code]
			if name == "" {
				quotes, err := withRetry(s, func() ([]*protocol.QuoteItem, error) {
					return s.svc.GetQuote(code)
				})
				if err == nil && len(quotes) > 0 {
					name = quotes[0].Name
				}
			}

			// Build inputs
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

			if len(inputs) == 0 {
				out.failed = &codeStatus{Code: code, Name: name, Status: "failed", Reason: "无K线数据"}
				outputs[idx] = out
				return
			}

			// Get params
			params := param.Resolve(code, param.DetectCategory(code))

			// Calculate indicators
			result := ta.Calculate(inputs, params)

			// Detect signals
			allSignals := signal.Detect(code, inputs, result, signal.DefaultDetectOptions())

			// Filter to only signals from the latest K-line (today)
			var signals []signal.Signal
			if len(inputs) > 0 {
				latestTime := inputs[len(inputs)-1].Time
				for _, s := range allSignals {
					if s.Date.Equal(latestTime) {
						signals = append(signals, s)
					}
				}
			}

			// Detect cycles
			cycles := signal.DetectAllCycles(code, inputs, result)

			// Filter by signals if specified (match any)
			if signalFilterSet != nil && len(signalFilterSet) > 0 {
				hasSignal := false
				for _, s := range signals {
					if signalFilterSet[string(s.Type)] {
						hasSignal = true
						break
					}
				}
				if !hasSignal {
					out.skipped = &codeStatus{Code: code, Name: name, Status: "skipped", Reason: "未命中指定信号"}
					outputs[idx] = out
					return
				}
			}

			out.result = &gin.H{
				"code":    code,
				"name":    name,
				"signals": buildSignalsResponse(signals),
				"cycles":  cycles,
				"ma":      result.MA,
				"macd": gin.H{
					"DIF":  result.MACD.DIF,
					"DEA":  result.MACD.DEA,
					"Hist": result.MACD.Hist,
					"HIST": result.MACD.Hist,
				},
				"kdj": gin.H{
					"K": result.KDJ.K,
					"D": result.KDJ.D,
					"J": result.KDJ.J,
				},
				"last": gin.H{
					"Open":   inputs[len(inputs)-1].Open,
					"High":   inputs[len(inputs)-1].High,
					"Low":    inputs[len(inputs)-1].Low,
					"Close":  inputs[len(inputs)-1].Close,
					"Volume": inputs[len(inputs)-1].Volume,
				},
			}
			outputs[idx] = out
		}(i, code)
	}
	wg.Wait()

	// Assemble results from concurrent outputs
	results := make([]gin.H, 0, len(codes))
	var failed []codeStatus
	var skipped []codeStatus
	for _, out := range outputs {
		if out.result != nil {
			results = append(results, *out.result)
		} else if out.failed != nil {
			failed = append(failed, *out.failed)
		} else if out.skipped != nil {
			skipped = append(skipped, *out.skipped)
		}
	}

	response := gin.H{
		"total":        len(codes),
		"successCount": len(results),
		"failedCount":  len(failed),
		"skippedCount": len(skipped),
		"results":      results,
		"failed":       failed,
		"skipped":      skipped,
	}
	if capped {
		response["capped"] = true
		response["maxCodes"] = maxCodes
		response["reason"] = fmt.Sprintf("批量上限 %d 只，已截断", maxCodes)
	}

	c.JSON(http.StatusOK, response)
}

// handleSignalAnalysis handles signal analysis requests
func (s *Server) handleSignalAnalysis(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	// Get klines
	klines, err := s.svc.FetchKlineAll(code, ktype)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build inputs
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

	// Get params
	params := param.Resolve(code, param.DetectCategory(code))

	// Calculate indicators
	result := ta.Calculate(inputs, params)

	// Detect signals
	signals := signal.Detect(code, inputs, result, signal.DefaultDetectOptions())

	// Detect trend
	trend := signal.TrendUnknown
	if result.MA != nil {
		trend = signal.DetectTrend(inputs, result.MA)
	}

	// Analyze cycles
	cycles := signal.DetectAllCycles(code, inputs, result)

	// Generate interpretations for each signal
	var interpretations []gin.H
	for _, s := range signals {
		interpretation := signal.InterpretSignal(s, trend)
		interpretations = append(interpretations, gin.H{
			"signal": gin.H{
				"type":      string(s.Type),
				"indicator": s.Indicator,
				"date":      s.Date,
				"strength":  s.Strength,
				"details":   s.Details,
			},
			"interpretation": gin.H{
				"summary":     interpretation.Summary,
				"explanation": interpretation.Explanation,
				"suggestions": interpretation.Suggestions,
				"risk_level":  interpretation.RiskLevel,
				"trend":       interpretation.Trend,
			},
		})
	}

	// Generate overall summary
	overallSummary := signal.InterpretAllSignals(signals, trend)

	// Build analysis
	analysis := gin.H{
		"code":            code,
		"count":           len(inputs),
		"signals":         len(signals),
		"overall_summary": overallSummary,
		"trend":           signal.TrendToString(trend),
		"interpretations": interpretations,
		"summary":         []gin.H{},
		"outcomes":        []gin.H{},
	}

	// Build summary
	signalCounts := make(map[string]int)
	for _, s := range signals {
		signalCounts[string(s.Type)]++
	}

	for sigType, count := range signalCounts {
		action := "买入参考"
		if strings.Contains(sigType, "死叉") || strings.Contains(sigType, "空头") {
			action = "卖出参考"
		}
		analysis["summary"] = append(analysis["summary"].([]gin.H), gin.H{
			"type":   sigType,
			"count":  count,
			"action": action,
		})
	}

	// Build outcomes from cycles
	for _, cycle := range cycles {
		analysis["outcomes"] = append(analysis["outcomes"].([]gin.H), gin.H{
			"date":      cycle.BuyDate,
			"type":      cycle.BuySignal,
			"indicator": cycle.BuySignal,
			"action":    "买入参考",
			"price":     cycle.BuyPrice,
		})
	}

	c.JSON(http.StatusOK, analysis)
}

// handleStockSearch handles stock search requests
func (s *Server) handleStockSearch(c *gin.Context) {
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		query = strings.TrimSpace(c.Query("q"))
	}
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 query 参数"})
		return
	}

	limit := stockSearchDefaultLimit
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > stockSearchMaxLimit {
		limit = stockSearchMaxLimit
	}

	matches, resolved, exact, err := s.searchStockMatches(query, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stockSearchResponse{Query: query, Total: len(matches), Exact: exact, Resolved: resolved, Matches: matches})
}

// handleStockSearchIndex handles search index requests
func (s *Server) handleStockSearchIndex(c *gin.Context) {
	items, err := s.getStockSearchIndex()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	entries := make([]stockSearchIndexEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, stockSearchIndexEntry{
			Code:     item.Code,
			Name:     item.Name,
			Exchange: item.Exchange,
			NameNorm: item.NameNorm,
			Pinyin:   item.PinyinNorm,
			Initials: item.Initials,
		})
	}
	s.stockSearchIndexCache.RLock()
	updatedAt := s.stockSearchIndexCache.builtAt.UnixMilli()
	s.stockSearchIndexCache.RUnlock()
	c.JSON(http.StatusOK, stockSearchIndexResponse{UpdatedAt: updatedAt, Total: len(entries), Items: entries})
}

// handleHistoryList handles history list requests

func (s *Server) handleStockCompare(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	// Set a deadline for the entire compare operation
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Get stock quote
	quotes, err := s.svc.GetQuote(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(quotes) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "quote not found"})
		return
	}
	stockQuote := quotes[0]
	stockChange := (stockQuote.Price - stockQuote.LastClose) / stockQuote.LastClose * 100

	// Get blocks containing this stock
	files := []string{"block_zs.dat", "block_fg.dat", "block_gn.dat"}
	blockComparisons := make([]gin.H, 0)

	// Limit total blocks to compare to avoid excessive requests
	const maxBlocks = 5
	blocksCompared := 0

	for _, f := range files {
		if blocksCompared >= maxBlocks {
			break
		}
		items, err := s.svc.FetchBlock(f)
		if err != nil {
			continue
		}

		// Find blocks containing this stock
		blockStocksMap := make(map[string][]string)
		blockTypeMap := make(map[string]uint16)

		for _, item := range items {
			blockStocksMap[item.BlockName] = append(blockStocksMap[item.BlockName], item.StockCode)
			blockTypeMap[item.BlockName] = item.BlockType
		}

		// Find blocks that contain this stock
		for blockName, stockCodes := range blockStocksMap {
			found := false
			for _, sc := range stockCodes {
				if sc == code {
					found = true
					break
				}
			}
			if !found {
				continue
			}

			// Cap the number of stocks to compare per block for performance
			const maxBlockStocks = 30
			compareStocks := stockCodes
			cappedBlock := false
			if len(compareStocks) > maxBlockStocks {
				// Ensure the target stock is always in the comparison set
				compareStocks = make([]string, 0, maxBlockStocks+1)
				targetIncluded := false
				for i, sc := range stockCodes {
					if i >= maxBlockStocks && sc != code {
						continue
					}
					if sc == code {
						targetIncluded = true
					}
					compareStocks = append(compareStocks, sc)
				}
				if !targetIncluded {
					compareStocks = append(compareStocks, code)
				}
				cappedBlock = true
			}

			// Bounded concurrent quote fetching with timeout
			const quoteConcurrency = 8
			type quoteResult struct {
				code   string
				name   string
				price  float64
				change float64
				ok     bool
			}
			quoteResults := make([]quoteResult, len(compareStocks))
			var qwg sync.WaitGroup
			qsem := make(chan struct{}, quoteConcurrency)

			for i, sc := range compareStocks {
				// Check if we still have time
				if ctx.Err() != nil {
					break
				}
				qwg.Add(1)
				go func(idx int, stockCode string) {
					defer qwg.Done()
					qsem <- struct{}{}
					defer func() { <-qsem }()

					qs, err := s.svc.GetQuote(stockCode)
					if err != nil || len(qs) == 0 {
						quoteResults[idx] = quoteResult{code: stockCode, ok: false}
						return
					}
					q := qs[0]
					change := (q.Price - q.LastClose) / q.LastClose * 100
					quoteResults[idx] = quoteResult{
						code:   stockCode,
						name:   q.Name,
						price:  q.Price,
						change: change,
						ok:     true,
					}
				}(i, sc)
			}
			qwg.Wait()

			// Collect results
			var blockQuotes []gin.H
			var totalChange float64
			var validCount int
			var upCount int
			var downCount int

			for _, qr := range quoteResults {
				if !qr.ok {
					continue
				}
				totalChange += qr.change
				validCount++
				if qr.change > 0 {
					upCount++
				} else if qr.change < 0 {
					downCount++
				}

				blockQuotes = append(blockQuotes, gin.H{
					"code":   qr.code,
					"name":   qr.name,
					"price":  qr.price,
					"change": qr.change,
				})
			}

			if validCount == 0 {
				continue
			}

			// Sort by change (descending)
			sort.Slice(blockQuotes, func(i, j int) bool {
				return blockQuotes[i]["change"].(float64) > blockQuotes[j]["change"].(float64)
			})

			// Find stock rank in block
			rank := 0
			for i, bq := range blockQuotes {
				if bq["code"].(string) == code {
					rank = i + 1
					break
				}
			}

			avgChange := totalChange / float64(validCount)

			blockComparisons = append(blockComparisons, gin.H{
				"block_name":   blockName,
				"block_type":   blockTypeMap[blockName],
				"block_file":   f,
				"total_stocks": len(stockCodes),
				"valid_stocks": validCount,
				"up_count":     upCount,
				"down_count":   downCount,
				"avg_change":   avgChange,
				"stock_rank":   rank,
				"stock_change": stockChange,
				"capped":       cappedBlock,
				"stock_quote": gin.H{
					"code":       stockQuote.Code,
					"name":       stockQuote.Name,
					"price":      stockQuote.Price,
					"change":     stockChange,
					"last_close": stockQuote.LastClose,
				},
				"top_stocks":    blockQuotes[:min(5, len(blockQuotes))],
				"bottom_stocks": blockQuotes[max(0, len(blockQuotes)-5):],
			})
			blocksCompared++
		}

		// Check timeout
		if ctx.Err() != nil {
			break
		}
	}

	// Sort by stock rank (ascending)
	sort.Slice(blockComparisons, func(i, j int) bool {
		return blockComparisons[i]["stock_rank"].(int) < blockComparisons[j]["stock_rank"].(int)
	})

	c.JSON(http.StatusOK, gin.H{
		"code":         code,
		"stock_name":   stockQuote.Name,
		"stock_change": stockChange,
		"comparisons":  blockComparisons,
	})
}

// fetchCompanyBlockContent fetches content from a specific block in the company's F10 data
func (s *Server) fetchCompanyBlockContent(code, block string) (string, error) {
	cats, err := withRetry(s, func() ([]*protocol.CompanyCategoryItem, error) {
		return s.svc.FetchCompanyCategory(code)
	})
	if err != nil {
		return "", err
	}

	return s.fetchCompanyBlockContentFromCategories(code, block, cats)
}

// fetchFinanceAnalysisContent retries once with fresh category metadata when
// cached byte offsets point into the middle of the latest F10 file.
func (s *Server) fetchFinanceAnalysisContent(code string) (string, error) {
	const block = "财务分析"

	content, err := s.fetchCompanyBlockContent(code, block)
	if err != nil || hasMainFinanceMetricSection(content) {
		return content, err
	}

	cats, err := withRetry(s, func() ([]*protocol.CompanyCategoryItem, error) {
		return s.svc.RefreshCompanyCategory(code)
	})
	if err != nil {
		return "", err
	}
	return s.fetchCompanyBlockContentFromCategories(code, block, cats)
}

func hasMainFinanceMetricSection(content string) bool {
	return strings.Contains(content, "【1.主要财务指标】")
}

func (s *Server) fetchCompanyBlockContentFromCategories(code, block string, cats []*protocol.CompanyCategoryItem) (string, error) {
	for _, cat := range cats {
		if cat.Name != block {
			continue
		}
		content, err := withRetry(s, func() (string, error) {
			return s.svc.FetchCompanyContent(code, cat.Filename, cat.Start, cat.Length)
		})
		return content, err
	}
	return "", fmt.Errorf("未找到块: %s", block)
}
