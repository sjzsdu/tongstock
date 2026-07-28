package main

import (
	"fmt"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
	"strings"
)

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
	_ = klineCmd.MarkFlagRequired("code")
}

func runKline(cmd *cobra.Command, args []string) error {
	// Parse kline type using shared helper
	ktype := tdx.ParseKlineType(klineType)

	if ktype != tdx.ParseKlineType("day") {
		return runLegacyKline(ktype)
	}
	service, cleanup, err := dialStockData(cmd.Context())
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer cleanup()
	result, err := service.Query(cmd.Context(), cliDataRequest(stockdata.DataSpec{
		Type: stockdata.DataKline, Market: cliMarketForCode(klineCode), Code: klineCode,
		Granularity: klineType, KType: ktype,
	}))
	if err != nil {
		return fmt.Errorf("获取K线失败: %w", err)
	}
	klines := result.Klines
	if !klineAll && len(klines) > 100 {
		klines = klines[len(klines)-100:]
	}

	fmt.Printf("共获取 %d 条K线数据\n", len(klines))
	for _, k := range klines {
		fmt.Printf("%s O:%.2f H:%.2f L:%.2f C:%.2f V:%.2f\n",
			k.Time.Format("2006-01-02"), k.Open, k.High, k.Low, k.Close, k.Volume)
	}
	return nil
}

func runLegacyKline(ktype uint8) error {
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

var (
	minuteDate string
)

var minuteCmd = &cobra.Command{
	Use:   "minute [code]",
	Short: "查询分时数据（支持当日和历史）",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runMinute,
}

func init() {
	minuteCmd.Flags().StringVarP(&minuteDate, "date", "d", "", "日期 (YYYYMMDD)，不指定则查询当日")
}

func runMinute(cmd *cobra.Command, args []string) error {
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	var resp *protocol.MinuteResp
	if minuteDate != "" {
		resp, err = svc.GetHistoryMinute(minuteDate, args[0])
	} else {
		resp, err = svc.GetMinute(args[0])
	}
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
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	items, err := svc.FetchXdXr(args[0])
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
	service, cleanup, err := dialStockData(cmd.Context())
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer cleanup()
	result, err := service.Query(cmd.Context(), cliDataRequest(stockdata.DataSpec{
		Type: stockdata.DataFinance, Market: cliMarketForCode(args[0]), Code: args[0],
	}))
	if err != nil {
		return fmt.Errorf("获取财务数据失败: %w", err)
	}
	info := result.Finance

	fmt.Printf("总股本: %.2f万股  流通股本: %.2f万股\n", info.ZongGuBen/10000, info.LiuTongGuBen/10000)
	fmt.Printf("总资产: %.2f亿元  净资产: %.2f亿元\n", info.ZongZiChan/1000000000, info.JingZiChan/1000000000)
	fmt.Printf("主营收入: %.2f亿元  净利润: %.2f亿元\n", info.ZhuYingShouRu/1000000000, info.JingLiRun/1000000000)
	fmt.Printf("每股净资产: %.4f元  股东人数: %.0f\n", info.MeiGuJingZiChan, info.GuDongRenShu)
	fmt.Printf("IPO日期: %d  更新日期: %d\n", info.IPODate, info.UpdatedDate)
	return nil
}

func cliMarketForCode(code string) string {
	lower := strings.ToLower(strings.TrimSpace(code))
	if len(lower) >= 2 {
		switch lower[:2] {
		case "sh", "sz", "bj":
			return lower[:2]
		}
	}
	if strings.HasPrefix(code, "6") {
		return "sh"
	}
	if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "4") {
		return "bj"
	}
	return "sz"
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
	_ = indexCmd.MarkFlagRequired("code")
}

func runIndex(cmd *cobra.Command, args []string) error {
	ktype := tdx.ParseKlineType(indexType)

	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	bars, err := svc.GetIndexBars(indexCode, ktype, 0, 100)
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
	Short: "查询公司信息(F10)目录",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runCompany,
}

func runCompany(cmd *cobra.Command, args []string) error {
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	cats, err := svc.FetchCompanyCategory(args[0])
	if err != nil {
		return fmt.Errorf("获取公司信息目录失败: %w", err)
	}

	for _, cat := range cats {
		fmt.Printf("[%s] %s (offset:%d len:%d)\n", cat.Filename, cat.Name, cat.Start, cat.Length)
	}
	return nil
}

var (
	companyContentStart  uint32
	companyContentLength uint32
	companyContentBlock  string
)

var companyContentCmd = &cobra.Command{
	Use:   "company-content [code] [filename]",
	Short: "查询公司信息(F10)具体内容",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runCompanyContent,
}

func runCompanyContent(cmd *cobra.Command, args []string) error {
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	code := args[0]
	var filename string
	if len(args) > 1 {
		filename = args[1]
	} else {
		// 自动推断 filename
		filename = code + ".txt"
	}

	start := companyContentStart
	length := companyContentLength

	// 如果指定了块名称，查找对应的 start 和 length
	if companyContentBlock != "" {
		cats, err := svc.FetchCompanyCategory(code)
		if err != nil {
			return fmt.Errorf("获取公司信息目录失败: %w", err)
		}
		found := false
		for _, cat := range cats {
			if cat.Name == companyContentBlock {
				start = cat.Start
				length = cat.Length
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("未找到块名称: %s", companyContentBlock)
		}
	}

	content, err := svc.FetchCompanyContent(code, filename, start, length)
	if err != nil {
		return fmt.Errorf("获取公司信息内容失败: %w", err)
	}

	fmt.Println(content)
	return nil
}

func runTrade(cmd *cobra.Command, args []string) error {
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	var resp *protocol.TradeResp
	if tradeHistory && tradeDate != "" {
		resp, err = svc.GetHistoryMinuteTrade(tradeDate, args[0], tradeStart, tradeCount)
	} else {
		resp, err = svc.GetMinuteTrade(args[0], tradeStart, tradeCount)
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

var countExchange string
