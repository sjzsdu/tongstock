package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	webstatic "github.com/sjzsdu/tongstock/pkg/web"
)

var svc *tdx.Service
var tdxMu sync.Mutex

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

	r.GET("/api/count", handleCount)
	r.GET("/api/auction", handleAuction)

	r.GET("/api/indicator", handleIndicator)
	r.GET("/api/screen", handleScreen)

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
		return s.FetchKline(code, klineType, 0, 100)
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

	c.JSON(http.StatusOK, codes)
}

func handleMinute(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

	date := c.Query("date")
	history := c.Query("history") == "true"

	var resp *protocol.MinuteResp
	resp, err := withRetry(func() (*protocol.MinuteResp, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		if history && date != "" {
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
	c.JSON(http.StatusOK, items)
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

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

	klines, err := withRetry(func() ([]*protocol.Kline, error) {
		s, e := getService()
		if e != nil {
			return nil, e
		}
		return s.FetchKline(code, tdx.ParseKlineType(ktype), 0, 250)
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

	c.JSON(http.StatusOK, gin.H{
		"code":     code,
		"type":     ktype,
		"category": string(category),
		"count":    len(inputs),
		"last":     inputs[len(inputs)-1],
		"ma":       result.MA,
		"macd":     result.MACD,
		"kdj":      result.KDJ,
		"boll":     result.BOLL,
		"rsi":      result.RSI,
		"signals":  signals,
	})
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
	for i := range codeList {
		codeList[i] = strings.TrimSpace(codeList[i])
	}

	_ = param.AutoInit()

	type result struct {
		Code    string               `json:"code"`
		Last    ta.KlineInput        `json:"last"`
		MA      map[string][]float64 `json:"ma"`
		MACD    *ta.MACDResult       `json:"macd,omitempty"`
		KDJ     *ta.KDJResult        `json:"kdj,omitempty"`
		Signals []signal.Signal      `json:"signals"`
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

			tdxMu.Lock()
			s, e := getService()
			var klines []*protocol.Kline
			if e == nil {
				klines, e = s.FetchKline(c, tdx.ParseKlineType(ktype), 0, 250)
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

			n := len(inputs)
			r := result{
				Code:    c,
				Last:    inputs[n-1],
				MA:      ind.MA,
				MACD:    ind.MACD,
				KDJ:     ind.KDJ,
				Signals: sigs,
			}

			mu.Lock()
			results[idx] = r
			mu.Unlock()
		}(i, code)
	}
	wg.Wait()

	if signalType != "" {
		var filtered []result
		for _, r := range results {
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

	c.JSON(http.StatusOK, gin.H{"results": results, "total": len(codeList)})
}
