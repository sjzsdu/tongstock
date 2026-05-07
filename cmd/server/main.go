package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	pinyin "github.com/mozillazg/go-pinyin"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/sjzsdu/tongstock/pkg/utils"
	webstatic "github.com/sjzsdu/tongstock/pkg/web"
)

var svc *tdx.Service
var tdxMu sync.Mutex
var db *history.DB

const (
	stockSearchDefaultLimit = 10
	stockSearchMaxLimit     = 20
	stockSearchIndexTTL     = 10 * time.Minute
)

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

type indicatorParamPayload struct {
	Defaults   indicatorCategoryPayload            `json:"defaults" yaml:"defaults"`
	Categories map[string]indicatorCategoryPayload `json:"categories,omitempty" yaml:"categories,omitempty"`
	Overrides  map[string]indicatorCategoryPayload `json:"overrides,omitempty" yaml:"overrides,omitempty"`
	Path       string                              `json:"path,omitempty"`
}

type indicatorCategoryPayload struct {
	MA   []int          `json:"ma,omitempty" yaml:"ma,omitempty"`
	MACD *ta.MACDConfig `json:"macd,omitempty" yaml:"macd,omitempty"`
	KDJ  *ta.KDJConfig  `json:"kdj,omitempty" yaml:"kdj,omitempty"`
	BOLL *ta.BOLLConfig `json:"boll,omitempty" yaml:"boll,omitempty"`
	RSI  []int          `json:"rsi,omitempty" yaml:"rsi,omitempty"`
}

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

type scoredStockMatch struct {
	stockSearchMatch
	Score int
}

var stockSearchIndexCache struct {
	sync.RWMutex
	builtAt time.Time
	items   []stockSearchIndexItem
}

func handleStockSearch(c *gin.Context) {
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

	matches, resolved, exact, err := searchStockMatches(query, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stockSearchResponse{Query: query, Total: len(matches), Exact: exact, Resolved: resolved, Matches: matches})
}

func handleStockSearchIndex(c *gin.Context) {
	items, err := getStockSearchIndex()
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
	stockSearchIndexCache.RLock()
	updatedAt := stockSearchIndexCache.builtAt.UnixMilli()
	stockSearchIndexCache.RUnlock()
	c.JSON(http.StatusOK, stockSearchIndexResponse{UpdatedAt: updatedAt, Total: len(entries), Items: entries})
}

func resolveStockCodeOrRespond(c *gin.Context, raw string) (string, bool) {
	query := strings.TrimSpace(raw)
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return "", false
	}

	matches, resolved, _, err := searchStockMatches(query, stockSearchDefaultLimit)
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

