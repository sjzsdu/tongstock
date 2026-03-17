package main

import (
	"fmt"
	"os"

	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "tongstock",
	Short: "通达信股票数据查询工具",
}

func init() {
	rootCmd.AddCommand(quoteCmd)
	rootCmd.AddCommand(codesCmd)
	rootCmd.AddCommand(klineCmd)
	rootCmd.AddCommand(minuteCmd)
	rootCmd.AddCommand(tradeCmd)
}

var quoteCmd = &cobra.Command{
	Use:   "quote [codes...]",
	Short: "查询股票行情",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runQuote,
}

func runQuote(cmd *cobra.Command, args []string) error {
	client, err := tdx.DialHosts(nil)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	quotes, err := client.GetQuote(args...)
	if err != nil {
		return fmt.Errorf("获取行情失败: %w", err)
	}

	for _, q := range quotes {
		fmt.Printf("%s %s\n", q.Code, q.Name)
		fmt.Printf("  最新价: %.3f\n", q.Price)
		fmt.Printf("  开盘: %.3f 最高: %.3f 最低: %.3f\n", q.Open, q.High, q.Low)
		fmt.Printf("  成交量: %.2f 手\n", q.Volume)
		fmt.Printf("  成交额: %.2f 万\n", q.Amount)
	}
	return nil
}

var codesExchange string

var codesCmd = &cobra.Command{
	Use:   "codes",
	Short: "获取股票代码列表",
	RunE:  runCodes,
}

func init() {
	codesCmd.Flags().StringVarP(&codesExchange, "exchange", "e", "sz", "交易所: sz/sh/bj")
}

func runCodes(cmd *cobra.Command, args []string) error {
	client, err := tdx.DialHosts(nil)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	exchange := protocol.ParseExchange(codesExchange)
	codes, err := client.GetCode(exchange)
	if err != nil {
		return fmt.Errorf("获取代码失败: %w", err)
	}

	fmt.Printf("共获取到 %d 条记录\n", len(codes))
	for _, code := range codes {
		fmt.Printf("%s %s\n", code.Code, code.Name)
	}
	return nil
}

var (
	klineCode string
	klineType string
	klineAll  bool
)

var klineCmd = &cobra.Command{
	Use:   "kline",
	Short: "查询K线数据",
	RunE:  runKline,
}

func init() {
	klineCmd.Flags().StringVarP(&klineCode, "code", "c", "", "股票代码")
	klineCmd.Flags().StringVarP(&klineType, "type", "t", "day", "K线类型: 1m/5m/15m/30m/60m/day/week/month/quarter/year")
	klineCmd.Flags().BoolVarP(&klineAll, "all", "a", false, "获取全部历史K线")
	klineCmd.MarkFlagRequired("code")
}

func runKline(cmd *cobra.Command, args []string) error {
	ktype := uint8(9)
	switch klineType {
	case "1m", "minute":
		ktype = 7
	case "5m":
		ktype = 0
	case "15m":
		ktype = 1
	case "30m":
		ktype = 2
	case "60m":
		ktype = 3
	case "day":
		ktype = 9
	case "week":
		ktype = 5
	case "month":
		ktype = 6
	case "quarter":
		ktype = 10
	case "year":
		ktype = 11
	}

	client, err := tdx.DialHosts(nil)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	var klines []*protocol.Kline
	if klineAll {
		klines, err = client.GetKlineAll(klineCode, ktype)
	} else {
		klines, err = client.GetKline(klineCode, ktype, 0, 100)
	}
	if err != nil {
		return fmt.Errorf("获取K线失败: %w", err)
	}

	fmt.Printf("共获取 %d 条K线数据\n", len(klines))
	for _, k := range klines {
		fmt.Printf("%s O:%.2f H:%.2f L:%.2f C:%.2f V:%.2f\n",
			k.Time.Format("2006-01-02"), k.Open, k.High, k.Low, k.Close, k.Volume)
	}
	return nil
}

var minuteCmd = &cobra.Command{
	Use:   "minute [code]",
	Short: "查询当日分时数据",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runMinute,
}

func runMinute(cmd *cobra.Command, args []string) error {
	client, err := tdx.DialHosts(nil)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	resp, err := client.GetMinute(args[0])
	if err != nil {
		return fmt.Errorf("获取分时数据失败: %w", err)
	}

	fmt.Printf("共获取 %d 条分时数据\n", resp.Count)
	for _, m := range resp.List {
		fmt.Printf("%s 价格: %.3f 成交量: %d\n", m.Time, m.Price, m.Number)
	}
	return nil
}

var (
	tradeCode    string
	tradeDate    string
	tradeStart   uint16
	tradeCount   uint16
	tradeHistory bool
)

var tradeCmd = &cobra.Command{
	Use:   "trade [code]",
	Short: "查询分笔成交数据",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runTrade,
}

func init() {
	tradeCmd.Flags().StringVarP(&tradeDate, "date", "d", "", "日期 (YYYYMMDD, 仅历史分时)")
	tradeCmd.Flags().Uint16VarP(&tradeStart, "start", "s", 0, "起始位置")
	tradeCmd.Flags().Uint16VarP(&tradeCount, "count", "c", 100, "数量")
	tradeCmd.Flags().BoolVarP(&tradeHistory, "history", "H", false, "历史分时成交")
}

func runTrade(cmd *cobra.Command, args []string) error {
	client, err := tdx.DialHosts(nil)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	var resp *protocol.TradeResp
	if tradeHistory && tradeDate != "" {
		resp, err = client.GetHistoryMinuteTrade(tradeDate, args[0], tradeStart, tradeCount)
	} else {
		resp, err = client.GetMinuteTrade(args[0], tradeStart, tradeCount)
	}
	if err != nil {
		return fmt.Errorf("获取分笔数据失败: %w", err)
	}

	fmt.Printf("共获取 %d 条分笔数据\n", resp.Count)
	for _, t := range resp.List {
		fmt.Printf("%s 价格: %.3f 成交量: %d 状态: %d\n",
			t.Time.Format("15:04"), t.Price, t.Volume, t.Status)
	}
	return nil
}
