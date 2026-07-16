package server

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	pinyin "github.com/mozillazg/go-pinyin"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
)

// Server holds the service and stores for the HTTP server
type Server struct {
	svc                   *tdx.Service
	historyDB             *history.Store
	watchlistDB           *watchlist.Store
	stockSearchIndexCache stockSearchIndexCache
	tdxMu                 sync.Mutex
}

const (
	stockSearchDefaultLimit = 10
	stockSearchMaxLimit     = 20
	stockSearchIndexTTL     = 10 * time.Minute
)

func isStockCode(code string) bool {
	switch {
	case strings.HasPrefix(code, "688"):
		return true
	case strings.HasPrefix(code, "6"):
		return true
	case strings.HasPrefix(code, "300"), strings.HasPrefix(code, "301"):
		return true
	case strings.HasPrefix(code, "399"):
		return true
	case strings.HasPrefix(code, "000"), strings.HasPrefix(code, "001"):
		return true
	case strings.HasPrefix(code, "002"):
		return true
	case strings.HasPrefix(code, "8"):
		return true
	case strings.HasPrefix(code, "5"):
		return true
	default:
		return false
	}
}

// NewServer creates a new Server instance
func NewServer(svc *tdx.Service, historyDB *history.Store, watchlistDB *watchlist.Store) *Server {
	return &Server{
		svc:                   svc,
		historyDB:             historyDB,
		watchlistDB:           watchlistDB,
		stockSearchIndexCache: stockSearchIndexCache{},
	}
}

// Stock search types
type stockSearchMatch struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Exchange  string `json:"exchange"`
	MatchType string `json:"matchType"`
}

type stockSearchIndexResponse struct {
	UpdatedAt int64                   `json:"updatedAt"`
	Total     int                     `json:"total"`
	Items     []stockSearchIndexEntry `json:"items"`
}

type stockSearchIndexEntry struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	NameNorm string `json:"nameNorm"`
	Pinyin   string `json:"pinyin"`
	Initials string `json:"initials"`
}

type stockSearchResponse struct {
	Query    string             `json:"query"`
	Total    int                `json:"total"`
	Exact    bool               `json:"exact"`
	Resolved bool               `json:"resolved"`
	Matches  []stockSearchMatch `json:"matches"`
}

type stockSearchErrorResponse struct {
	Error   string             `json:"error"`
	Query   string             `json:"query"`
	Total   int                `json:"total"`
	Matches []stockSearchMatch `json:"matches"`
}

type stockSearchIndexItem struct {
	Code       string
	Name       string
	Exchange   string
	NameNorm   string
	PinyinNorm string
	Initials   string
}

type scoredStockMatch struct {
	stockSearchMatch
	Score int
}

type stockSearchIndexCache struct {
	sync.RWMutex
	builtAt time.Time
	items   []stockSearchIndexItem
}

// TDX service management
// Note: With Pool mode, service is created at startup and passed to Server.
// Pool handles reconnection internally. withRetry only provides mutex locking for thread safety.

func withRetry[T any](s *Server, fn func() (T, error)) (T, error) {
	s.tdxMu.Lock()
	result, err := fn()
	s.tdxMu.Unlock()
	return result, err
}

// Stock search helper functions
func (s *Server) resolveStockCodeOrRespond(c *gin.Context, raw string) (string, bool) {
	query := strings.TrimSpace(raw)
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return "", false
	}

	// Fast path: 6-digit numeric code is always treated as a direct stock code.
	// Skip search index — let the TDX data fetch validate existence.
	if code := normalizeStockCodeQuery(query); code != "" {
		return code, true
	}

	matches, resolved, _, err := s.searchStockMatches(query, stockSearchDefaultLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return "", false
	}
	if len(matches) == 0 {
		c.JSON(http.StatusNotFound, stockSearchErrorResponse{Error: "未找到匹配股票", Query: query, Total: 0, Matches: []stockSearchMatch{}})
		return "", false
	}
	if !resolved {
		c.JSON(http.StatusConflict, stockSearchErrorResponse{Error: "找到多个匹配股票，请先选择具体个股", Query: query, Total: len(matches), Matches: matches})
		return "", false
	}
	return matches[0].Code, true
}

func (s *Server) searchStockMatches(query string, limit int) ([]stockSearchMatch, bool, bool, error) {
	if limit <= 0 {
		limit = stockSearchDefaultLimit
	}
	if limit > stockSearchMaxLimit {
		limit = stockSearchMaxLimit
	}

	items, err := s.getStockSearchIndex()
	if err != nil {
		return nil, false, false, err
	}

	normalizedQuery := normalizeStockSearchText(query)
	normalizedCode := normalizeStockCodeQuery(query)
	if normalizedQuery == "" && normalizedCode == "" {
		return []stockSearchMatch{}, false, false, nil
	}

	matches := make([]scoredStockMatch, 0, limit)
	var exactCodeMatch *scoredStockMatch
	for _, item := range items {
		score, matchType, ok := s.scoreStockMatch(item, normalizedQuery, normalizedCode)
		if !ok {
			continue
		}
		m := scoredStockMatch{stockSearchMatch: stockSearchMatch{Code: item.Code, Name: item.Name, Exchange: item.Exchange, MatchType: matchType}, Score: score}
		matches = append(matches, m)
		if matchType == "exact_code" && exactCodeMatch == nil {
			exactCodeMatch = &m
		}
	}

	// If there's an exact code match, return it directly as resolved
	if exactCodeMatch != nil {
		result := []stockSearchMatch{exactCodeMatch.stockSearchMatch}
		return result, true, true, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].Code != matches[j].Code {
			return matches[i].Code < matches[j].Code
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}

	result := make([]stockSearchMatch, len(matches))
	for i, match := range matches {
		result[i] = match.stockSearchMatch
	}
	exact := len(result) == 1 && strings.HasPrefix(result[0].MatchType, "exact_")
	resolved := len(result) == 1
	return result, resolved, exact, nil
}