func searchStockMatches(query string, limit int) ([]stockSearchMatch, bool, bool, error) {
	if limit <= 0 {
		limit = stockSearchDefaultLimit
	}
	if limit > stockSearchMaxLimit {
		limit = stockSearchMaxLimit
	}

	items, err := getStockSearchIndex()
	if err != nil {
		return nil, false, false, err
	}

	normalizedQuery := normalizeStockSearchText(query)
	normalizedCode := normalizeStockCodeQuery(query)
	if normalizedQuery == "" && normalizedCode == "" {
		return []stockSearchMatch{}, false, false, nil
	}

	matches := make([]scoredStockMatch, 0, limit)
	for _, item := range items {
		score, matchType, ok := scoreStockMatch(item, normalizedQuery, normalizedCode)
		if !ok {
			continue
		}
		matches = append(matches, scoredStockMatch{stockSearchMatch: stockSearchMatch{Code: item.Code, Name: item.Name, Exchange: item.Exchange, MatchType: matchType}, Score: score})
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

func scoreStockMatch(item stockSearchIndexItem, normalizedQuery, normalizedCode string) (int, string, bool) {
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

func getStockSearchIndex() ([]stockSearchIndexItem, error) {
	stockSearchIndexCache.RLock()
	if time.Since(stockSearchIndexCache.builtAt) < stockSearchIndexTTL && len(stockSearchIndexCache.items) > 0 {
		items := stockSearchIndexCache.items
		stockSearchIndexCache.RUnlock()
		return items, nil
	}
	stockSearchIndexCache.RUnlock()

	stockSearchIndexCache.Lock()
	defer stockSearchIndexCache.Unlock()
	if time.Since(stockSearchIndexCache.builtAt) < stockSearchIndexTTL && len(stockSearchIndexCache.items) > 0 {
		return stockSearchIndexCache.items, nil
	}

	s, err := getService()
	if err != nil {
		return nil, err
	}
	sources := []struct {
		exchange protocol.Exchange
		label    string
	}{{protocol.ExchangeSH, "上交所"}, {protocol.ExchangeSZ, "深交所"}, {protocol.ExchangeBJ, "北交所"}}

	items := make([]stockSearchIndexItem, 0, 6000)
	for _, source := range sources {
		codes, err := s.FetchCodes(source.exchange)
		if err != nil {
			return nil, err
		}
		for _, code := range codes {
			item := stockSearchIndexItem{Code: code.Code, Name: code.Name, Exchange: source.label}
			item.NameNorm = normalizeStockSearchText(item.Name)
			item.PinyinNorm, item.Initials = buildStockPinyinKeys(item.Name)
			items = append(items, item)
		}
	}
	stockSearchIndexCache.items = items
	stockSearchIndexCache.builtAt = time.Now()
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

func main() {
	port := flag.Int("port", 0, "服务端口 (默认从配置文件读取)")
	flag.Usage = func() {
		fmt.Println("TongStock Server - 通达信股票数据 HTTP API 服务")
		fmt.Println()
		fmt.Println("用法: tongstock-server [选项]")
		fmt.Println()
		fmt.Println("选项:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  tongstock-server              # 启动服务 (默认端口 8080)")
		fmt.Println("  tongstock-server --port 9090  # 指定端口")
		fmt.Println("  浏览器访问 http://localhost:8080")
	}
	flag.Parse()

	for _, arg := range os.Args[1:] {
		if arg == "--help" || arg == "-h" || arg == "-help" {
			flag.Usage()
			os.Exit(0)
		}
	}

	if err := config.Init(); err != nil {
		log.Printf("加载配置失败: %v, 使用默认配置", err)
	}
	cfg := config.Get()

	var err error
	if err = config.EnsureHomeDir(); err != nil {
		log.Printf("创建缓存目录失败: %v", err)
	}
	db, err = history.Open(config.DBPath())
	if err != nil {
		log.Printf("打开数据库失败: %v", err)
	} else {
		if err := history.InitTable(db); err != nil {
			log.Printf("初始化历史表失败: %v", err)
		}
	}

	if *port > 0 {
		cfg.Server.Port = *port
	}

	client, err := tdx.DialHosts(cfg.TDX.Hosts, tdx.WithRedial(true))
	if err != nil {
		log.Printf("连接服务器失败: %v, 将在请求时重连", err)
	} else {
		svc, err = tdx.NewService(client)
		if err != nil {
			log.Printf("初始化服务失败: %v", err)
		}
	}

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/api/quote", handleQuote)
	r.GET("/api/kline", handleKline)
	r.GET("/api/codes", handleCodes)
	r.GET("/api/minute", handleMinute)
	r.GET("/api/trade", handleTrade)
	r.GET("/api/xdxr", handleXdXr)
	r.GET("/api/finance", handleFinance)
	r.GET("/api/finance/trends", handleFinanceTrends)
	r.GET("/api/finance/metrics", handleFinanceMetrics)
	r.GET("/api/index", handleIndex)
	r.GET("/api/company", handleCompany)
	r.GET("/api/company/content", handleCompanyContent)
	r.GET("/api/stocks/search", handleStockSearch)
	r.GET("/api/stocks/search-index", handleStockSearchIndex)
	r.GET("/api/block", handleBlock)
	r.GET("/api/block/files", handleBlockFiles)
	r.GET("/api/block/list", handleBlockList)
	r.GET("/api/block/show", handleBlockShow)

	r.GET("/api/count", handleCount)
	r.GET("/api/auction", handleAuction)

	r.GET("/api/indicator", handleIndicator)
	r.GET("/api/screen", handleScreen)
	r.GET("/api/signal-analysis", handleSignalAnalysis)
	r.POST("/api/sync/daily", handleSyncDaily)

	r.GET("/api/history", handleHistoryGet)
	r.POST("/api/history", handleHistoryPost)
	r.DELETE("/api/history/:code", handleHistoryDelete)
	r.GET("/api/watchlist", handleHistoryGet)
	r.POST("/api/watchlist", handleHistoryPost)
	r.DELETE("/api/watchlist/:code", handleHistoryDelete)

	r.GET("/api/settings/indicator", handleIndicatorSettingsGet)
	r.PUT("/api/settings/indicator", handleIndicatorSettingsPut)

	dist := webstatic.DistFileServer()
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" || path == "/index.html" {
			dist.ServeHTTP(c.Writer, c.Request)
			return
		}
		if webstatic.Exists(path[1:]) {
			dist.ServeHTTP(c.Writer, c.Request)
			return
		}
		c.Request.URL.Path = "/"
		dist.ServeHTTP(c.Writer, c.Request)
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("服务启动于 http://localhost:%d", cfg.Server.Port)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func getService() (*tdx.Service, error) {
	if svc != nil {
		return svc, nil
	}
	client, err := tdx.DialHosts(config.Get().TDX.Hosts, tdx.WithRedial(true))
	if err != nil {
		log.Printf("[tdx] 连接失败: %v", err)
		return nil, err
	}
	var s *tdx.Service
	s, err = tdx.NewService(client)
	if err != nil {
		log.Printf("[tdx] 初始化失败: %v", err)
		return nil, err
	}
	log.Printf("[tdx] 连接成功")
	svc = s
	return svc, nil
}

func resetService() {
	if svc != nil {
		svc.Close()
		svc = nil
	}
}

func withRetry[T any](fn func() (T, error)) (T, error) {
	tdxMu.Lock()
	result, err := fn()
	tdxMu.Unlock()

	if err != nil {
		log.Printf("[tdx] 请求失败, 尝试重连: %v", err)
		resetService()
		tdxMu.Lock()
		defer tdxMu.Unlock()
		return fn()
	}
	return result, nil
}

func handleQuote(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	quotes, err := withRetry(func() ([]*protocol.QuoteItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.Client.GetQuote(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取行情失败: %v", err)})
		return
	}

	if len(quotes) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该股票"})
		return
	}

	c.JSON(http.StatusOK, quotes[0])
}

func handleKline(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}
	ktype := c.Query("type")

	klineType := tdx.ParseKlineType(ktype)

	klines, err := withRetry(func() ([]*protocol.Kline, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchKline(code, klineType, 0, 250)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, klines)
}

func handleCodes(c *gin.Context) {
	exchangeStr := c.DefaultQuery("exchange", "sz")
	exchange := protocol.ParseExchange(exchangeStr)

	codes, err := withRetry(func() ([]*protocol.CodeItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchCodes(exchange)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取代码失败: %v", err)})
		return
	}

	stocksOnly := c.DefaultQuery("stocks_only", "false") == "true"
	if stocksOnly {
		filtered := make([]*protocol.CodeItem, 0, len(codes))
		for _, item := range codes {
			if utils.IsStock(item.Code) {
				filtered = append(filtered, item)
			}
		}
		codes = filtered
	}

	c.JSON(http.StatusOK, codes)
}

func handleMinute(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	date := c.Query("date")

	var resp *protocol.MinuteResp
	resp, err := withRetry(func() (*protocol.MinuteResp, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		if date != "" {
			return s.Client.GetHistoryMinute(date, code)
		}
		return s.Client.GetMinute(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取分时数据失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func handleCount(c *gin.Context) {
	exchangeStr := c.DefaultQuery("exchange", "sz")
	exchange := protocol.ParseExchange(exchangeStr)

	count, err := withRetry(func() (int, error) {
		s, e := getService()
		if e != nil {
			return 0, e
		}
		return s.Client.GetSecurityCount(exchange)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取证券数量失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"exchange": exchangeStr, "count": count})
}

func handleAuction(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	resp, err := withRetry(func() (*protocol.CallAuctionResp, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.Client.GetCallAuction(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取集合竞价数据失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func handleXdXr(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	items, err := withRetry(func() ([]*protocol.XdXrItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchXdXr(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取除权除息失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, items)
}

func handleFinance(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	info, err := withRetry(func() (*protocol.FinanceInfo, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchFinance(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取财务数据失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, info)
}

func handleFinanceTrends(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(c.DefaultQuery("mode", "quarter")))
	if mode != "quarter" && mode != "year" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode 仅支持 quarter 或 year"})
		return
	}

	content, err := fetchCompanyBlockContent(code, "财务分析")
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

func handleFinanceMetrics(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}
	content, err := fetchCompanyBlockContent(code, "财务分析")
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

func handleIndex(c *gin.Context) {
	code := c.Query("code")
	ktype := c.Query("type")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

	klineType := tdx.ParseKlineType(ktype)

	bars, err := withRetry(func() ([]*protocol.IndexBar, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.Client.GetIndexBars(code, klineType, 0, 100)
	})
	if err != nil {
		log.Printf("[index] GetIndexBars %s failed: %v", code, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取指数K线失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, bars)
}

func handleCompany(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	cats, err := withRetry(func() ([]*protocol.CompanyCategoryItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchCompanyCategory(code)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, cats)
}

func handleCompanyContent(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	block := c.Query("block")
	filename := c.Query("filename")

	start := uint32(0)
	length := uint32(10000)

	if block != "" {
		cats, err := withRetry(func() ([]struct {
			Filename string
			Name     string
			Start    uint32
			Length   uint32
		}, error) {
			s, e := getService()
			if e != nil {
				return nil, e
			}
			raw, e2 := s.FetchCompanyCategory(code)
			if e2 != nil {
				return nil, e2
			}
			var result []struct {
				Filename string
				Name     string
				Start    uint32
				Length   uint32
			}
			for _, cat := range raw {
				result = append(result, struct {
					Filename string
					Name     string
					Start    uint32
					Length   uint32
				}{cat.Filename, cat.Name, cat.Start, cat.Length})
			}
			return result, nil
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
		if s := c.Query("start"); s != "" {
			if v, err := strconv.ParseUint(s, 10, 32); err == nil {
				start = uint32(v)
			}
		}
		if l := c.Query("length"); l != "" {
			if v, err := strconv.ParseUint(l, 10, 32); err == nil {
				length = uint32(v)
			}
		}
	}

	content, err := withRetry(func() (string, error) {
		s, e := getService()
		if e != nil {
			return "", e
		}
		return s.Client.GetCompanyInfoContent(code, filename, start, length)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": content})
}

func handleBlock(c *gin.Context) {
	blockFile := c.DefaultQuery("file", "block_zs.dat")
	stocksOnly := c.DefaultQuery("stocks_only", "false") == "true"

	items, err := withRetry(func() ([]*protocol.BlockItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchBlock(blockFile)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取板块信息失败: %v", err)})
		return
	}

	if stocksOnly {
		filtered := make([]*protocol.BlockItem, 0, len(items))
		for _, item := range items {
			if utils.IsStock(item.StockCode) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	c.JSON(http.StatusOK, items)
}

// handleBlockFiles 返回所有可用的板块文件列表
func handleBlockFiles(c *gin.Context) {
	files := []struct {
		File string `json:"file"`
		Name string `json:"name"`
		Desc string `json:"desc"`
	}{
		{"block.dat", "综合板块", "综合分类"},
		{"block_zs.dat", "指数板块", "主要指数成分股"},
		{"block_fg.dat", "行业板块", "行业分类"},
		{"block_gn.dat", "概念板块", "概念主题"},
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
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

// handleBlockList 返回结构化的板块列表
func handleBlockList(c *gin.Context) {
	blockFile := c.DefaultQuery("file", "block_zs.dat")
	blockType := c.Query("type")
	sortByCount := c.Query("sort") == "true"

	items, err := withRetry(func() ([]*protocol.BlockItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchBlock(blockFile)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取板块信息失败: %v", err)})
		return
	}

	blockMap := serverBlockStats(items)

	// 构建板块列表并过滤
	type blockInfo struct {
		BlockType uint16   `json:"type"`
		Name      string   `json:"name"`
		Count     int      `json:"count"`
		Stocks    []string `json:"stocks"`
	}
	var blocks []blockInfo
	for name, stats := range blockMap {
		if !isValidBlockNameServer(name) {
			continue
		}
		if blockType != "" && fmt.Sprintf("%d", stats.blockType) != blockType {
			continue
		}
		blocks = append(blocks, blockInfo{
			BlockType: stats.blockType,
			Name:      name,
			Count:     len(stats.stockCodes),
			Stocks:    stats.stockCodes,
		})
	}

	// 排序
	if sortByCount {
		sort.Slice(blocks, func(i, j int) bool {
			return blocks[i].Count > blocks[j].Count
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"file":   blockFile,
		"total":  len(blocks),
		"blocks": blocks,
	})
}

// getCodeNameMapServer 获取股票代码到名称的映射
func getCodeNameMapServer() map[string]string {
	codeNameMap := make(map[string]string)

	svc, err := getService()
	if err != nil {
		return codeNameMap
	}

	codesSZ, _ := svc.FetchCodes(protocol.ExchangeSZ)
	for _, c := range codesSZ {
		codeNameMap[c.Code] = c.Name
	}

	codesSH, _ := svc.FetchCodes(protocol.ExchangeSH)
	for _, c := range codesSH {
		codeNameMap[c.Code] = c.Name
	}

	return codeNameMap
}

// handleBlockShow 返回指定板块的成分股
func handleBlockShow(c *gin.Context) {
	blockFile := c.DefaultQuery("file", "block_zs.dat")
	blockName := c.Query("name")
	code := c.Query("code") // 根据股票代码查询所属板块

	items, err := withRetry(func() ([]*protocol.BlockItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchBlock(blockFile)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取板块信息失败: %v", err)})
		return
	}

	// 根据股票代码查询所属板块
	if code != "" {
		type blockResult struct {
			Name  string `json:"name"`
			Type  uint16 `json:"type"`
			Count int    `json:"count"`
		}
		var results []blockResult

		blockMap := serverBlockStats(items)
		for name, stats := range blockMap {
			if !isValidBlockNameServer(name) {
				continue
			}
			for _, stockCode := range stats.stockCodes {
				if stockCode == code {
					results = append(results, blockResult{
						Name:  name,
						Type:  stats.blockType,
						Count: len(stats.stockCodes),
					})
					break
				}
			}
		}

		if len(results) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("未找到股票 %s 所属的板块", code)})
			return
		}

		// 获取股票名称
		codeNameMap := getCodeNameMapServer()
		stockName := codeNameMap[code]
		if stockName == "" {
			stockName = "未知"
		}

		c.JSON(http.StatusOK, gin.H{
			"code":   code,
			"name":   stockName,
			"blocks": results,
		})
		return
	}

	// 根据板块名称查询成分股
	if blockName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 name 或 code 参数"})
		return
	}

	blockMap := serverBlockStats(items)

	// 模糊匹配
	var matchedBlocks []struct {
		name      string
		blockType uint16
		stocks    []string
	}
	for name, stats := range blockMap {
		if !isValidBlockNameServer(name) {
			continue
		}
		if strings.Contains(name, blockName) || name == blockName {
			matchedBlocks = append(matchedBlocks, struct {
				name      string
				blockType uint16
				stocks    []string
			}{name: name, blockType: stats.blockType, stocks: stats.stockCodes})
		}
	}

	if len(matchedBlocks) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("未找到板块: %s", blockName)})
		return
	}

	if len(matchedBlocks) > 1 {
		c.JSON(http.StatusOK, gin.H{
			"message": "找到多个匹配的板块",
			"blocks":  matchedBlocks,
		})
		return
	}

	block := matchedBlocks[0]
	stocks := block.stocks

	// 获取股票名称
	codeNameMap := getCodeNameMapServer()

	type stockInfo struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Exchange string `json:"exchange"`
	}
	var stockList []stockInfo
	for _, stockCode := range stocks {
		name := codeNameMap[stockCode]
		if name == "" {
			name = "未知"
		}
		exchange := "未知"
		if len(stockCode) >= 1 {
			switch stockCode[0] {
			case '0', '3':
				exchange = "深交所"
			case '6':
				exchange = "上交所"
			case '8', '9':
				exchange = "北交所"
			}
		}
		stockList = append(stockList, stockInfo{
			Code:     stockCode,
			Name:     name,
			Exchange: exchange,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"name":   block.name,
		"type":   block.blockType,
		"count":  len(stocks),
		"stocks": stockList,
	})
}

func handleTrade(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}

	start := uint16(0)
	count := uint16(100)
	date := c.Query("date")
	history := c.Query("history") == "true"

	resp, err := withRetry(func() (*protocol.TradeResp, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		if history && date != "" {
			return s.Client.GetHistoryMinuteTrade(date, code, start, count)
		}
		return s.Client.GetMinuteTrade(code, start, count)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取分笔数据失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func toKlineInputs(klines []*protocol.Kline) []ta.KlineInput {
	inputs := make([]ta.KlineInput, len(klines))
	for i, k := range klines {
		inputs[i] = ta.KlineInput{
			Time: k.Time, Open: k.Open, High: k.High,
			Low: k.Low, Close: k.Close, Volume: k.Volume, Amount: k.Amount,
		}
	}
	return inputs
}

func handleIndicator(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}
	ktype := c.DefaultQuery("type", "day")
	daysStr := c.DefaultQuery("days", "0")

	klines, err := withRetry(func() ([]*protocol.Kline, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchKlineAll(code, tdx.ParseKlineType(ktype))
	})
	if err != nil {
		log.Printf("[indicator] FetchKline %s failed: %v", code, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线失败: %v", err)})
		return
	}

	inputs := toKlineInputs(klines)
	if len(inputs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该股票的K线数据"})
		return
	}
	_ = param.AutoInit()
	category := param.DetectCategory(code)
	cfg := param.Resolve(code, category)

	var result *ta.IndicatorResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[indicator] panic: %v", r)
				result = nil
			}
		}()
		result = ta.Calculate(inputs, cfg)
	}()
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "指标计算失败"})
		return
	}
	signals := signal.Detect(code, inputs, result, nil)

	days := 0
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	response := gin.H{
		"code":         code,
		"type":         ktype,
		"category":     string(category),
		"count":        len(inputs),
		"ma":           result.MA,
		"macd":         result.MACD,
		"kdj":          result.KDJ,
		"boll":         result.BOLL,
		"rsi":          result.RSI,
		"volume_ratio": result.VolumeRatio,
		"signals":      signals,
	}

	if days > 0 && days < len(inputs) {
		start := len(inputs) - days
		response["days"] = days
		response["klines"] = inputs[start:]
		response["last"] = inputs[len(inputs)-1]
	} else {
		response["klines"] = inputs
		response["last"] = inputs[len(inputs)-1]
	}

	c.JSON(http.StatusOK, response)
}

func handleScreen(c *gin.Context) {
	codesStr := c.Query("codes")
	ktype := c.DefaultQuery("type", "day")
	signalType := c.Query("signal")

	if codesStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 codes 参数"})
		return
	}

	codeList := strings.Split(codesStr, ",")
	var filteredCodes []string
	for _, c := range codeList {
		c = strings.TrimSpace(c)
		if c != "" {
			filteredCodes = append(filteredCodes, c)
		}
	}
	codeList = filteredCodes

	if len(codeList) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的股票代码"})
		return
	}

	_ = param.AutoInit()

	// 预先加载股票代码到名称的映射
	// 注意：先加载SH，再加载SZ和BJ，让股票交易所的名称覆盖指数交易所（避免000001被SH的上证指数覆盖）
	codeNameMap := make(map[string]string)
	if s, err := getService(); err == nil {
		if codes, err := s.FetchCodes(protocol.ExchangeSH); err == nil {
			for _, c := range codes {
				codeNameMap[c.Code] = c.Name
			}
		}
		if codes, err := s.FetchCodes(protocol.ExchangeBJ); err == nil {
			for _, c := range codes {
				codeNameMap[c.Code] = c.Name
			}
		}
		if codes, err := s.FetchCodes(protocol.ExchangeSZ); err == nil {
			for _, c := range codes {
				codeNameMap[c.Code] = c.Name
			}
		}
	}

	type result struct {
		Code    string               `json:"code"`
		Name    string               `json:"name"`
		Last    ta.KlineInput        `json:"last"`
		MA      map[string][]float64 `json:"ma"`
		MACD    *ta.MACDResult       `json:"macd,omitempty"`
		KDJ     *ta.KDJResult        `json:"kdj,omitempty"`
		Signals []signal.Signal      `json:"signals"`
		Cycles  []signal.TradeCycle  `json:"cycles"` // 完整的交易周期
	}

	results := make([]result, len(codeList))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	var mu sync.Mutex

	for i, code := range codeList {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, c string) {
			defer wg.Done()
			defer func() { <-sem }()

			// 从预加载的映射中获取股票名称
			stockName := codeNameMap[c]

			tdxMu.Lock()
			s, e := getService()
			var klines []*protocol.Kline
			if e == nil {
				// 使用全量历史数据来检测完整的交易周期
				klines, e = s.FetchKlineAll(c, tdx.ParseKlineType(ktype))
			}
			tdxMu.Unlock()
			if e != nil {
				return
			}
			if len(klines) == 0 {
				return
			}

			inputs := toKlineInputs(klines)
			cat := param.DetectCategory(c)
			cfg := param.Resolve(c, cat)
			ind := ta.Calculate(inputs, cfg)
			sigs := signal.Detect(c, inputs, ind, nil)

			// 使用全量历史数据计算完整的交易周期
			cycles := signal.DetectAllCycles(c, inputs, ind)

			n := len(inputs)
			r := result{
				Code:    c,
				Name:    stockName,
				Last:    inputs[n-1],
				MA:      ind.MA,
				MACD:    ind.MACD,
				KDJ:     ind.KDJ,
				Signals: sigs,
				Cycles:  cycles,
			}

			mu.Lock()
			results[idx] = r
			mu.Unlock()
		}(i, code)
	}
	wg.Wait()

	// 过滤掉没有有效数据的股票（没有代码或没有名称）
	var validResults []result
	for _, r := range results {
		if r.Code != "" && r.Name != "" {
			validResults = append(validResults, r)
		}
	}

	// 买入信号类型
	buySignalTypes := map[signal.SignalType]bool{
		signal.SignalGoldenCross: true, // 金叉
		signal.SignalOversold:    true, // 超卖
		signal.SignalBreakLower:  true, // 跌破下轨
		signal.SignalBullAlign:   true, // 多头排列
	}

	// 只返回当前处于买入信号状态的个股
	var buySignalResults []result
	for _, r := range validResults {
		// 检查最近的信号是否是买入信号
		if len(r.Signals) > 0 {
			// 获取最新的信号
			latestSignal := r.Signals[len(r.Signals)-1]
			if buySignalTypes[latestSignal.Type] {
				// 只保留最新的买入信号
				r.Signals = []signal.Signal{latestSignal}
				buySignalResults = append(buySignalResults, r)
			}
		}
	}

	if signalType != "" {
		var filtered []result
		for _, r := range validResults {
			if r.Code == "" {
				continue
			}
			for _, s := range r.Signals {
				match := false
				switch signalType {
				case "golden_cross":
					match = s.Type == signal.SignalGoldenCross
				case "death_cross":
					match = s.Type == signal.SignalDeathCross
				case "overbought":
					match = s.Type == signal.SignalOverbought
				case "oversold":
					match = s.Type == signal.SignalOversold
				}
				if match {
					filtered = append(filtered, r)
					break
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"results": filtered, "total": len(codeList), "matched": len(filtered)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": buySignalResults, "total": len(codeList), "matched": len(buySignalResults)})
}

func handleSignalAnalysis(c *gin.Context) {
	code, ok := resolveStockCodeOrRespond(c, c.Query("code"))
	if !ok {
		return
	}
	ktype := c.DefaultQuery("type", "day")

	klines, err := withRetry(func() ([]*protocol.Kline, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchKline(code, tdx.ParseKlineType(ktype), 0, 500)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线失败: %v", err)})
		return
	}
	if len(klines) < 30 {
		c.JSON(http.StatusNotFound, gin.H{"error": "该股票K线数据不足，暂不支持信号分析"})
		return
	}

	inputs := toKlineInputs(klines)
	_ = param.AutoInit()
	category := param.DetectCategory(code)
	cfg := param.Resolve(code, category)
	result := ta.Calculate(inputs, cfg)
	signals := signal.Detect(code, inputs, result, nil)

	lastState := map[string]string{}
	var dedupedSignals []signal.Signal
	for _, s := range signals {
		dateStr := s.Date.Format("2006-01-02")
		stateTypes := map[signal.SignalType]bool{
			"多头排列": true, "空头排列": true, "超买": true, "超卖": true,
		}
		if stateTypes[s.Type] {
			key := s.Indicator + string(s.Type)
			if lastState[key] == dateStr || lastState[key] == prevDate(dateStr) {
				lastState[key] = dateStr
				continue
			}
			lastState[key] = dateStr
		}
		dedupedSignals = append(dedupedSignals, s)
	}

	type signalOutcome struct {
		Date      string   `json:"date"`
		Type      string   `json:"type"`
		Indicator string   `json:"indicator"`
		Details   string   `json:"details"`
		Price     float64  `json:"price"`
		Chg1      *float64 `json:"chg1"`
		Chg5      *float64 `json:"chg5"`
		Chg10     *float64 `json:"chg10"`
		Chg20     *float64 `json:"chg20"`
		Action    string   `json:"action"`
	}

	buySignals := map[string]bool{
		"金叉": true, "超卖": true, "跌破下轨": true, "多头排列": true,
	}

	var outcomes []signalOutcome
	for _, s := range dedupedSignals {
		idx := -1
		dateStr := s.Date.Format("2006-01-02")
		for j, k := range klines {
			if k.Time.Format("2006-01-02") == dateStr {
				idx = j
				break
			}
		}
		if idx < 0 {
			continue
		}

		action := "卖出参考"
		if buySignals[string(s.Type)] {
			action = "买入参考"
		}

		o := signalOutcome{
			Date:      dateStr,
			Type:      string(s.Type),
			Indicator: s.Indicator,
			Details:   s.Details,
			Price:     klines[idx].Close,
			Action:    action,
		}
		for _, d := range []struct {
			days  int
			field **float64
		}{
			{1, &o.Chg1}, {5, &o.Chg5}, {10, &o.Chg10}, {20, &o.Chg20},
		} {
			target := idx + d.days
			if target < len(klines) && klines[idx].Close > 0 {
				v := (klines[target].Close - klines[idx].Close) / klines[idx].Close * 100
				*d.field = &v
			}
		}
		outcomes = append(outcomes, o)
	}

	type summary struct {
		Type    string  `json:"type"`
		Action  string  `json:"action"`
		Count   int     `json:"count"`
		Valid1  int     `json:"valid1"`
		Valid5  int     `json:"valid5"`
		Valid10 int     `json:"valid10"`
		Valid20 int     `json:"valid20"`
		Win1    float64 `json:"win1"`
		Win5    float64 `json:"win5"`
		Win10   float64 `json:"win10"`
		Win20   float64 `json:"win20"`
		Avg1    float64 `json:"avg1"`
		Avg5    float64 `json:"avg5"`
		Avg10   float64 `json:"avg10"`
		Avg20   float64 `json:"avg20"`
	}

	summaries := map[string]*summary{}
	for _, o := range outcomes {
		s, ok := summaries[o.Type]
		if !ok {
			s = &summary{Type: o.Type, Action: o.Action}
			summaries[o.Type] = s
		}
		s.Count++
		for _, d := range []struct {
			chg   *float64
			valid *int
			win   *float64
			avg   *float64
		}{
			{o.Chg1, &s.Valid1, &s.Win1, &s.Avg1},
			{o.Chg5, &s.Valid5, &s.Win5, &s.Avg5},
			{o.Chg10, &s.Valid10, &s.Win10, &s.Avg10},
			{o.Chg20, &s.Valid20, &s.Win20, &s.Avg20},
		} {
			if d.chg != nil {
				*d.valid++
				if *d.chg > 0 {
					*d.win++
				}
				*d.avg += *d.chg
			}
		}
	}

	var sumList []summary
	for _, s := range summaries {
		if s.Valid1 > 0 {
			s.Win1 = s.Win1 / float64(s.Valid1) * 100
			s.Avg1 /= float64(s.Valid1)
		}
		if s.Valid5 > 0 {
			s.Win5 = s.Win5 / float64(s.Valid5) * 100
			s.Avg5 /= float64(s.Valid5)
		}
		if s.Valid10 > 0 {
			s.Win10 = s.Win10 / float64(s.Valid10) * 100
			s.Avg10 /= float64(s.Valid10)
		}
		if s.Valid20 > 0 {
			s.Win20 = s.Win20 / float64(s.Valid20) * 100
			s.Avg20 /= float64(s.Valid20)
		}
		sumList = append(sumList, *s)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":     code,
		"type":     ktype,
		"count":    len(klines),
		"signals":  len(outcomes),
		"outcomes": outcomes,
		"summary":  sumList,
	})
}

type syncDailyRequest struct {
	Codes       []string `json:"codes"`
	Mode        string   `json:"mode"`
	Concurrency int      `json:"concurrency"`
}

func handleSyncDaily(c *gin.Context) {
	var req syncDailyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	codes := normalizeCodeList(req.Codes)
	if len(codes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 codes"})
		return
	}
	mode := tdx.SyncMode(req.Mode)
	if mode == "" {
		mode = tdx.SyncModeAuto
	}
	s, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("初始化服务失败: %v", err)})
		return
	}
	result := s.SyncDailyKlines(codes, mode, req.Concurrency)
	c.JSON(http.StatusOK, result)
}

func normalizeCodeList(codes []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range codes {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
			code := strings.TrimSpace(part)
			if code == "" || seen[code] {
				continue
			}
			seen[code] = true
			out = append(out, code)
		}
	}
	return out
}

func prevDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

func fetchCompanyBlockContent(code, block string) (string, error) {
	cats, err := withRetry(func() ([]*protocol.CompanyCategoryItem, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchCompanyCategory(code)
	})
	if err != nil {
		return "", err
	}
	for _, cat := range cats {
		if cat.Name != block {
			continue
		}
		return withRetry(func() (string, error) {
			s, e := getService()
			if e != nil {
				return "", e
			}
			return s.FetchCompanyContent(code, cat.Filename, cat.Start, cat.Length)
		})
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

func indicatorCategoryToPayload(src param.CategoryParams) indicatorCategoryPayload {
	return indicatorCategoryPayload{
		MA:   append([]int(nil), src.MA...),
		MACD: cloneMACDConfig(src.MACD),
		KDJ:  cloneKDJConfig(src.KDJ),
		BOLL: cloneBOLLConfig(src.BOLL),
		RSI:  append([]int(nil), src.RSI...),
	}
}

func indicatorConfigToPayload(cfg *param.ParamConfig) indicatorParamPayload {
	payload := indicatorParamPayload{
		Defaults: indicatorCategoryToPayload(cfg.Defaults),
		Path:     config.IndicatorConfigPath(),
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

func payloadCategoryToParam(src indicatorCategoryPayload) param.CategoryParams {
	return param.CategoryParams{
		MA:   append([]int(nil), src.MA...),
		MACD: cloneMACDConfig(src.MACD),
		KDJ:  cloneKDJConfig(src.KDJ),
		BOLL: cloneBOLLConfig(src.BOLL),
		RSI:  append([]int(nil), src.RSI...),
	}
}

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

func handleIndicatorSettingsGet(c *gin.Context) {
	cfg, err := param.GetConfig()
	if err != nil {
		log.Printf("[settings] load indicator config failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("读取配置失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, indicatorConfigToPayload(cfg))
}

func handleIndicatorSettingsPut(c *gin.Context) {
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
		log.Printf("[settings] save indicator config failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存配置失败: %v", err)})
		return
	}
	cfg, err := param.GetConfig()
	if err != nil {
		log.Printf("[settings] reload indicator config failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存后读取配置失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "ok",
		"config":  indicatorConfigToPayload(cfg),
	})
}

func handleHistoryGet(c *gin.Context) {
	if db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库未初始化，请检查服务配置"})
		return
	}
	stocks, err := history.GetAll(db)
	if err != nil {
		log.Printf("[history] GetAll failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("查询失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stocks})
}

func handleHistoryPost(c *gin.Context) {
	if db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库未初始化，请检查服务配置"})
		return
	}
	var stock history.HistoryStock
	if err := c.ShouldBindJSON(&stock); err != nil {
		log.Printf("[history] bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	stock.AnalyzedAt = time.Now()
	if stock.Name == "" {
		matches, resolved, _, err := searchStockMatches(stock.Code, 1)
		if err == nil && resolved && len(matches) == 1 {
			stock.Name = matches[0].Name
		}
	}
	if err := history.Upsert(db, stock); err != nil {
		log.Printf("[history] upsert error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func handleHistoryDelete(c *gin.Context) {
	if db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库未初始化，请检查服务配置"})
		return
	}
	code := c.Param("code")
	if err := history.Delete(db, code); err != nil {
		log.Printf("[history] delete error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
