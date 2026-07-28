package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleQuote(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	result, err := s.stockData.Query(c.Request.Context(), stockdata.DataRequest{
		Spec: stockdata.DataSpec{Type: stockdata.DataQuote, Market: marketForCode(code), Code: code},
		Mode: consistencyMode(c), ForceRefresh: forceRefreshRequested(c),
	})
	if err != nil {
		writeStockDataError(c, err)
		return
	}
	if result.Quote == nil {
		WriteError(c, http.StatusNotFound, "not_found", "未找到该股票")
		return
	}
	result.Quote.Name = s.resolveDisplayName(code, result.Quote.Name)
	setFreshnessHeaders(c, result.Metadata)
	c.JSON(http.StatusOK, result.Quote)
}

// handleQuotes handles batch quote requests
func (s *Server) handleQuotes(c *gin.Context) {
	codesStr := c.Query("codes")
	if codesStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "codes is required"})
		return
	}

	codes := strings.Split(codesStr, ",")
	for i := range codes {
		codes[i] = strings.TrimSpace(codes[i])
	}

	var results []*protocol.QuoteItem
	for _, code := range codes {
		if code == "" {
			continue
		}

		result, err := s.stockData.Query(c.Request.Context(), stockdata.DataRequest{
			Spec: stockdata.DataSpec{Type: stockdata.DataQuote, Market: marketForCode(code), Code: code},
			Mode: consistencyMode(c), ForceRefresh: forceRefreshRequested(c),
		})
		if err != nil {
			continue
		}
		if result.Quote != nil {
			result.Quote.Name = s.resolveDisplayName(code, result.Quote.Name)
			results = append(results, result.Quote)
		}
	}

	c.JSON(http.StatusOK, results)
}

// handleCodes handles legacy codes requests
func (s *Server) handleCodes(c *gin.Context) {
	exchange := c.Query("exchange")
	if exchange == "" {
		exchange = "sz"
	}

	var ex protocol.Exchange
	switch exchange {
	case "sh":
		ex = protocol.ExchangeSH
	case "bj":
		ex = protocol.ExchangeBJ
	default:
		ex = protocol.ExchangeSZ
	}

	codes, err := s.svc.FetchCodes(ex)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return simplified list
	var items []gin.H
	for _, item := range codes {
		items = append(items, gin.H{
			"Code": item.Code,
			"Name": item.Name,
		})
	}
	c.JSON(http.StatusOK, items)
}

// handleCodesList handles structured codes list requests
func (s *Server) handleCodesList(c *gin.Context) {
	exchange := c.Query("exchange")
	if exchange == "" {
		exchange = "sz"
	}

	var ex protocol.Exchange
	switch exchange {
	case "sh":
		ex = protocol.ExchangeSH
	case "bj":
		ex = protocol.ExchangeBJ
	default:
		ex = protocol.ExchangeSZ
	}

	codes, err := s.svc.FetchCodes(ex)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var filtered []gin.H
	for _, item := range codes {
		filtered = append(filtered, gin.H{
			"code":     item.Code,
			"name":     item.Name,
			"exchange": exchange,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"exchange": exchange,
		"total":    len(filtered),
		"codes":    filtered,
	})
}

// handleCodesMarket handles market-wide stock codes with deduplication
func (s *Server) handleCodesMarket(c *gin.Context) {
	exchanges := []struct {
		name string
		ex   protocol.Exchange
	}{
		{"sz", protocol.ExchangeSZ},
		{"sh", protocol.ExchangeSH},
		{"bj", protocol.ExchangeBJ},
	}

	// Use map for deduplication by code
	codesMap := make(map[string]gin.H)

	for _, item := range exchanges {
		codes, err := s.svc.FetchCodes(item.ex)
		if err != nil {
			continue
		}
		for _, code := range codes {
			// Filter: only valid A-share stock codes
			if !isStockCode(code.Code, item.name) {
				continue
			}

			if _, exists := codesMap[code.Code]; !exists {
				codesMap[code.Code] = gin.H{
					"code":     code.Code,
					"name":     code.Name,
					"exchange": item.name,
				}
			}
		}
	}

	// Convert map to slice
	var result []gin.H
	for _, v := range codesMap {
		result = append(result, v)
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(result),
		"codes": result,
	})
}

