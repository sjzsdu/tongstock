package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

var client *tdx.Client

func main() {
	var err error
	client, err = tdx.DialHosts(nil)
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

	log.Println("服务启动于 http://localhost:8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func getClient() (*tdx.Client, error) {
	if client != nil {
		return client, nil
	}
	return tdx.DialHosts(nil)
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
