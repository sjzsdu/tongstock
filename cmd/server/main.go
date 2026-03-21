package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

var svc *tdx.Service

func main() {
	if err := config.Init(); err != nil {
		log.Printf("加载配置失败: %v, 使用默认配置", err)
	}
	cfg := config.Get()

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
		return nil, err
	}
	var s *tdx.Service
	s, err = tdx.NewService(client)
	if err != nil {
		return nil, err
	}
	svc = s
	return svc, nil
}

func handleQuote(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	quotes, err := svc.Client.GetQuote(code)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	klines, err := svc.FetchKline(code, klineType, 0, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, klines)
}

func handleCodes(c *gin.Context) {
	exchangeStr := c.DefaultQuery("exchange", "sz")
	exchange := protocol.ParseExchange(exchangeStr)

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	codes, err := svc.FetchCodes(exchange)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}
	date := c.Query("date")
	history := c.Query("history") == "true"

	var resp *protocol.MinuteResp
	var err2 error
	if history && date != "" {
		resp, err2 = svc.Client.GetHistoryMinute(date, code)
	} else {
		resp, err2 = svc.Client.GetMinute(code)
	}
	if err2 != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取分时数据失败: %v", err2)})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func handleCount(c *gin.Context) {
	exchangeStr := c.DefaultQuery("exchange", "sz")
	exchange := protocol.ParseExchange(exchangeStr)

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	count, err := svc.Client.GetSecurityCount(exchange)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	resp, err := svc.Client.GetCallAuction(code)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	items, err := svc.FetchXdXr(code)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	info, err := svc.FetchFinance(code)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	bars, err := svc.Client.GetIndexBars(code, klineType, 0, 100)
	if err != nil {
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	cats, err := svc.FetchCompanyCategory(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, cats)
}

func handleCompanyContent(c *gin.Context) {
	code := c.Query("code")
	filename := c.Query("filename")
	if code == "" || filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 或 filename 参数"})
		return
	}

	start := uint32(0)
	length := uint32(10000)

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	content, err := svc.FetchCompanyContent(code, filename, start, length)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": content})
}

func handleBlock(c *gin.Context) {
	blockFile := c.DefaultQuery("file", "block_zs.dat")

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	items, err := svc.FetchBlock(blockFile)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}
	start := uint16(0)
	count := uint16(100)
	date := c.Query("date")
	history := c.Query("history") == "true"

	var resp *protocol.TradeResp
	if history && date != "" {
		resp, err = svc.Client.GetHistoryMinuteTrade(date, code, start, count)
	} else {
		resp, err = svc.Client.GetMinuteTrade(code, start, count)
	}
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	klines, err := svc.FetchKline(code, tdx.ParseKlineType(ktype), 0, 250)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线失败: %v", err)})
		return
	}

	inputs := toKlineInputs(klines)
	_ = param.AutoInit()
	category := param.DetectCategory(code)
	cfg := param.Resolve(code, category)
	result := ta.Calculate(inputs, cfg)
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

	svc, err := getService()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
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

			klines, err := svc.FetchKline(c, tdx.ParseKlineType(ktype), 0, 250)
			if err != nil {
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
