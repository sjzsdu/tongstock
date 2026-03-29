package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

	client, err := tdx.DialHosts(cfg.TDX.Hosts)
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
	r.GET("/api/index", handleIndex)
	r.GET("/api/company", handleCompany)
	r.GET("/api/company/content", handleCompanyContent)
	r.GET("/api/block", handleBlock)
	r.GET("/api/block/files", handleBlockFiles)
	r.GET("/api/block/list", handleBlockList)
	r.GET("/api/block/show", handleBlockShow)

	r.GET("/api/count", handleCount)
	r.GET("/api/auction", handleAuction)

	r.GET("/api/indicator", handleIndicator)
	r.GET("/api/screen", handleScreen)
	r.GET("/api/signal-analysis", handleSignalAnalysis)

	r.GET("/api/history", handleHistoryGet)
	r.POST("/api/history", handleHistoryPost)
	r.DELETE("/api/history/:code", handleHistoryDelete)

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
	client, err := tdx.DialHosts(config.Get().TDX.Hosts)
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
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
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
	code := c.Query("code")
	ktype := c.Query("type")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

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
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
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
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
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
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
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
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
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
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
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
	if codesSZ != nil {
		for _, c := range codesSZ {
			codeNameMap[c.Code] = c.Name
		}
	}

	codesSH, _ := svc.FetchCodes(protocol.ExchangeSH)
	if codesSH != nil {
		for _, c := range codesSH {
			codeNameMap[c.Code] = c.Name
		}
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
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
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
	code := c.Query("code")
	ktype := c.DefaultQuery("type", "day")
	daysStr := c.DefaultQuery("days", "0")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "无K线数据"})
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
	code := c.Query("code")
	ktype := c.DefaultQuery("type", "day")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "K线数据不足"})
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

func prevDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

func handleHistoryGet(c *gin.Context) {
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库未初始化"})
		return
	}
	stocks, err := history.GetAll(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stocks})
}

func handleHistoryPost(c *gin.Context) {
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库未初始化"})
		return
	}
	var stock history.HistoryStock
	if err := c.ShouldBindJSON(&stock); err != nil {
		log.Printf("history bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("history add: %+v", stock)
	stock.AnalyzedAt = time.Now()
	if err := history.Upsert(db, stock); err != nil {
		log.Printf("history upsert error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func handleHistoryDelete(c *gin.Context) {
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库未初始化"})
		return
	}
	code := c.Param("code")
	if err := history.Delete(db, code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