// handleCodesWithMarketCap returns stock codes with market cap information
func (s *Server) handleCodesWithMarketCap(c *gin.Context) {
	var minMarketCap, maxMarketCap float64
	if minStr := c.Query("minMarketCap"); minStr != "" {
		fmt.Sscanf(minStr, "%f", &minMarketCap)
	}
	if maxStr := c.Query("maxMarketCap"); maxStr != "" {
		fmt.Sscanf(maxStr, "%f", &maxMarketCap)
	}

	// First, try to get data from stockinfo store (faster)
	if s.stockinfoDB != nil {
		infos, err := s.stockinfoDB.GetByMarketCap(minMarketCap, maxMarketCap)
		if err == nil && len(infos) > 0 {
			var result []gin.H
			for _, info := range infos {
				result = append(result, gin.H{
					"code":      info.Code,
					"name":      info.Name,
					"exchange":  info.Exchange,
					"marketCap": info.MarketCap,
					"price":     info.Price,
				})
			}
			c.JSON(http.StatusOK, gin.H{
				"total": len(result),
				"codes": result,
			})
			return
		}
	}

	// Fallback to TDX if stockinfo store not available or empty
	exchanges := []struct {
		name string
		ex   protocol.Exchange
	}{
		{"sz", protocol.ExchangeSZ},
		{"sh", protocol.ExchangeSH},
		{"bj", protocol.ExchangeBJ},
	}

	codesMap := make(map[string]gin.H)
	processed := 0
	maxProcess := 500 // Limit to avoid timeout

	for _, item := range exchanges {
		codes, err := s.svc.FetchCodes(item.ex)
		if err != nil {
			continue
		}
		for _, code := range codes {
			if !isStockCode(code.Code, item.name) {
				continue
			}
			if processed >= maxProcess {
				break
			}

			fullCode := item.name + code.Code
			quotes, _ := s.svc.GetQuote(fullCode)
			finance, _ := s.svc.FetchFinance(fullCode)

			var marketCap float64
			var price float64
			if len(quotes) > 0 && finance != nil && quotes[0].Price > 0 && finance.LiuTongGuBen > 0 {
				price = quotes[0].Price
				// LiuTongGuBen unit is 万股 (10,000 shares)
				// Market cap = LiuTongGuBen(万股) * Price(元) / 10000 = 亿元
				marketCap = finance.LiuTongGuBen * price / 10000 // 流通市值(亿)
			}

			// Apply market cap filter
			// Skip stocks with missing market cap data
			if marketCap <= 0 {
				continue
			}
			if (minMarketCap > 0 && marketCap < minMarketCap) ||
				(maxMarketCap > 0 && marketCap > maxMarketCap) {
				continue
			}

			if _, exists := codesMap[code.Code]; !exists {
				codesMap[code.Code] = gin.H{
					"code":      code.Code,
					"name":      code.Name,
					"exchange":  item.name,
					"marketCap": marketCap,
					"price":     price,
				}
				processed++
			}
		}
		if processed >= maxProcess {
			break
		}
	}

	var result []gin.H
	for _, v := range codesMap {
		result = append(result, v)
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(result),
		"codes": result,
	})
}

