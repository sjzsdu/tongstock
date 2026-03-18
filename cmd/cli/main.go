package main

import (
	"fmt"
	"os"

	"github.com/sjzsdu/tongstock/pkg/config"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.Init()
	},
}

// dialService creates a connected Service wrapper around a Client.
func dialService() (*tdx.Service, error) {
	client, err := tdx.DialHosts(config.Get().TDX.Hosts)
	if err != nil {
		return nil, err
	}
	return tdx.NewService(client)
}

// dialClient keeps backward compatibility for commands that use the raw Client.
func dialClient() (*tdx.Client, error) {
	return tdx.DialHosts(config.Get().TDX.Hosts)
}

func init() {
	rootCmd.AddCommand(quoteCmd)
	rootCmd.AddCommand(codesCmd)
	rootCmd.AddCommand(klineCmd)
	rootCmd.AddCommand(minuteCmd)
	rootCmd.AddCommand(tradeCmd)
	rootCmd.AddCommand(xdxrCmd)
	rootCmd.AddCommand(financeCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(companyCmd)
	rootCmd.AddCommand(blockCmd)
}

var quoteCmd = &cobra.Command{
	Use:   "quote [codes...]",
	Short: "查询股票行情",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runQuote,
}

func runQuote(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
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
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()
	exchange := protocol.ParseExchange(codesExchange)
	codes, err := svc.FetchCodes(exchange)
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
	// Parse kline type using shared helper
	ktype := tdx.ParseKlineType(klineType)

	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	var klines []*protocol.Kline
	if klineAll {
		klines, err = svc.FetchKlineAll(klineCode, ktype)
	} else {
		klines, err = svc.FetchKline(klineCode, ktype, 0, 100)
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
	client, err := dialClient()
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

var xdxrCmd = &cobra.Command{
	Use:   "xdxr [code]",
	Short: "查询除权除息信息",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runXdXr,
}

func runXdXr(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	items, err := client.GetXdXrInfo(args[0])
	if err != nil {
		return fmt.Errorf("获取除权除息失败: %w", err)
	}

	fmt.Printf("共获取 %d 条除权除息记录\n", len(items))
	for _, item := range items {
		fmt.Printf("%s [%s] ", item.Date.Format("2006-01-02"), item.Category)
		switch item.Category {
		case protocol.XdXrChuQuanChuXi:
			fmt.Printf("分红:%.4f 配股价:%.2f 送转:%.2f 配股:%.2f\n",
				item.FenHong, item.PeiGuJia, item.SongZhuanGu, item.PeiGu)
		default:
			fmt.Printf("流通:%.0f 总股本:%.0f\n", item.PanHouLiuTong, item.HouZongGuBen)
		}
	}
	return nil
}

var financeCmd = &cobra.Command{
	Use:   "finance [code]",
	Short: "查询财务数据",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runFinance,
}

func runFinance(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	info, err := client.GetFinanceInfo(args[0])
	if err != nil {
		return fmt.Errorf("获取财务数据失败: %w", err)
	}

	fmt.Printf("总股本: %.2f万  流通股本: %.2f万\n", info.ZongGuBen, info.LiuTongGuBen)
	fmt.Printf("总资产: %.2f万  净资产: %.2f万\n", info.ZongZiChan, info.JingZiChan)
	fmt.Printf("主营收入: %.2f万  净利润: %.2f万\n", info.ZhuYingShouRu, info.JingLiRun)
	fmt.Printf("每股净资产: %.4f  股东人数: %.0f\n", info.MeiGuJingZiChan, info.GuDongRenShu)
	fmt.Printf("IPO日期: %d  更新日期: %d\n", info.IPODate, info.UpdatedDate)
	return nil
}

var (
	indexCode string
	indexType string
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "查询指数K线数据",
	RunE:  runIndex,
}

func init() {
	indexCmd.Flags().StringVarP(&indexCode, "code", "c", "", "指数代码")
	indexCmd.Flags().StringVarP(&indexType, "type", "t", "day", "K线类型: 1m/5m/15m/30m/60m/day/week/month")
	indexCmd.MarkFlagRequired("code")
}

func runIndex(cmd *cobra.Command, args []string) error {
	ktype := tdx.ParseKlineType(indexType)

	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	bars, err := client.GetIndexBars(indexCode, ktype, 0, 100)
	if err != nil {
		return fmt.Errorf("获取指数K线失败: %w", err)
	}

	fmt.Printf("共获取 %d 条指数K线数据\n", len(bars))
	for _, b := range bars {
		fmt.Printf("%s O:%.2f H:%.2f L:%.2f C:%.2f V:%.2f Up:%d Down:%d\n",
			b.Time.Format("2006-01-02"), b.Open, b.High, b.Low, b.Close, b.Volume, b.UpCount, b.DownCount)
	}
	return nil
}

var companyCmd = &cobra.Command{
	Use:   "company [code]",
	Short: "查询公司信息(F10)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runCompany,
}

func runCompany(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	cats, err := client.GetCompanyInfoCategory(args[0])
	if err != nil {
		return fmt.Errorf("获取公司信息目录失败: %w", err)
	}

	for _, cat := range cats {
		fmt.Printf("[%s] %s (offset:%d len:%d)\n", cat.Filename, cat.Name, cat.Start, cat.Length)
	}
	return nil
}

var (
	blockFile string
)

var blockCmd = &cobra.Command{
	Use:   "block",
	Short: "查询板块分类信息",
	RunE:  runBlock,
}

func init() {
	blockCmd.Flags().StringVarP(&blockFile, "file", "f", "block_zs.dat", "板块文件: block.dat/block_zs.dat/block_fg.dat/block_gn.dat")
}

func runBlock(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	items, err := client.GetBlockInfoAll(blockFile)
	if err != nil {
		return fmt.Errorf("获取板块信息失败: %w", err)
	}

	fmt.Printf("共获取 %d 条板块记录\n", len(items))
	for _, item := range items {
		fmt.Printf("[%s] %s (type:%d)\n", item.BlockName, item.StockCode, item.BlockType)
	}
	return nil
}

func runTrade(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
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