func (s *Server) scoreStockMatch(item stockSearchIndexItem, normalizedQuery, normalizedCode string) (int, string, bool) {
	if normalizedCode != "" {
		switch {
		case item.Code == normalizedCode:
			return 1000, "exact_code", true
		case strings.HasPrefix(item.Code, normalizedCode):
			return 900, "prefix_code", true
		case strings.Contains(item.Code, normalizedCode):
			return 760, "contains_code", true
		}
	}
	if normalizedQuery == "" {
		return 0, "", false
	}
	if item.NameNorm == normalizedQuery {
		return 980, "exact_name", true
	}
	if item.PinyinNorm == normalizedQuery {
		return 970, "exact_pinyin", true
	}
	if item.Initials == normalizedQuery {
		return 960, "exact_initials", true
	}
	if strings.HasPrefix(item.NameNorm, normalizedQuery) {
		return 880, "prefix_name", true
	}
	if strings.HasPrefix(item.PinyinNorm, normalizedQuery) {
		return 870, "prefix_pinyin", true
	}
	if strings.HasPrefix(item.Initials, normalizedQuery) {
		return 860, "prefix_initials", true
	}
	if strings.Contains(item.NameNorm, normalizedQuery) {
		return 780, "contains_name", true
	}
	if strings.Contains(item.PinyinNorm, normalizedQuery) {
		return 770, "contains_pinyin", true
	}
	if strings.Contains(item.Initials, normalizedQuery) {
		return 765, "contains_initials", true
	}
	return 0, "", false
}

func (s *Server) getStockSearchIndex() ([]stockSearchIndexItem, error) {
	s.stockSearchIndexCache.RLock()
	if time.Since(s.stockSearchIndexCache.builtAt) < stockSearchIndexTTL && len(s.stockSearchIndexCache.items) > 0 {
		items := s.stockSearchIndexCache.items
		s.stockSearchIndexCache.RUnlock()
		return items, nil
	}
	s.stockSearchIndexCache.RUnlock()

	s.stockSearchIndexCache.Lock()
	defer s.stockSearchIndexCache.Unlock()
	if time.Since(s.stockSearchIndexCache.builtAt) < stockSearchIndexTTL && len(s.stockSearchIndexCache.items) > 0 {
		return s.stockSearchIndexCache.items, nil
	}

	svc := s.svc
	if svc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	sources := []struct {
		exchange protocol.Exchange
		label    string
	}{{protocol.ExchangeSH, "上交所"}, {protocol.ExchangeSZ, "深交所"}, {protocol.ExchangeBJ, "北交所"}}

	items := make([]stockSearchIndexItem, 0, 6000)
	for _, source := range sources {
		codes, err := svc.FetchCodes(source.exchange)
		if err != nil {
			return nil, err
		}
		for _, code := range codes {
			if !isStockCode(code.Code) {
				continue
			}
			item := stockSearchIndexItem{Code: code.Code, Name: code.Name, Exchange: source.label}
			item.NameNorm = normalizeStockSearchText(item.Name)
			item.PinyinNorm, item.Initials = buildStockPinyinKeys(item.Name)
			items = append(items, item)
		}
	}
	s.stockSearchIndexCache.items = items
	s.stockSearchIndexCache.builtAt = time.Now()
	return items, nil
}

func buildStockPinyinKeys(name string) (string, string) {
	baseArgs := pinyin.NewArgs()
	baseArgs.Fallback = func(r rune, _ pinyin.Args) []string { return []string{string(r)} }
	full := normalizeStockSearchText(strings.Join(pinyin.LazyPinyin(name, baseArgs), ""))

	initialArgs := pinyin.NewArgs()
	initialArgs.Style = pinyin.FirstLetter
	initialArgs.Fallback = func(r rune, _ pinyin.Args) []string { return []string{string(r)} }
	initials := normalizeStockSearchText(strings.Join(pinyin.LazyPinyin(name, initialArgs), ""))
	return full, initials
}

func normalizeStockSearchText(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	input = strings.ReplaceAll(input, " ", "")
	input = strings.ReplaceAll(input, "-", "")
	input = strings.ReplaceAll(input, "_", "")
	return input
}