// handleCodesStats handles codes statistics
func (s *Server) handleCodesStats(c *gin.Context) {
	exchange := c.Query("exchange")
	all := c.Query("all") == "true"

	exchanges := []string{exchange}
	if all || exchange == "" {
		exchanges = []string{"sz", "sh", "bj"}
	}

	var stats []gin.H
	for _, exStr := range exchanges {
		var ex protocol.Exchange
		switch exStr {
		case "sh":
			ex = protocol.ExchangeSH
		case "bj":
			ex = protocol.ExchangeBJ
		default:
			ex = protocol.ExchangeSZ
		}

		codes, err := s.svc.FetchCodes(ex)
		if err != nil {
			continue
		}

		name := "深圳"
		if exStr == "sh" {
			name = "上海"
		} else if exStr == "bj" {
			name = "北京"
		}

		stats = append(stats, gin.H{
			"exchange":   exStr,
			"name":       name,
			"total":      len(codes),
			"categories": gin.H{},
		})
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// handleKline handles kline requests
func (s *Server) handleKline(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	startStr := c.Query("start")
	countStr := c.Query("count")

	var klines []*protocol.Kline
	var err error

	if startStr != "" && countStr != "" {
		start, _ := strconv.ParseUint(startStr, 10, 16)
		count, _ := strconv.ParseUint(countStr, 10, 16)
		klines, err = withRetry(s, func() ([]*protocol.Kline, error) {
			return s.svc.FetchKline(code, ktype, uint16(start), uint16(count))
		})
	} else if ktype == tdx.ParseKlineType("day") {
		result, queryErr := s.stockData.Query(c.Request.Context(), stockdata.DataRequest{
			Spec: stockdata.DataSpec{
				Type: stockdata.DataKline, Market: marketForCode(code), Code: code,
				Granularity: ktypeStr, KType: ktype,
			},
			Mode: consistencyMode(c), ForceRefresh: forceRefreshRequested(c),
		})
		err = queryErr
		klines = result.Klines
		if err == nil {
			setFreshnessHeaders(c, result.Metadata)
		}
	} else {
		klines, err = s.svc.FetchKlineAll(code, ktype)
	}

	if err != nil {
		writeStockDataError(c, err)
		return
	}

	var items []gin.H
	for _, k := range klines {
		items = append(items, gin.H{
			"Time":   formatKlineAPITime(k.Time, ktype),
			"Open":   k.Open,
			"High":   k.High,
			"Low":    k.Low,
			"Close":  k.Close,
			"Volume": k.Volume,
			"Amount": k.Amount,
		})
	}
	c.JSON(http.StatusOK, items)
}

func formatKlineAPITime(t time.Time, ktype uint8) string {
	if isMinuteKlineType(ktype) {
		return t.Format("2006-01-02 15:04:05")
	}
	return t.Format("2006-01-02")
}

func isMinuteKlineType(ktype uint8) bool {
	switch ktype {
	case 7, 0, 1, 2, 3:
		return true
	default:
		return false
	}
}

// handleIndex handles index kline requests
func (s *Server) handleIndex(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		code = "999999"
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	bars, err := withRetry(s, func() ([]*protocol.IndexBar, error) {
		return s.svc.GetIndexBars(code, ktype, 0, 500)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取指数K线失败: %v", err)})
		return
	}

	var items []gin.H
	for _, bar := range bars {
		items = append(items, gin.H{
			"Time":      bar.Time.Format("2006-01-02"),
			"Open":      bar.Open,
			"High":      bar.High,
			"Low":       bar.Low,
			"Close":     bar.Close,
			"Volume":    bar.Volume,
			"Amount":    bar.Amount,
			"UpCount":   bar.UpCount,
			"DownCount": bar.DownCount,
		})
	}
	c.JSON(http.StatusOK, items)
}

// handleMinute handles minute data requests
func (s *Server) handleMinute(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	history := c.Query("history") == "true"
	date := c.Query("date")

	var result *protocol.MinuteResp
	var err error

	if history && date != "" {
		result, err = withRetry(s, func() (*protocol.MinuteResp, error) {
			return s.svc.GetHistoryMinute(date, code)
		})
	} else {
		result, err = withRetry(s, func() (*protocol.MinuteResp, error) {
			return s.svc.GetMinute(code)
		})
	}

	if err != nil {
		// 指数分时数据可能为空或上游无数据，不应返回500
		// 将"数据长度不足"视为空数据返回200
		if strings.Contains(err.Error(), "数据长度不足") {
			c.JSON(http.StatusOK, gin.H{"List": []interface{}{}, "message": "暂无分时数据（指数可能不支持分时查询）"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取分时数据失败: %v", err)})
		return
	}

	var items []gin.H
	for _, item := range result.List {
		items = append(items, gin.H{
			"Time":   item.Time,
			"Price":  item.Price,
			"Number": item.Number,
		})
	}

	if len(items) == 0 {
		c.JSON(http.StatusOK, gin.H{"List": []interface{}{}, "message": "暂无分时数据（非交易时段或指数不支持）"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"List": items})
}

// handleTrade handles trade data requests
func (s *Server) handleTrade(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	history := c.Query("history") == "true"
	date := c.Query("date")

	var result *protocol.TradeResp
	var err error

	if history && date != "" {
		result, err = withRetry(s, func() (*protocol.TradeResp, error) {
			return s.svc.GetHistoryMinuteTrade(date, code, 0, 2000)
		})
	} else {
		result, err = withRetry(s, func() (*protocol.TradeResp, error) {
			return s.svc.GetMinuteTradeAll(code)
		})
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取成交数据失败: %v", err)})
		return
	}

	var items []gin.H
	for _, item := range result.List {
		items = append(items, gin.H{
			"Time":   item.Time.Format("15:04:05"),
			"Price":  item.Price,
			"Volume": item.Volume,
			"Status": item.Status,
		})
	}
	c.JSON(http.StatusOK, gin.H{"List": items})
}

// handleAuction handles auction data requests
func (s *Server) handleAuction(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	result, err := withRetry(s, func() (*protocol.CallAuctionResp, error) {
		return s.svc.GetCallAuction(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取集合竞价数据失败: %v", err)})
		return
	}

	var items []gin.H
	for _, item := range result.List {
		items = append(items, gin.H{
			"Time":      item.Time.Format("15:04:05"),
			"Price":     item.Price,
			"Match":     item.Match,
			"Unmatched": item.Unmatched,
			"Flag":      item.Flag,
		})
	}
	c.JSON(http.StatusOK, gin.H{"List": items})
}

// handleXdXr handles xdxr data requests
func (s *Server) handleXdXr(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	items, err := withRetry(s, func() ([]*protocol.XdXrItem, error) {
		return s.svc.FetchXdXr(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取除权除息数据失败: %v", err)})
		return
	}

	var result []gin.H
	for _, item := range items {
		result = append(result, gin.H{
			"Date":          item.Date,
			"Category":      item.Category,
			"FenHong":       item.FenHong,
			"SongZhuanGu":   item.SongZhuanGu,
			"PeiGuJia":      item.PeiGuJia,
			"PanHouLiuTong": item.PanHouLiuTong,
			"HouZongGuBen":  item.HouZongGuBen,
		})
	}
	c.JSON(http.StatusOK, result)
}

// handleFinance handles finance data requests
func (s *Server) handleFinance(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	result, err := s.stockData.Query(c.Request.Context(), stockdata.DataRequest{
		Spec: stockdata.DataSpec{Type: stockdata.DataFinance, Market: marketForCode(code), Code: code},
		Mode: consistencyMode(c), ForceRefresh: forceRefreshRequested(c),
	})
	if err != nil {
		writeStockDataError(c, err)
		return
	}
	info := result.Finance
	if info == nil {
		WriteError(c, http.StatusNotFound, "not_found", "未找到财务数据")
		return
	}
	setFreshnessHeaders(c, result.Metadata)

	c.JSON(http.StatusOK, gin.H{
		"ZongGuBen":       info.ZongGuBen,
		"LiuTongGuBen":    info.LiuTongGuBen,
		"ZongZiChan":      info.ZongZiChan,
		"JingZiChan":      info.JingZiChan,
		"ZhuYingShouRu":   info.ZhuYingShouRu,
		"JingLiRun":       info.JingLiRun,
		"MeiGuJingZiChan": info.MeiGuJingZiChan,
		"GuDongRenShu":    info.GuDongRenShu,
	})
}

func consistencyMode(c *gin.Context) stockdata.ConsistencyMode {
	switch strings.ToLower(strings.TrimSpace(c.Query("consistency"))) {
	case "cache_only":
		return stockdata.CacheOnly
	case "allow_stale":
		return stockdata.AllowStale
	default:
		return stockdata.RequireFresh
	}
}

func forceRefreshRequested(c *gin.Context) bool {
	value := strings.ToLower(strings.TrimSpace(c.Query("refresh")))
	return value == "1" || value == "true" || value == "yes"
}

func marketForCode(code string) string {
	lower := strings.ToLower(strings.TrimSpace(code))
	if len(lower) >= 2 {
		switch lower[:2] {
		case "sh", "sz", "bj":
			return lower[:2]
		}
	}
	code = normalizeStockCodeQuery(code)
	if strings.HasPrefix(code, "6") {
		return "sh"
	}
	if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "4") {
		return "bj"
	}
	return "sz"
}

func setFreshnessHeaders(c *gin.Context, metadata stockdata.ResultMetadata) {
	c.Header("X-Data-Freshness", metadata.Freshness)
	c.Header("X-Data-Sync-Status", metadata.SyncStatus)
	if !metadata.AsOf.IsZero() {
		c.Header("X-Data-As-Of", metadata.AsOf.Format(time.RFC3339))
	}
}

func writeStockDataError(c *gin.Context, err error) {
	switch stockdata.CodeOf(err) {
	case stockdata.ErrInvalidRequest:
		WriteError(c, http.StatusBadRequest, string(stockdata.ErrInvalidRequest), "股票数据请求无效")
	case stockdata.ErrCacheMiss:
		WriteError(c, http.StatusNotFound, string(stockdata.ErrCacheMiss), "本地数据库中没有所需数据")
	case stockdata.ErrUpstreamTimeout:
		WriteError(c, http.StatusGatewayTimeout, string(stockdata.ErrUpstreamTimeout), "TDX 数据同步超时")
	case stockdata.ErrUpstream, stockdata.ErrStaleData:
		WriteError(c, http.StatusServiceUnavailable, string(stockdata.CodeOf(err)), "TDX 数据暂时不可用")
	default:
		WriteError(c, http.StatusInternalServerError, string(stockdata.ErrPersistence), "股票数据处理失败")
	}
}

// handleFinanceTrends handles finance trends requests (placeholder)
func (s *Server) handleFinanceTrends(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	mode := strings.ToLower(strings.TrimSpace(c.DefaultQuery("mode", "quarter")))
	if mode != "quarter" && mode != "year" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode 仅支持 quarter 或 year"})
		return
	}

	content, err := s.fetchFinanceAnalysisContent(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取财务趋势数据失败: %v", err)})
		return
	}

	records, metrics := parseFinanceTrendRecords(content, mode)
	if len(records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到可用于绘图的财务趋势数据"})
		return
	}

	c.JSON(http.StatusOK, financeTrendsResponse{
		Code:      code,
		Mode:      mode,
		Metrics:   metrics,
		Records:   records,
		Available: []string{"quarter", "year"},
	})
}

// handleFinanceMetrics handles finance metrics requests
func (s *Server) handleFinanceMetrics(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	content, err := s.fetchFinanceAnalysisContent(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取主要财务指标失败: %v", err)})
		return
	}

	tables := parseMainFinanceMetricTables(content)
	if len(tables) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到主要财务指标数据"})
		return
	}

	c.JSON(http.StatusOK, financeMetricTableResponse{Code: code, Tables: tables})
}

// handleCompany handles company category requests
func (s *Server) handleCompany(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	cats, err := withRetry(s, func() ([]*protocol.CompanyCategoryItem, error) {
		return s.svc.FetchCompanyCategory(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息目录失败: %v", err)})
		return
	}

	var result []gin.H
	for _, cat := range cats {
		result = append(result, gin.H{
			"Name":     cat.Name,
			"Filename": cat.Filename,
			"Start":    cat.Start,
			"Length":   cat.Length,
		})
	}
	c.JSON(http.StatusOK, result)
}

// handleCompanyContent handles company content requests
func (s *Server) handleCompanyContent(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

	block := c.Query("block")
	filename := c.Query("filename")

	start := uint32(0)
	length := uint32(10000)

	if block != "" {
		cats, err := withRetry(s, func() ([]*protocol.CompanyCategoryItem, error) {
			return s.svc.FetchCompanyCategory(code)
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息目录失败: %v", err)})
			return
		}
		found := false
		for _, cat := range cats {
			if cat.Name == block {
				filename = cat.Filename
				start = cat.Start
				length = cat.Length
				found = true
				break
			}
		}
		if !found {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("未找到块: %s", block)})
			return
		}
	} else if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 block 或 filename 参数"})
		return
	} else {
		if startStr := c.Query("start"); startStr != "" {
			if v, err := strconv.ParseUint(startStr, 10, 32); err == nil {
				start = uint32(v)
			}
		}
		if lengthStr := c.Query("length"); lengthStr != "" {
			if v, err := strconv.ParseUint(lengthStr, 10, 32); err == nil {
				length = uint32(v)
			}
		}
	}

	content, err := withRetry(s, func() (string, error) {
		return s.svc.FetchCompanyContent(code, filename, start, length)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": content})
}

// handleBlock handles legacy block requests
func (s *Server) handleBlock(c *gin.Context) {
	file := c.DefaultQuery("file", "block_zs.dat")

	items, err := withRetry(s, func() ([]*protocol.BlockItem, error) {
		return s.svc.FetchBlock(file)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取板块数据失败: %v", err)})
		return
	}

	// Group by block name
	blockMap := make(map[string]*gin.H)
	for _, item := range items {
		if _, ok := blockMap[item.BlockName]; !ok {
			blockMap[item.BlockName] = &gin.H{
				"Name":  item.BlockName,
				"Type":  item.BlockType,
				"Count": 0,
			}
		}
		(*blockMap[item.BlockName])["Count"] = (*blockMap[item.BlockName])["Count"].(int) + 1
	}

	var result []gin.H
	for _, b := range blockMap {
		result = append(result, *b)
	}
	c.JSON(http.StatusOK, result)
}

// handleBlockFiles handles block files list requests
func (s *Server) handleBlockFiles(c *gin.Context) {
	files := []gin.H{
		{"file": "block_gn.dat", "name": "概念板块", "desc": "概念主题"},
		{"file": "block_fg.dat", "name": "风格板块", "desc": "资金、风格与主题分类"},
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// handleBlockList handles structured block list requests
func (s *Server) handleBlockList(c *gin.Context) {
	file := c.DefaultQuery("file", "block_zs.dat")
	typeFilter := c.Query("type")

	items, err := withRetry(s, func() ([]*protocol.BlockItem, error) {
		return s.svc.FetchBlock(file)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取板块列表失败: %v", err)})
		return
	}

	// Group by block name and count
	blockMap := make(map[string]gin.H)
	for _, item := range items {
		if !isValidBlockNameServer(item.BlockName) || strings.TrimSpace(item.StockCode) == "" {
			continue
		}
		if typeFilter != "" {
			typeInt, _ := strconv.Atoi(typeFilter)
			if int(item.BlockType) != typeInt {
				continue
			}
		}
		if _, ok := blockMap[item.BlockName]; !ok {
			blockMap[item.BlockName] = gin.H{
				"name":  item.BlockName,
				"type":  item.BlockType,
				"count": 0,
			}
		}
		blockMap[item.BlockName]["count"] = blockMap[item.BlockName]["count"].(int) + 1
	}

	var blocks []gin.H
	for _, b := range blockMap {
		blocks = append(blocks, b)
	}
	if c.Query("sort") == "count" {
		sort.Slice(blocks, func(i, j int) bool {
			ci := blocks[i]["count"].(int)
			cj := blocks[j]["count"].(int)
			if ci != cj {
				return ci > cj
			}
			return blocks[i]["name"].(string) < blocks[j]["name"].(string)
		})
	} else {
		sort.Slice(blocks, func(i, j int) bool {
			return blocks[i]["name"].(string) < blocks[j]["name"].(string)
		})
	}

	c.JSON(http.StatusOK, gin.H{"blocks": blocks, "file": file, "total": len(blocks)})
}

// handleBlockShow handles block detail requests
func (s *Server) handleBlockShow(c *gin.Context) {
	name := c.Query("name")
	code := c.Query("code")
	file := c.DefaultQuery("file", "block_zs.dat")

	if name != "" {
		items, err := withRetry(s, func() ([]*protocol.BlockItem, error) {
			return s.svc.FetchBlock(file)
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取板块详情失败: %v", err)})
			return
		}

		// Find stocks in the matching block using serverBlockStats
		blockMap := serverBlockStats(items)

		// Find matching blocks
		var matchedStocks []string
		for blockName, stats := range blockMap {
			if isValidBlockNameServer(blockName) && (strings.Contains(blockName, name) || blockName == name) {
				matchedStocks = append(matchedStocks, stats.stockCodes...)
			}
		}

		if len(matchedStocks) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "block not found"})
			return
		}

		// Get stock names from preloaded code->name map
		codeNameMap := s.getCodeNameMapServer()

		var stockList []gin.H
		for _, sc := range matchedStocks {
			stockName := codeNameMap[sc]
			if stockName == "" {
				stockName = "未知"
			}
			stockList = append(stockList, gin.H{
				"code":     sc,
				"name":     stockName,
				"exchange": getExchangeFromCode(sc),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"stocks": stockList,
		})
		return
	}

	if code != "" {
		// Find blocks containing this stock
		files := []string{"block_zs.dat", "block_fg.dat", "block_gn.dat"}
		var blocks []gin.H

		for _, f := range files {
			items, err := withRetry(s, func() ([]*protocol.BlockItem, error) {
				return s.svc.FetchBlock(f)
			})
			if err != nil {
				continue
			}

			blockCountMap := make(map[string]int)
			blockTypeMap := make(map[string]uint16)

			for _, item := range items {
				blockCountMap[item.BlockName]++
				blockTypeMap[item.BlockName] = item.BlockType
			}

			for _, item := range items {
				if item.StockCode == code {
					blocks = append(blocks, gin.H{
						"name":  item.BlockName,
						"type":  blockTypeMap[item.BlockName],
						"count": blockCountMap[item.BlockName],
					})
					break // Only add once per block
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"blocks": blocks})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "name or code is required"})
}

// blockStatsServer 用于按板块名称分组统计
type blockStatsServer struct {
	blockType  uint16
	stockCodes []string
}

// serverBlockStats 按板块名称分组
func serverBlockStats(items []*protocol.BlockItem) map[string]*blockStatsServer {
	result := make(map[string]*blockStatsServer)
	for _, item := range items {
		if _, ok := result[item.BlockName]; !ok {
			result[item.BlockName] = &blockStatsServer{blockType: item.BlockType, stockCodes: make([]string, 0)}
		}
		result[item.BlockName].stockCodes = append(result[item.BlockName].stockCodes, item.StockCode)
	}
	return result
}

// isValidBlockNameServer 检查板块名称是否有效 (过滤掉纯数字)
func isValidBlockNameServer(name string) bool {
	if name == "" {
		return false
	}
	hasNonDigit := false
	for _, c := range name {
		if c < '0' || c > '9' {
			hasNonDigit = true
			break
		}
	}
	return hasNonDigit
}

// getCodeNameMapServer 获取股票代码到名称的映射
func (s *Server) getCodeNameMapServer() map[string]string {
	codeNameMap := make(map[string]string)

	codes, _ := s.svc.FetchCodes(protocol.ExchangeSH)
	for _, c := range codes {
		codeNameMap[c.Code] = c.Name
	}

	codes, _ = s.svc.FetchCodes(protocol.ExchangeSZ)
	for _, c := range codes {
		// 深市股票代码与部分上证指数代码重叠，板块成分股中 000/002/300 通常应优先展示深市股票名称。
		codeNameMap[c.Code] = c.Name
	}

	codes, _ = s.svc.FetchCodes(protocol.ExchangeBJ)
	for _, c := range codes {
		codeNameMap[c.Code] = c.Name
	}

	return codeNameMap
}

func (s *Server) resolveDisplayName(code, current string) string {
	code = strings.TrimSpace(code)
	current = strings.TrimSpace(current)
	items, err := s.getStockSearchIndex()
	if err == nil {
		for _, item := range items {
			if item.Code == code && strings.TrimSpace(item.Name) != "" {
				return strings.TrimSpace(item.Name)
			}
		}
	}
	if current == "" || strings.ContainsRune(current, '\ufffd') {
		return code
	}
	return current
}

func getExchangeFromCode(code string) string {
	if len(code) == 0 {
		return "sz"
	}
	if code[0] == '6' {
		return "sh"
	}
	if code[0] == '8' || code[0] == '4' {
		return "bj"
	}
	return "sz"
}

// handleCount handles security count requests
func (s *Server) handleCount(c *gin.Context) {
	exchange := c.Query("exchange")
	if exchange == "" {
		exchange = "sz"
	}

	var ex protocol.Exchange
	switch exchange {
	case "sh":
		ex = protocol.ExchangeSH
	case "bj":
		ex = protocol.ExchangeBJ
	default:
		ex = protocol.ExchangeSZ
	}

	count, err := s.svc.GetSecurityCount(ex)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"exchange": exchange, "count": count})
}

// findCorruptedKlines detects obviously corrupted kline data points.
// TDX delta-encoding can produce spikes (e.g., close jumping from 13 to 3000+)
// when the binary stream is corrupted. Returns the dates of corrupted entries.
func findCorruptedKlines(klines []*protocol.Kline) []string {
	if len(klines) == 0 {
		return nil
	}

	const maxPriceChangeRatio = 5.0 // 500% price change in one day is suspicious
	const maxPrice = 100000.0       // No A-share stock should exceed 100k yuan

	var corrupted []string
	var lastValidClose float64

	for i, k := range klines {
		isBad := false

		// Basic sanity checks
		if k.Close <= 0 || k.Open <= 0 || k.High <= 0 || k.Low <= 0 {
			isBad = true
		} else if k.High < k.Low {
			isBad = true
		} else if k.Close > maxPrice || k.Open > maxPrice {
			isBad = true
		} else if i > 0 && lastValidClose > 0 {
			// Check for unreasonable price jump from last valid close
			ratio := k.Close / lastValidClose
			if ratio > maxPriceChangeRatio || ratio < 1.0/maxPriceChangeRatio {
				isBad = true
			}
		}

		if isBad {
			corrupted = append(corrupted, k.Time.Format("2006-01-02"))
		} else {
			lastValidClose = k.Close
		}
	}

	return corrupted
}

// handleIndicator handles indicator requests
