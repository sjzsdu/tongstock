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

var client *tdx.Client

func main() {
	if err := config.Init(); err != nil {
		log.Printf("加载配置失败: %v, 使用默认配置", err)
	}
	cfg := config.Get()

	var err error
	client, err = tdx.DialHosts(cfg.TDX.Hosts)
	if err != nil {
		log.Printf("连接服务器失败: %v, 将在请求时重连", err)
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

func getClient() (*tdx.Client, error) {
	if client != nil {
		return client, nil
	}
	return tdx.DialHosts(config.Get().TDX.Hosts)
}

func handleQuote(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return
	}

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	quotes, err := cli.GetQuote(code)
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

	klineType := uint8(9)
	switch ktype {
	case "1m", "minute":
		klineType = 7
	case "5m":
		klineType = 0
	case "15m":
		klineType = 1
	case "30m":
		klineType = 2
	case "60m":
		klineType = 3
	case "day":
		klineType = 9
	case "week":
		klineType = 5
	case "month":
		klineType = 6
	case "quarter":
		klineType = 10
	case "year":
		klineType = 11
	}

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	klines, err := cli.GetKline(code, klineType, 0, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取K线失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, klines)
}

func handleCodes(c *gin.Context) {
	exchangeStr := c.DefaultQuery("exchange", "sz")
	exchange := protocol.ParseExchange(exchangeStr)

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	codes, err := cli.GetCode(exchange)
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

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	resp, err := cli.GetMinute(code)
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

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	items, err := cli.GetXdXrInfo(code)
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

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	info, err := cli.GetFinanceInfo(code)
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

	klineType := uint8(9)
	switch ktype {
	case "1m", "minute":
		klineType = 7
	case "5m":
		klineType = 0
	case "15m":
		klineType = 1
	case "30m":
		klineType = 2
	case "60m":
		klineType = 3
	case "day":
		klineType = 9
	case "week":
		klineType = 5
	case "month":
		klineType = 6
	case "quarter":
		klineType = 10
	case "year":
		klineType = 11
	}

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	bars, err := cli.GetIndexBars(code, klineType, 0, 100)
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

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	cats, err := cli.GetCompanyInfoCategory(code)
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

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	content, err := cli.GetCompanyInfoContent(code, filename, start, length)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取公司信息内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": content})
}

func handleBlock(c *gin.Context) {
	blockFile := c.DefaultQuery("file", "block_zs.dat")

	cli, err := getClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("连接失败: %v", err)})
		return
	}

	items, err := cli.GetBlockInfoAll(blockFile)
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

	cli, err := getClient()
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
		resp, err = cli.GetHistoryMinuteTrade(date, code, start, count)
	} else {
		resp, err = cli.GetMinuteTrade(code, start, count)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取分笔数据失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resp)
}
