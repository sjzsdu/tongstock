package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/config"
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

	resp, err := svc.Client.GetMinute(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取分时数据失败: %v", err)})
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
