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

// signalDirection 把信号类型归入买入/卖出方向，与 suggestAction 同一口径：
// 超买/死叉/空头排列是卖出/减仓，其余（金叉/超卖/突破上轨/多头排列）是买入参考。
func signalDirection(sigType string) string {
	switch {
	case strings.Contains(sigType, "超买"),
		strings.Contains(sigType, "死叉"),
		strings.Contains(sigType, "空头排列"):
		return "sell"
	default:
		return "buy"
	}
}

// buildSignalsResponse 序列化信号并为每条附带同日 peers 快照：
// 当日全部信号按买入/卖出方向的计数（含自身）+ 同日其他信号列表。
// 用于「信号触发时同时评估其他信号族当日状态」的同日共振展示。
func buildSignalsResponse(signals []signal.Signal) []gin.H {
	type dateGroup struct {
		buy     int
		sell    int
		indexes []int
	}
	groups := make(map[string]*dateGroup)
	for i, s := range signals {
		date := s.Date.Format("2006-01-02")
		g := groups[date]
		if g == nil {
			g = &dateGroup{}
			groups[date] = g
		}
		g.indexes = append(g.indexes, i)
		if signalDirection(string(s.Type)) == "buy" {
			g.buy++
		} else {
			g.sell++
		}
	}

	result := make([]gin.H, 0, len(signals))
	for i, s := range signals {
		date := s.Date.Format("2006-01-02")
		g := groups[date]
		others := make([]gin.H, 0, max(0, len(g.indexes)-1))
		for _, j := range g.indexes {
			if j == i {
				continue
			}
			others = append(others, gin.H{
				"indicator": signals[j].Indicator,
				"type":      string(signals[j].Type),
			})
		}
		result = append(result, gin.H{
			"Code":      s.Code,
			"Date":      date,
			"Type":      string(s.Type),
			"Indicator": s.Indicator,
			"Details":   s.Details,
			"Strength":  s.Strength,
			"Peers": gin.H{
				"buy_count":  g.buy,
				"sell_count": g.sell,
				"others":     others,
			},
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

			// 昨收：最新 K 线的前一根收盘价，作为标准涨跌幅的基准。
			var prevClose float64
			if len(inputs) >= 2 {
				prevClose = inputs[len(inputs)-2].Close
			}

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
					// 昨收用于计算标准涨跌幅：涨跌幅必须相对昨收而非今开，
					// 跳空高开/低开时两者方向都可能相反。
					"PrevClose": prevClose,
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

	// Build summary：按信号类型汇总触发后 1/5/10/20 日的真实表现。
	// 此前只有 type/count/action，winN/avgN/validN 全部缺失，
	// 前端回测表整列显示 "-"。
	type outcomeStats struct {
		count  int
		valid  [4]int
		win    [4]int
		sumChg [4]float64
	}
	horizons := [4]int{1, 5, 10, 20}
	stats := make(map[string]*outcomeStats)
	statsFor := func(sigType string) *outcomeStats {
		st, ok := stats[sigType]
		if !ok {
			st = &outcomeStats{}
			stats[sigType] = st
		}
		return st
	}

	closeAt := func(idx int) (float64, bool) {
		if idx < 0 || idx >= len(inputs) {
			return 0, false
		}
		c := inputs[idx].Close
		if c <= 0 {
			return 0, false
		}
		return c, true
	}

	// 信号类型 -> 操作建议。超买是回调风险、死叉/空头排列是离场信号，
	// 不能一律标「买入参考」。
	suggestAction := func(sigType string) string {
		switch {
		case strings.Contains(sigType, "超买"),
			strings.Contains(sigType, "死叉"),
			strings.Contains(sigType, "空头排列"):
			return "卖出/减仓参考"
		default:
			return "买入参考"
		}
	}

	// 信号日期 -> K 线下标，用于从触发点向后看 N 日表现。
	indexByDate := make(map[string]int, len(inputs))
	for i, in := range inputs {
		indexByDate[in.Time.Format("2006-01-02")] = i
	}

	outcomeRows := make([]gin.H, 0, len(signals))
	for _, sig := range signals {
		dateStr := sig.Date.Format("2006-01-02")
		triggerIdx, ok := indexByDate[dateStr]
		if !ok {
			continue
		}
		base, ok := closeAt(triggerIdx)
		if !ok {
			continue
		}

		action := suggestAction(string(sig.Type))

		row := gin.H{
			"date":      dateStr,
			"type":      string(sig.Type),
			"indicator": sig.Indicator,
			"details":   sig.Details,
			"action":    action,
			"price":     base,
		}
		st := statsFor(string(sig.Type))
		st.count++

		for h, horizon := range horizons {
			futureIdx := triggerIdx + horizon
			future, ok := closeAt(futureIdx)
			chgKey := fmt.Sprintf("chg%d", horizon)
			if !ok {
				// 样本不足（近端信号还没有 N 日数据），置 null 让前端显示 -
				row[chgKey] = nil
				continue
			}
			chg := (future - base) / base * 100
			row[chgKey] = chg
			st.valid[h]++
			st.sumChg[h] += chg
			if chg > 0 {
				st.win[h]++
			}
		}
		outcomeRows = append(outcomeRows, row)
	}
	analysis["outcomes"] = outcomeRows

	for sigType, st := range stats {
		action := suggestAction(sigType)
		row := gin.H{
			"type":   sigType,
			"count":  st.count,
			"action": action,
		}
		for h, horizon := range horizons {
			row[fmt.Sprintf("valid%d", horizon)] = st.valid[h]
			row[fmt.Sprintf("win%d", horizon)] = func() float64 {
				if st.valid[h] == 0 {
					return 0
				}
				return float64(st.win[h]) / float64(st.valid[h]) * 100
			}()
			row[fmt.Sprintf("avg%d", horizon)] = func() float64 {
				if st.valid[h] == 0 {
					return 0
				}
				return st.sumChg[h] / float64(st.valid[h])
			}()
		}
		analysis["summary"] = append(analysis["summary"].([]gin.H), row)
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

// /api/stock/compare 单次要拉 ≈500 次行情（17 板块 × ≤31 股），裸跑 7~10s。
// 行情是分钟级时效，45s 结果缓存完全够用；命中时直接返回。
const (
	stockCompareTTL = 45 * time.Second
	// 板块成分文件按日更新，成分缓存 10 分钟。
	blockItemsTTL = 10 * time.Minute
	// 概念(gn)/指数(zs) 每类最多参与对比的板块数；行业(fg) 全量参与。
	compareMaxBlocksPerType = 5
)

type stockCompareCacheEntry struct {
	expiresAt time.Time
	payload   gin.H
}

type blockItemsCacheEntry struct {
	expiresAt time.Time
	items     []*protocol.BlockItem
}

func (s *Server) getCachedStockCompare(code string) (gin.H, bool) {
	s.compareMu.Lock()
	defer s.compareMu.Unlock()
	entry, ok := s.compareCache[code]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.payload, true
}

func (s *Server) putCachedStockCompare(code string, payload gin.H) {
	s.compareMu.Lock()
	defer s.compareMu.Unlock()
	if s.compareCache == nil {
		s.compareCache = make(map[string]stockCompareCacheEntry)
	}
	// 顺带清理过期项，避免长期运行后缓存无界增长。
	now := time.Now()
	for k, v := range s.compareCache {
		if now.After(v.expiresAt) {
			delete(s.compareCache, k)
		}
	}
	s.compareCache[code] = stockCompareCacheEntry{expiresAt: now.Add(stockCompareTTL), payload: payload}
}

// fetchBlockCached 返回板块成分并做 10 分钟缓存，成分文件按日更新。
// FetchBlock 失败时不缓存，下一次请求会重试。
func (s *Server) fetchBlockCached(f string) []*protocol.BlockItem {
	s.blockItemsMu.Lock()
	defer s.blockItemsMu.Unlock()
	if entry, ok := s.blockItemsCache[f]; ok && time.Now().Before(entry.expiresAt) {
		return entry.items
	}
	items, err := s.svc.FetchBlock(f)
	if err != nil {
		return nil
	}
	if s.blockItemsCache == nil {
		s.blockItemsCache = make(map[string]blockItemsCacheEntry)
	}
	s.blockItemsCache[f] = blockItemsCacheEntry{expiresAt: time.Now().Add(blockItemsTTL), items: items}
	return items
}

func (s *Server) handleStockCompare(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	// 结果缓存命中：7~10s 的板块对比不必每次重算。
	if payload, ok := s.getCachedStockCompare(code); ok {
		c.JSON(http.StatusOK, payload)
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
	// TDX quotes often carry an empty or garbled name; backfill from the
	// market code list so the compare page can render stock names.
	stockQuote.Name = s.resolveDisplayName(code, stockQuote.Name)
	stockChange := (stockQuote.Price - stockQuote.LastClose) / stockQuote.LastClose * 100

	// Get blocks containing this stock
	files := []string{"block_zs.dat", "block_fg.dat", "block_gn.dat"}
	blockComparisons := make([]gin.H, 0)

	// 按文件类型分别限额：行业(fg) 全量参与，概念(gn)/指数(zs) 各取前 N 个命中板块。
	// 此前统一的 maxBlocks=5 判断放在 file 循环顶部，第一份 block_zs.dat 攒满 5 个
	// 板块后整个循环直接 break，行业/概念板块永远进不了对比结果。
	blocksPerFile := make(map[string]int)

	for _, f := range files {
		items := s.fetchBlockCached(f)
		if len(items) == 0 {
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

			// 行业(fg) 全量，概念/指数按类型限额
			if f != "block_fg.dat" && blocksPerFile[f] >= compareMaxBlocksPerType {
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
						name:   s.resolveDisplayName(stockCode, q.Name),
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
			blocksPerFile[f]++
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

	response := gin.H{
		"code":         code,
		"stock_name":   stockQuote.Name,
		"stock_change": stockChange,
		"comparisons":  blockComparisons,
	}
	s.putCachedStockCompare(code, response)

	c.JSON(http.StatusOK, response)
}

// fetchFinanceAnalysisContent returns the 财务分析 block of a stock's F10 data.
// The block reader verifies its byte boundaries against the document, so a
// catalogue that is older than the daily-refreshed file is repaired instead of
// slicing text out of the middle of a section.
func (s *Server) fetchFinanceAnalysisContent(code string) (string, error) {
	return s.svc.FetchCompanyBlock(code, "财务分析")
}