func normalizeStockCodeQuery(input string) string {
	input = normalizeStockSearchText(input)
	if len(input) == 8 {
		prefix := input[:2]
		if prefix == "sh" || prefix == "sz" || prefix == "bj" {
			input = input[2:]
		}
	}
	if len(input) != 6 {
		return ""
	}
	for _, ch := range input {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return input
}

// SetupRoutes configures all API routes
func (s *Server) SetupRoutes(r *gin.Engine) {
	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	// API group
	api := r.Group("/api")
	{
		// Quote
		api.GET("/quote", s.handleQuote)

		// Codes
		api.GET("/codes", s.handleCodes)
		api.GET("/codes/list", s.handleCodesList)
		api.GET("/codes/stats", s.handleCodesStats)

		// Kline
		api.GET("/kline", s.handleKline)

		// Index
		api.GET("/index", s.handleIndex)

		// Minute
		api.GET("/minute", s.handleMinute)

		// Trade
		api.GET("/trade", s.handleTrade)

		// Auction
		api.GET("/auction", s.handleAuction)

		// XdXr
		api.GET("/xdxr", s.handleXdXr)

		// Finance
		api.GET("/finance", s.handleFinance)
		api.GET("/finance/trends", s.handleFinanceTrends)
		api.GET("/finance/metrics", s.handleFinanceMetrics)

		// Company
		api.GET("/company", s.handleCompany)
		api.GET("/company/content", s.handleCompanyContent)

		// Block
		api.GET("/block", s.handleBlock)
		api.GET("/block/files", s.handleBlockFiles)
		api.GET("/block/list", s.handleBlockList)
		api.GET("/block/show", s.handleBlockShow)

		// Count
		api.GET("/count", s.handleCount)

		// Indicator
		api.GET("/indicator", s.handleIndicator)

		// Screen
		api.GET("/screen", s.handleScreen)

		// Signal Analysis
		api.GET("/signal-analysis", s.handleSignalAnalysis)

		// Stock Compare
		api.GET("/stock/compare", s.handleStockCompare)

		// Stock Search
		api.GET("/stocks/search", s.handleStockSearch)
		api.GET("/stocks/search-index", s.handleStockSearchIndex)

		// History
		api.GET("/history", s.handleHistoryList)
		api.POST("/history", s.handleHistoryAdd)
		api.DELETE("/history/:code", s.handleHistoryDelete)

		// Watchlist
		api.GET("/watchlist", s.handleWatchlistList)
		api.POST("/watchlist", s.handleWatchlistAdd)
		api.DELETE("/watchlist/:code", s.handleWatchlistDelete)
		api.PUT("/watchlist/:code/note", s.handleWatchlistUpdateNote)
		api.PUT("/watchlist/:code/group", s.handleWatchlistUpdateGroup)
		api.GET("/watchlist/groups", s.handleWatchlistGroups)

		// Sync
		api.POST("/sync/daily", s.handleSyncDaily)
		api.GET("/sync/state", s.handleSyncState)

		// Data cleanup
		api.POST("/kline/clean", s.handleCleanKlines)

		// Settings
		api.GET("/settings/indicator", s.handleIndicatorSettings)
		api.PUT("/settings/indicator", s.handleSaveIndicatorSettings)
	}
}

// handleQuote handles quote requests
func (s *Server) handleQuote(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	quotes, err := withRetry(s, func() ([]*protocol.QuoteItem, error) {
		return s.svc.Client.GetQuote(code)
	})
	if err != nil {
		fallback, fallbackErr := s.fallbackQuoteFromKline(code)
		if fallbackErr == nil {
			c.JSON(http.StatusOK, fallback)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取行情失败: %v", err)})
		return
	}
	if len(quotes) == 0 {
		fallback, fallbackErr := s.fallbackQuoteFromKline(code)
		if fallbackErr == nil {
			c.JSON(http.StatusOK, fallback)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该股票"})
		return
	}
	quotes[0].Name = s.resolveDisplayName(code, quotes[0].Name)
	c.JSON(http.StatusOK, quotes[0])
}

func (s *Server) fallbackQuoteFromKline(code string) (*protocol.QuoteItem, error) {
	klines, err := s.svc.FetchKlineAll(code, tdx.ParseKlineType("day"))
	if err != nil {
		return nil, err
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("no kline data")
	}
	last := klines[len(klines)-1]
	if !isFiniteKlinePrice(last.Open) || !isFiniteKlinePrice(last.High) || !isFiniteKlinePrice(last.Low) || !isFiniteKlinePrice(last.Close) {
		return nil, fmt.Errorf("invalid latest kline")
	}
	lastClose := last.Close
	if len(klines) > 1 && isFiniteKlinePrice(klines[len(klines)-2].Close) {
		lastClose = klines[len(klines)-2].Close
	}
	return &protocol.QuoteItem{
		Code:      code,
		Name:      s.resolveDisplayName(code, ""),
		Open:      last.Open,
		High:      last.High,
		Low:       last.Low,
		Price:     last.Close,
		LastClose: lastClose,
		Volume:    last.Volume,
		Amount:    last.Amount,
	}, nil
}

func isFiniteKlinePrice(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
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
	} else {
		klines, err = withRetry(s, func() ([]*protocol.Kline, error) {
			return s.svc.FetchKlineAll(code, ktype)
		})
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线失败: %v", err)})
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
		return s.svc.Client.GetIndexBars(code, ktype, 0, 500)
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
			return s.svc.Client.GetHistoryMinute(date, code)
		})
	} else {
		result, err = withRetry(s, func() (*protocol.MinuteResp, error) {
			return s.svc.Client.GetMinute(code)
		})
	}

	if err != nil {
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
			return s.svc.Client.GetHistoryMinuteTrade(date, code, 0, 2000)
		})
	} else {
		result, err = withRetry(s, func() (*protocol.TradeResp, error) {
			return s.svc.Client.GetMinuteTradeAll(code)
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
		return s.svc.Client.GetCallAuction(code)
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

	info, err := withRetry(s, func() (*protocol.FinanceInfo, error) {
		return s.svc.FetchFinance(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取财务数据失败: %v", err)})
		return
	}

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

	content, err := s.fetchCompanyBlockContent(code, "财务分析")
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

	content, err := s.fetchCompanyBlockContent(code, "财务分析")
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

	count, err := s.svc.Client.GetSecurityCount(ex)
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
		return s.svc.Client.GetQuote(code)
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
					return s.svc.Client.GetQuote(code)
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
			signals := signal.Detect(code, inputs, result, signal.DefaultDetectOptions())

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
func (s *Server) handleHistoryList(c *gin.Context) {
	stocks, err := s.historyDB.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if stocks == nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}

	var data []gin.H
	for _, stock := range stocks {
		resolvedName := s.resolveDisplayName(stock.Code, stock.Name)
		data = append(data, gin.H{
			"code":        stock.Code,
			"name":        resolvedName,
			"analyzed_at": stock.AnalyzedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleHistoryAdd handles history add requests
func (s *Server) handleHistoryAdd(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	stock := history.HistoryStock{
		Code:       req.Code,
		Name:       s.resolveDisplayName(req.Code, req.Name),
		AnalyzedAt: time.Now(),
	}

	if err := s.historyDB.Upsert(stock); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "added"})
}

// handleHistoryDelete handles history delete requests
func (s *Server) handleHistoryDelete(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	if err := s.historyDB.Delete(code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// handleWatchlistList handles watchlist list requests
func (s *Server) handleWatchlistList(c *gin.Context) {
	group := c.Query("group")

	var stocks []watchlist.WatchlistStock
	var err error

	if group != "" {
		stocks, err = s.watchlistDB.GetByGroup(group)
	} else {
		stocks, err = s.watchlistDB.GetAll()
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if stocks == nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}

	var data []gin.H
	for _, stock := range stocks {
		resolvedName := s.resolveDisplayName(stock.Code, stock.Name)
		data = append(data, gin.H{
			"code":       stock.Code,
			"name":       resolvedName,
			"group":      stock.Group,
			"note":       stock.Note,
			"added_at":   stock.AddedAt.Format(time.RFC3339),
			"updated_at": stock.UpdatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleWatchlistAdd handles watchlist add requests
func (s *Server) handleWatchlistAdd(c *gin.Context) {
	var req struct {
		Code  string `json:"code"`
		Name  string `json:"name"`
		Group string `json:"group"`
		Note  string `json:"note"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	stock := watchlist.WatchlistStock{
		Code:    req.Code,
		Name:    s.resolveDisplayName(req.Code, req.Name),
		Group:   req.Group,
		Note:    req.Note,
		AddedAt: time.Now(),
	}

	if err := s.watchlistDB.Upsert(stock); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "added"})
}

// handleWatchlistDelete handles watchlist delete requests
func (s *Server) handleWatchlistDelete(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	if err := s.watchlistDB.Delete(code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// handleWatchlistUpdateNote handles watchlist note update requests
func (s *Server) handleWatchlistUpdateNote(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.watchlistDB.UpdateNote(code, req.Note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// handleWatchlistUpdateGroup handles watchlist group update requests
func (s *Server) handleWatchlistUpdateGroup(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	var req struct {
		Group string `json:"group"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.watchlistDB.UpdateGroup(code, req.Group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// handleWatchlistGroups handles watchlist groups list requests
func (s *Server) handleWatchlistGroups(c *gin.Context) {
	groups, err := s.watchlistDB.GetGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	counts, err := s.watchlistDB.CountByGroup()
	if err != nil {
		counts = make(map[string]int)
	}

	data := make([]gin.H, 0)
	for _, g := range groups {
		data = append(data, gin.H{
			"name":  g,
			"count": counts[g],
		})
	}

	c.JSON(http.StatusOK, gin.H{"groups": data})
}

// normalizeCodeList 标准化代码列表
func normalizeCodeList(codes []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		result = append(result, code)
	}
	return result
}

// handleSyncDaily handles daily sync requests
func (s *Server) handleSyncDaily(c *gin.Context) {
	var req struct {
		Codes       []string `json:"codes"`
		Mode        string   `json:"mode"`
		Concurrency int      `json:"concurrency"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Normalize code list
	codes := normalizeCodeList(req.Codes)
	if len(codes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "codes is required"})
		return
	}

	mode := tdx.SyncMode(req.Mode)
	if mode == "" {
		mode = tdx.SyncModeAuto
	}

	// Check service availability first
	if s.svc == nil {
		log.Printf("[sync] 服务不可用: s.svc is nil")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务不可用: s.svc is nil"})
		return
	}

	result := s.svc.SyncDailyKlines(codes, mode, req.Concurrency)
	c.JSON(http.StatusOK, result)
}

// handleSyncState returns sync state for a given code without triggering a sync.
func (s *Server) handleSyncState(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	ktypeStr := c.DefaultQuery("ktype", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	state, err := s.svc.GetSyncState(code, ktype)
	if err != nil {
		// No sync state record found — return empty state
		c.JSON(http.StatusOK, gin.H{
			"code":   code,
			"ktype":  ktype,
			"status": "unknown",
		})
		return
	}

	c.JSON(http.StatusOK, state)
}

// handleCleanKlines handles requests to clean corrupted kline data and re-fetch.
func (s *Server) handleCleanKlines(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	// Clean corrupted data and re-fetch
	klines, err := s.svc.CleanAndRefetchKlines(code, ktype)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"count":   len(klines),
		"message": "数据清理完成并已重新获取",
	})
}

// handleIndicatorSettings handles indicator settings requests
func (s *Server) handleIndicatorSettings(c *gin.Context) {
	config, err := param.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, indicatorConfigToPayload(config))
}

// handleSaveIndicatorSettings handles save indicator settings requests
func (s *Server) handleSaveIndicatorSettings(c *gin.Context) {
	var payload indicatorParamPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	normalizeIndicatorPayload(&payload)
	if err := validateIndicatorPayload(payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := param.SaveConfig(payloadToIndicatorConfig(payload)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, err := param.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok", "config": indicatorConfigToPayload(cfg)})
}

// indicatorParamPayload 是指标参数字段负载结构
type indicatorParamPayload struct {
	Defaults   indicatorCategoryPayload            `json:"defaults" yaml:"defaults"`
	Categories map[string]indicatorCategoryPayload `json:"categories,omitempty" yaml:"categories,omitempty"`
	Overrides  map[string]indicatorCategoryPayload `json:"overrides,omitempty" yaml:"overrides,omitempty"`
	Path       string                              `json:"path,omitempty"`
}

// indicatorCategoryPayload 是指标分类参数字段负载
type indicatorCategoryPayload struct {
	MA   []int          `json:"ma,omitempty" yaml:"ma,omitempty"`
	MACD *ta.MACDConfig `json:"macd,omitempty" yaml:"macd,omitempty"`
	KDJ  *ta.KDJConfig  `json:"kdj,omitempty" yaml:"kdj,omitempty"`
	BOLL *ta.BOLLConfig `json:"boll,omitempty" yaml:"boll,omitempty"`
	RSI  []int          `json:"rsi,omitempty" yaml:"rsi,omitempty"`
}

// indicatorCategoryToPayload 将 CategoryParams 转换为 indicatorCategoryPayload
func indicatorCategoryToPayload(src param.CategoryParams) indicatorCategoryPayload {
	return indicatorCategoryPayload{
		MA:   append([]int(nil), src.MA...),
		MACD: cloneMACDConfig(src.MACD),
		KDJ:  cloneKDJConfig(src.KDJ),
		BOLL: cloneBOLLConfig(src.BOLL),
		RSI:  append([]int(nil), src.RSI...),
	}
}

// indicatorConfigToPayload 将 ParamConfig 转换为 indicatorParamPayload
func indicatorConfigToPayload(cfg *param.ParamConfig) indicatorParamPayload {
	payload := indicatorParamPayload{
		Defaults: indicatorCategoryToPayload(cfg.Defaults),
	}
	if len(cfg.Categories) > 0 {
		payload.Categories = make(map[string]indicatorCategoryPayload, len(cfg.Categories))
		for key, value := range cfg.Categories {
			payload.Categories[key] = indicatorCategoryToPayload(value)
		}
	}
	if len(cfg.Overrides) > 0 {
		payload.Overrides = make(map[string]indicatorCategoryPayload, len(cfg.Overrides))
		for key, value := range cfg.Overrides {
			payload.Overrides[key] = indicatorCategoryToPayload(value)
		}
	}
	return payload
}

// payloadCategoryToParam 将 indicatorCategoryPayload 转换为 CategoryParams
func payloadCategoryToParam(src indicatorCategoryPayload) param.CategoryParams {
	return param.CategoryParams{
		MA:   append([]int(nil), src.MA...),
		MACD: cloneMACDConfig(src.MACD),
		KDJ:  cloneKDJConfig(src.KDJ),
		BOLL: cloneBOLLConfig(src.BOLL),
		RSI:  append([]int(nil), src.RSI...),
	}
}

// payloadToIndicatorConfig 将 indicatorParamPayload 转换为 ParamConfig
func payloadToIndicatorConfig(payload indicatorParamPayload) *param.ParamConfig {
	cfg := &param.ParamConfig{
		Defaults: payloadCategoryToParam(payload.Defaults),
	}
	if len(payload.Categories) > 0 {
		cfg.Categories = make(map[string]param.CategoryParams, len(payload.Categories))
		for key, value := range payload.Categories {
			cfg.Categories[key] = payloadCategoryToParam(value)
		}
	}
	if len(payload.Overrides) > 0 {
		cfg.Overrides = make(map[string]param.CategoryParams, len(payload.Overrides))
		for key, value := range payload.Overrides {
			cfg.Overrides[key] = payloadCategoryToParam(value)
		}
	}
	return cfg
}

func cloneMACDConfig(src *ta.MACDConfig) *ta.MACDConfig {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneKDJConfig(src *ta.KDJConfig) *ta.KDJConfig {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneBOLLConfig(src *ta.BOLLConfig) *ta.BOLLConfig {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func normalizeIndicatorPayload(payload *indicatorParamPayload) {
	payload.Defaults.MA = normalizePeriods(payload.Defaults.MA)
	payload.Defaults.RSI = normalizePeriods(payload.Defaults.RSI)
	if payload.Categories == nil {
		payload.Categories = map[string]indicatorCategoryPayload{}
	}
	for key, value := range payload.Categories {
		value.MA = normalizePeriods(value.MA)
		value.RSI = normalizePeriods(value.RSI)
		payload.Categories[key] = value
	}
	if payload.Overrides == nil {
		payload.Overrides = map[string]indicatorCategoryPayload{}
	}
	for key, value := range payload.Overrides {
		value.MA = normalizePeriods(value.MA)
		value.RSI = normalizePeriods(value.RSI)
		payload.Overrides[key] = value
	}
}

func normalizePeriods(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func validateIndicatorCategory(name string, payload indicatorCategoryPayload, allowEmpty bool) error {
	if len(payload.MA) > 0 {
		for _, value := range payload.MA {
			if value <= 0 {
				return fmt.Errorf("%s 的 ma 周期必须大于 0", name)
			}
		}
	}
	if payload.MACD != nil {
		if payload.MACD.Fast <= 0 || payload.MACD.Slow <= 0 || payload.MACD.Signal <= 0 {
			return fmt.Errorf("%s 的 MACD 参数必须大于 0", name)
		}
	}
	if payload.KDJ != nil {
		if payload.KDJ.N <= 0 || payload.KDJ.M1 <= 0 || payload.KDJ.M2 <= 0 {
			return fmt.Errorf("%s 的 KDJ 参数必须大于 0", name)
		}
	}
	if payload.BOLL != nil {
		if payload.BOLL.N <= 0 || payload.BOLL.K <= 0 {
			return fmt.Errorf("%s 的 BOLL 参数必须大于 0", name)
		}
	}
	if len(payload.RSI) > 0 {
		for _, value := range payload.RSI {
			if value <= 0 {
				return fmt.Errorf("%s 的 RSI 周期必须大于 0", name)
			}
		}
	}
	if allowEmpty {
		return nil
	}
	if len(payload.MA) == 0 && payload.MACD == nil && payload.KDJ == nil && payload.BOLL == nil && len(payload.RSI) == 0 {
		return fmt.Errorf("%s 至少需要一个指标配置", name)
	}
	return nil
}

func validateIndicatorPayload(payload indicatorParamPayload) error {
	if err := validateIndicatorCategory("默认参数", payload.Defaults, false); err != nil {
		return err
	}
	for key, value := range payload.Categories {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("分类名称不能为空")
		}
		if err := validateIndicatorCategory(fmt.Sprintf("分类 %s", key), value, true); err != nil {
			return err
		}
	}
	for key, value := range payload.Overrides {
		if matched, _ := regexp.MatchString(`^\d{6}$`, key); !matched {
			return fmt.Errorf("个股覆盖代码 %s 非法", key)
		}
		if err := validateIndicatorCategory(fmt.Sprintf("个股 %s", key), value, true); err != nil {
			return err
		}
	}
	return nil
}

// Finance metric types
type financeTrendRecord struct {
	Period          string   `json:"period"`
	Year            int      `json:"year"`
	Quarter         string   `json:"quarter"`
	Label           string   `json:"label"`
	Revenue         *float64 `json:"revenue,omitempty"`
	NetProfit       *float64 `json:"netProfit,omitempty"`
	GrossMargin     *float64 `json:"grossMargin,omitempty"`
	NetMargin       *float64 `json:"netMargin,omitempty"`
	ROE             *float64 `json:"roe,omitempty"`
	EPS             *float64 `json:"eps,omitempty"`
	OperatingCashPS *float64 `json:"operatingCashPerShare,omitempty"`
}

type financeTrendsResponse struct {
	Code      string               `json:"code"`
	Mode      string               `json:"mode"`
	Metrics   []string             `json:"metrics"`
	Records   []financeTrendRecord `json:"records"`
	Available []string             `json:"available"`
}

type financeMetricTableResponse struct {
	Code   string               `json:"code"`
	Tables []financeMetricTable `json:"tables"`
}

type financeMetricTable struct {
	Title   string             `json:"title"`
	Periods []string           `json:"periods"`
	Rows    []financeMetricRow `json:"rows"`
}

type financeMetricRow struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// handleStockCompare handles stock vs block comparison requests
func (s *Server) handleStockCompare(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	// Get stock quote
	quotes, err := s.svc.Client.GetQuote(code)
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

	for _, f := range files {
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
			const maxBlockStocks = 100
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

			// Bounded concurrent quote fetching
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
				qwg.Add(1)
				go func(idx int, stockCode string) {
					defer qwg.Done()
					qsem <- struct{}{}
					defer func() { <-qsem }()

					qs, err := s.svc.Client.GetQuote(stockCode)
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
	var cats []*protocol.CompanyCategoryItem
	var err error

	cats, err = withRetry(s, func() ([]*protocol.CompanyCategoryItem, error) {
		return s.svc.FetchCompanyCategory(code)
	})
	if err != nil {
		return "", err
	}

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

func parseMainFinanceMetricTables(content string) []financeMetricTable {
	return parseFinanceMetricTablesInSection(content, "【1.主要财务指标】", "【2.", []string{"年度对比", "最新季度"}, 2)
}

func parseProfitabilityFinanceMetricTables(content string) []financeMetricTable {
	return parseFinanceMetricTablesInSection(content, "【4.盈利能力指标】", "【5.", []string{"盈利年度对比", "盈利最新季度"}, 2)
}

func parseFinanceMetricTablesInSection(content, sectionTitle, nextSectionPrefix string, titles []string, maxTables int) []financeMetricTable {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), sectionTitle) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	tables := make([]financeMetricTable, 0, maxTables)
	for i := start; i < len(lines) && (maxTables <= 0 || len(tables) < maxTables); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, nextSectionPrefix) {
			break
		}
		if !strings.HasPrefix(line, "┌") {
			continue
		}
		rows := extractBoxTableRows(lines[i:])
		if table := buildFinanceMetricTable(rows, titleAt(titles, len(tables))); len(table.Periods) > 0 && len(table.Rows) > 0 {
			tables = append(tables, table)
		}
	}
	return tables
}

func titleAt(titles []string, index int) string {
	if index >= 0 && index < len(titles) && titles[index] != "" {
		return titles[index]
	}
	return "财务指标"
}

func buildFinanceMetricTable(rows [][]string, title string) financeMetricTable {
	if len(rows) < 2 || len(rows[0]) < 2 {
		return financeMetricTable{}
	}
	periods := make([]string, 0, len(rows[0])-1)
	for _, header := range rows[0][1:] {
		period := strings.TrimSpace(header)
		if regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(period) {
			periods = append(periods, period)
		}
	}
	if len(periods) == 0 {
		return financeMetricTable{}
	}
	result := financeMetricTable{Title: title, Periods: periods, Rows: make([]financeMetricRow, 0, len(rows)-1)}
	for _, row := range mergeWrappedFinanceMetricRows(rows[1:]) {
		if len(row) < len(periods)+1 {
			continue
		}
		name := sanitizeFinanceMetricName(row[0])
		if name == "" || strings.Contains(name, "审计意见") {
			continue
		}
		values := make([]string, 0, len(periods))
		for _, value := range row[1 : len(periods)+1] {
			values = append(values, strings.TrimSpace(value))
		}
		result.Rows = append(result.Rows, financeMetricRow{Name: name, Values: values})
	}
	return result
}

func mergeWrappedFinanceMetricRows(rows [][]string) [][]string {
	merged := make([][]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		first := strings.TrimSpace(row[0])
		if first != "" && len(merged) > 0 && allEmptyCells(row[1:]) {
			prev := merged[len(merged)-1]
			prev[0] = strings.TrimSpace(prev[0] + first)
			merged[len(merged)-1] = prev
			continue
		}
		merged = append(merged, append([]string(nil), row...))
	}
	return merged
}

func allEmptyCells(cells []string) bool {
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func parseFinanceTrendRecords(content, mode string) ([]financeTrendRecord, []string) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	mainTables := parseMainFinanceMetricTables(content)
	if len(mainTables) > 0 {
		idx := 1
		if mode == "year" || len(mainTables) == 1 {
			idx = 0
		}
		if idx < len(mainTables) {
			if records, metrics := financeTrendRecordsFromMetricTable(mainTables[idx]); len(records) > 0 {
				profitTables := parseProfitabilityFinanceMetricTables(content)
				if idx < len(profitTables) {
					supplementalRecords, supplementalMetrics := financeTrendRecordsFromMetricTable(profitTables[idx])
					records, metrics = mergeFinanceTrendRecordsByPeriod(records, supplementalRecords, metrics, supplementalMetrics)
				}
				return records, metrics
			}
		}
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	var yearRecords []financeTrendRecord
	var yearMetrics []string
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, "近五年每股收益对比") {
			continue
		}
		if records, metrics := parseYearFinanceTable(lines[i+1:]); len(records) > 0 {
			yearRecords = records
			yearMetrics = metrics
			break
		}
	}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, "最新财报") {
			continue
		}
		if records, metrics := parseQuarterFinanceTable(lines[i+1:]); len(records) > 0 {
			if mode == "quarter" {
				return records, metrics
			}
			if mode == "year" {
				return mergeYearFinanceRecords(aggregateQuarterFinanceRecords(records), yearRecords, metrics, yearMetrics)
			}
		}
	}
	if mode == "year" {
		return yearRecords, yearMetrics
	}
	return nil, nil
}

func financeTrendRecordsFromMetricTable(table financeMetricTable) ([]financeTrendRecord, []string) {
	if len(table.Periods) == 0 || len(table.Rows) == 0 {
		return nil, nil
	}
	records := make([]financeTrendRecord, len(table.Periods))
	for i, period := range table.Periods {
		records[i] = financeTrendRecord{Period: period, Year: parseYear(period), Quarter: quarterLabel(period), Label: quarterLabel(period)}
	}
	metricSeen := map[string]struct{}{}
	metrics := make([]string, 0, 7)
	for _, row := range table.Rows {
		assignFinanceMetricValues(records, row.Name, row.Values)
		for _, metric := range financeMetricKeysForName(row.Name) {
			if _, ok := metricSeen[metric]; ok {
				continue
			}
			metricSeen[metric] = struct{}{}
			metrics = append(metrics, metric)
		}
	}
	records = pruneEmptyFinanceRecords(records)
	sort.Slice(records, func(i, j int) bool { return records[i].Period < records[j].Period })
	return records, metrics
}

func mergeFinanceTrendRecordsByPeriod(base, supplemental []financeTrendRecord, baseMetrics, supplementalMetrics []string) ([]financeTrendRecord, []string) {
	if len(base) == 0 {
		return supplemental, supplementalMetrics
	}
	supplementalByPeriod := make(map[string]financeTrendRecord, len(supplemental))
	for _, record := range supplemental {
		supplementalByPeriod[record.Period] = record
	}
	merged := append([]financeTrendRecord(nil), base...)
	for i := range merged {
		other, ok := supplementalByPeriod[merged[i].Period]
		if !ok {
			continue
		}
		fillMissingFinanceTrendFields(&merged[i], other)
	}
	return merged, mergeMetricKeys(baseMetrics, supplementalMetrics)
}

func fillMissingFinanceTrendFields(dst *financeTrendRecord, src financeTrendRecord) {
	if dst.Revenue == nil {
		dst.Revenue = src.Revenue
	}
	if dst.NetProfit == nil {
		dst.NetProfit = src.NetProfit
	}
	if dst.GrossMargin == nil {
		dst.GrossMargin = src.GrossMargin
	}
	if dst.NetMargin == nil {
		dst.NetMargin = src.NetMargin
	}
	if dst.ROE == nil {
		dst.ROE = src.ROE
	}
	if dst.EPS == nil {
		dst.EPS = src.EPS
	}
	if dst.OperatingCashPS == nil {
		dst.OperatingCashPS = src.OperatingCashPS
	}
}

func mergeMetricKeys(groups ...[]string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, group := range groups {
		for _, metric := range group {
			if _, ok := seen[metric]; ok {
				continue
			}
			seen[metric] = struct{}{}
			result = append(result, metric)
		}
	}
	return result
}

func parseQuarterFinanceTable(lines []string) ([]financeTrendRecord, []string) {
	rows := extractBoxTableRows(lines)
	if len(rows) < 3 {
		return nil, nil
	}
	headers := rows[0]
	if len(headers) < 2 {
		return nil, nil
	}
	periods := make([]string, 0, len(headers)-1)
	for _, header := range headers[1:] {
		period := strings.TrimSpace(header)
		if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(period) {
			continue
		}
		periods = append(periods, period)
	}
	if len(periods) == 0 {
		return nil, nil
	}
	records := make([]financeTrendRecord, len(periods))
	for i, period := range periods {
		records[i] = financeTrendRecord{
			Period:  period,
			Year:    parseYear(period),
			Quarter: quarterLabel(period),
			Label:   quarterLabel(period),
		}
	}
	metrics := make([]string, 0, 5)
	metricSeen := map[string]struct{}{}
	for _, row := range mergeWrappedTableRows(rows[1:]) {
		if len(row) < len(periods)+1 {
			continue
		}
		name := sanitizeFinanceMetricName(row[0])
		values := row[1:]
		assignFinanceMetricValues(records, name, values)
		for _, metric := range financeMetricKeysForName(name) {
			if _, ok := metricSeen[metric]; ok {
				continue
			}
			metricSeen[metric] = struct{}{}
			metrics = append(metrics, metric)
		}
	}
	return pruneEmptyFinanceRecords(records), metrics
}

func parseYearFinanceTable(lines []string) ([]financeTrendRecord, []string) {
	rows := extractBoxTableRows(lines)
	if len(rows) < 3 {
		return nil, nil
	}
	headers := rows[0]
	if len(headers) < 2 {
		return nil, nil
	}
	labels := headers[1:]
	indices := map[string]int{}
	for idx, label := range labels {
		indices[strings.TrimSpace(label)] = idx + 1
	}
	if _, ok := indices["年度"]; !ok {
		return nil, nil
	}
	metrics := []string{"eps"}
	records := make([]financeTrendRecord, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) <= indices["年度"] {
			continue
		}
		year := parseYear(strings.TrimSpace(row[0]))
		if year == 0 {
			continue
		}
		record := financeTrendRecord{
			Period:  fmt.Sprintf("%04d-12-31", year),
			Year:    year,
			Quarter: "年度",
			Label:   fmt.Sprintf("%d年度", year),
			EPS:     parseOptionalFloat(cellAt(row, indices["年度"])),
		}
		records = append(records, record)
	}
	return records, metrics
}

func extractBoxTableRows(lines []string) [][]string {
	rows := make([][]string, 0)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(rows) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "└") && len(rows) > 0 {
			break
		}
		if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "├") {
			continue
		}
		if !strings.HasPrefix(line, "│") {
			if len(rows) > 0 {
				break
			}
			continue
		}
		cells := parseBoxTableLine(line)
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	return rows
}

func parseBoxTableLine(line string) []string {
	parts := strings.Split(line, "│")
	cells := make([]string, 0, len(parts))
	for i := 1; i < len(parts)-1; i++ {
		cells = append(cells, strings.TrimSpace(parts[i]))
	}
	return cells
}

func mergeWrappedTableRows(rows [][]string) [][]string {
	merged := make([][]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		first := strings.TrimSpace(row[0])
		isContinuation := first != "" && !containsAny(first, []string{"每股", "营业", "利润", "毛利", "净利", "收益率", "净资产"})
		if isContinuation && len(merged) > 0 {
			prev := merged[len(merged)-1]
			prev[0] = strings.TrimSpace(prev[0] + first)
			merged[len(merged)-1] = prev
			continue
		}
		copied := append([]string(nil), row...)
		merged = append(merged, copied)
	}
	return merged
}

func sanitizeFinanceMetricName(name string) string {
	name = strings.ReplaceAll(name, " ", "")
	name = strings.ReplaceAll(name, "\t", "")
	name = strings.ReplaceAll(name, "（", "(")
	name = strings.ReplaceAll(name, "）", ")")
	return name
}

func assignFinanceMetricValues(records []financeTrendRecord, name string, values []string) {
	for i := range records {
		if i >= len(values) {
			break
		}
		v := parseOptionalFloat(values[i])
		if v == nil {
			continue
		}
		isGrowthRate := strings.Contains(name, "增长率") || strings.Contains(name, "同比")
		switch {
		case !isGrowthRate && (strings.Contains(name, "营业收入") || strings.Contains(name, "营业总收") || strings.Contains(name, "总营收")):
			records[i].Revenue = v
		case isNetProfitMetricName(name):
			records[i].NetProfit = v
		case strings.Contains(name, "销售毛利率") || strings.Contains(name, "毛利率"):
			records[i].GrossMargin = v
		case strings.Contains(name, "销售净利率") || strings.Contains(name, "净利润率"):
			records[i].NetMargin = v
		case strings.Contains(name, "加权净资产收益率") || strings.Contains(name, "净资产收益率"):
			records[i].ROE = v
		case strings.Contains(name, "每股收益"):
			records[i].EPS = v
		case strings.Contains(name, "每股经营现金流"):
			records[i].OperatingCashPS = v
		}
	}
}

func financeMetricKeysForName(name string) []string {
	isGrowthRate := strings.Contains(name, "增长率") || strings.Contains(name, "同比")
	switch {
	case !isGrowthRate && (strings.Contains(name, "营业收入") || strings.Contains(name, "营业总收") || strings.Contains(name, "总营收")):
		return []string{"revenue"}
	case isNetProfitMetricName(name):
		return []string{"netProfit"}
	case strings.Contains(name, "销售毛利率") || strings.Contains(name, "毛利率"):
		return []string{"grossMargin"}
	case strings.Contains(name, "销售净利率") || strings.Contains(name, "净利润率"):
		return []string{"netMargin"}
	case strings.Contains(name, "加权净资产收益率") || strings.Contains(name, "净资产收益率"):
		return []string{"roe"}
	case strings.Contains(name, "每股收益"):
		return []string{"eps"}
	case strings.Contains(name, "每股经营现金流"):
		return []string{"operatingCashPerShare"}
	default:
		return nil
	}
}

func isNetProfitMetricName(name string) bool {
	if strings.Contains(name, "增长率") || strings.Contains(name, "现金含量") || strings.Contains(name, "净利率") || strings.Contains(name, "净资产") || strings.Contains(name, "总资产") {
		return false
	}
	return strings.Contains(name, "归属母公司净利润") || strings.Contains(name, "归母净利") || strings.HasPrefix(name, "净利润")
}

func aggregateQuarterFinanceRecords(records []financeTrendRecord) []financeTrendRecord {
	byYear := map[int]financeTrendRecord{}
	for _, record := range records {
		if record.Quarter != "Q4" {
			continue
		}
		record.Label = fmt.Sprintf("%d年度", record.Year)
		record.Quarter = "年度"
		byYear[record.Year] = record
	}
	years := make([]int, 0, len(byYear))
	for year := range byYear {
		years = append(years, year)
	}
	sort.Ints(years)
	result := make([]financeTrendRecord, 0, len(years))
	for _, year := range years {
		result = append(result, byYear[year])
	}
	return result
}

func mergeYearFinanceRecords(base, fallback []financeTrendRecord, baseMetrics, fallbackMetrics []string) ([]financeTrendRecord, []string) {
	fallbackByYear := make(map[int]financeTrendRecord, len(fallback))
	for _, record := range fallback {
		fallbackByYear[record.Year] = record
	}
	merged := make([]financeTrendRecord, 0, len(base))
	for _, record := range base {
		if fallbackRecord, ok := fallbackByYear[record.Year]; ok {
			if record.EPS == nil {
				record.EPS = fallbackRecord.EPS
			}
		}
		merged = append(merged, record)
	}
	if len(merged) == 0 {
		return fallback, fallbackMetrics
	}
	metricSeen := map[string]struct{}{}
	metrics := make([]string, 0, len(baseMetrics)+len(fallbackMetrics))
	for _, metric := range append(append([]string(nil), baseMetrics...), fallbackMetrics...) {
		if _, ok := metricSeen[metric]; ok {
			continue
		}
		metricSeen[metric] = struct{}{}
		metrics = append(metrics, metric)
	}
	return merged, metrics
}

func pruneEmptyFinanceRecords(records []financeTrendRecord) []financeTrendRecord {
	result := make([]financeTrendRecord, 0, len(records))
	for _, record := range records {
		if record.Revenue == nil && record.NetProfit == nil && record.GrossMargin == nil && record.NetMargin == nil && record.ROE == nil && record.EPS == nil && record.OperatingCashPS == nil {
			continue
		}
		result = append(result, record)
	}
	return result
}

func parseOptionalFloat(value string) *float64 {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if trimmed == "" || trimmed == "---" || trimmed == "--" || trimmed == "-" {
		return nil
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseYear(value string) int {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 4 {
		parsed, err := strconv.Atoi(trimmed[:4])
		if err == nil {
			return parsed
		}
	}
	return 0
}

func quarterLabel(period string) string {
	switch {
	case strings.HasSuffix(period, "03-31"):
		return "Q1"
	case strings.HasSuffix(period, "06-30"):
		return "Q2"
	case strings.HasSuffix(period, "09-30"):
		return "Q3"
	case strings.HasSuffix(period, "12-31"):
		return "Q4"
	default:
		return period
	}
}

func cellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
