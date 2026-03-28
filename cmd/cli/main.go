package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/ta"
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
	rootCmd.AddCommand(companyContentCmd)
	rootCmd.AddCommand(blockCmd)
	rootCmd.AddCommand(countCmd)
	rootCmd.AddCommand(auctionCmd)
	rootCmd.AddCommand(indicatorCmd)
	rootCmd.AddCommand(screenCmd)
}

// Indicator command and screen command
var (
	indicatorCode   string
	indicatorType   string
	indicatorAll    bool
	indicatorCount  int
	indicatorConfig string
)

var indicatorCmd = &cobra.Command{
	Use:   "indicator",
	Short: "查询技术指标",
	RunE:  runIndicator,
}

func init() {
	indicatorCmd.Flags().StringVarP(&indicatorCode, "code", "c", "", "股票代码")
	indicatorCmd.Flags().StringVarP(&indicatorType, "type", "t", "day", "K线类型")
	indicatorCmd.Flags().BoolVarP(&indicatorAll, "all", "a", false, "获取全部历史K线")
	indicatorCmd.Flags().IntVarP(&indicatorCount, "count", "n", 250, "K线数量")
	indicatorCmd.Flags().StringVarP(&indicatorConfig, "config", "", "", "参数配置文件路径")
	_ = indicatorCmd.MarkFlagRequired("code")
}

func runIndicator(cmd *cobra.Command, args []string) error {
	ktype := tdx.ParseKlineType(indicatorType)

	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	var klines []*protocol.Kline
	if indicatorAll {
		klines, err = svc.FetchKlineAll(indicatorCode, ktype)
	} else {
		klines, err = svc.FetchKline(indicatorCode, ktype, 0, uint16(indicatorCount))
	}
	if err != nil {
		return fmt.Errorf("获取K线失败: %w", err)
	}

	inputs := make([]ta.KlineInput, len(klines))
	for i, k := range klines {
		inputs[i] = ta.KlineInput{Time: k.Time, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume, Amount: k.Amount}
	}

	if indicatorConfig != "" {
		_ = param.Init(indicatorConfig)
	} else {
		_ = param.AutoInit()
	}
	category := param.DetectCategory(indicatorCode)
	cfg := param.Resolve(indicatorCode, category)

	result := ta.Calculate(inputs, cfg)
	signals := signal.Detect(indicatorCode, inputs, result, nil)

	// Table header
	fmt.Printf("\n%s 技术指标 (分类: %s)\n", indicatorCode, category)
	fmt.Println(strings.Repeat("=", 80))
	header := fmt.Sprintf("%-12s %-8s %-8s %-8s %-8s", "日期", "收盘", "MA5", "MA10", "MA20")
	if result.MACD != nil {
		header += fmt.Sprintf(" %-8s %-8s %-8s", "DIF", "DEA", "HIST")
	}
	if result.KDJ != nil {
		header += fmt.Sprintf(" %-8s %-8s %-8s", "K", "D", "J")
	}
	if result.BOLL != nil {
		header += fmt.Sprintf(" %-8s %-8s %-8s", "UPPER", "MID", "LOWER")
	}
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)))

	// Show last 20 rows (most recent)
	start := 0
	if len(inputs) > 20 {
		start = len(inputs) - 20
	}
	for i := start; i < len(inputs); i++ {
		row := fmt.Sprintf("%-12s %-8.2f %-8.2f %-8.2f %-8.2f",
			inputs[i].Time.Format("2006-01-02"), inputs[i].Close,
			result.MA["5"][i], result.MA["10"][i], result.MA["20"][i])
		if result.MACD != nil {
			row += fmt.Sprintf(" %-8.2f %-8.2f %-8.2f", result.MACD.DIF[i], result.MACD.DEA[i], result.MACD.Hist[i])
		}
		if result.KDJ != nil {
			row += fmt.Sprintf(" %-8.2f %-8.2f %-8.2f", result.KDJ.K[i], result.KDJ.D[i], result.KDJ.J[i])
		}
		if result.BOLL != nil {
			row += fmt.Sprintf(" %-8.2f %-8.2f %-8.2f", result.BOLL.Upper[i], result.BOLL.Middle[i], result.BOLL.Lower[i])
		}
		fmt.Println(row)
	}

	// Signals summary
	if len(signals) > 0 {
		fmt.Printf("\n最新信号:\n")
		fmt.Println(strings.Repeat("-", 60))
		recentSignals := signals
		if len(signals) > 10 {
			recentSignals = signals[len(signals)-10:]
		}
		for _, s := range recentSignals {
			fmt.Printf("  [%s] %s %s (%s) 强度: %.2f\n",
				s.Date.Format("2006-01-02"), s.Indicator, s.Type, s.Details, s.Strength)
		}
	}

	return nil
}

var (
	screenType   string
	screenCodes  string
	screenFile   string
	screenSignal string
	screenPool   int
)

var screenCmd = &cobra.Command{
	Use:   "screen",
	Short: "批量筛选股票信号",
	RunE:  runScreen,
}

func init() {
	screenCmd.Flags().StringVarP(&screenType, "type", "t", "day", "K线类型")
	screenCmd.Flags().StringVarP(&screenCodes, "codes", "c", "", "逗号分隔的股票代码列表")
	screenCmd.Flags().StringVarP(&screenFile, "file", "f", "", "股票代码文件路径（每行一个代码）")
	screenCmd.Flags().StringVarP(&screenSignal, "signal", "s", "", "筛选信号类型: golden_cross/death_cross/overbought/oversold")
	screenCmd.Flags().IntVarP(&screenPool, "pool", "p", 10, "并发池大小")
}

func runScreen(cmd *cobra.Command, args []string) error {
	ktype := tdx.ParseKlineType(screenType)

	// Parse code list
	var codeList []string
	if screenCodes != "" {
		for _, c := range strings.Split(screenCodes, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				codeList = append(codeList, c)
			}
		}
	} else if screenFile != "" {
		data, err := os.ReadFile(screenFile)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				codeList = append(codeList, line)
			}
		}
	} else {
		return fmt.Errorf("请指定 --codes 或 --file 参数")
	}

	if len(codeList) == 0 {
		return fmt.Errorf("没有有效的股票代码")
	}

	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	_ = param.AutoInit()

	type screenResult struct {
		Code    string
		Klines  []ta.KlineInput
		Ind     *ta.IndicatorResult
		Signals []signal.Signal
		Err     error
	}

	results := make([]screenResult, len(codeList))
	sem := make(chan struct{}, screenPool)
	var wg sync.WaitGroup

	for i, code := range codeList {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, c string) {
			defer wg.Done()
			defer func() { <-sem }()

			klines, err := svc.FetchKline(c, ktype, 0, 250)
			if err != nil {
				results[idx] = screenResult{Code: c, Err: err}
				return
			}

			inputs := make([]ta.KlineInput, len(klines))
			for j, k := range klines {
				inputs[j] = ta.KlineInput{Time: k.Time, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume, Amount: k.Amount}
			}

			category := param.DetectCategory(c)
			cfg := param.Resolve(c, category)
			ind := ta.Calculate(inputs, cfg)
			sigs := signal.Detect(c, inputs, ind, nil)

			results[idx] = screenResult{Code: c, Klines: inputs, Ind: ind, Signals: sigs}
		}(i, code)
	}
	wg.Wait()

	// Filter by signal type if specified
	var filtered []screenResult
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		if screenSignal == "" {
			filtered = append(filtered, r)
			continue
		}
		for _, s := range r.Signals {
			match := false
			switch screenSignal {
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

	// Output results
	fmt.Printf("\n筛选结果 (%d/%d 只股票)\n", len(filtered), len(codeList))
	fmt.Println(strings.Repeat("=", 100))
	header := fmt.Sprintf("%-8s %-10s %-8s %-8s %-8s %-8s %-8s %-8s %-8s 信号",
		"代码", "日期", "收盘", "MA5", "MA10", "MA20", "DIF", "K", "J")
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)+20))

	for _, r := range filtered {
		n := len(r.Klines)
		if n == 0 {
			continue
		}
		last := r.Klines[n-1]
		ma5 := r.Ind.MA["5"][n-1]
		ma10 := r.Ind.MA["10"][n-1]
		ma20 := r.Ind.MA["20"][n-1]
		dif := 0.0
		kVal := 0.0
		jVal := 0.0
		if r.Ind.MACD != nil {
			dif = r.Ind.MACD.DIF[n-1]
		}
		if r.Ind.KDJ != nil {
			kVal = r.Ind.KDJ.K[n-1]
			jVal = r.Ind.KDJ.J[n-1]
		}

		var sigStrs []string
		for _, s := range r.Signals {
			if n > 0 && s.Date.Equal(r.Klines[n-1].Time) {
				sigStrs = append(sigStrs, fmt.Sprintf("%s%s", s.Indicator, s.Type))
			}
		}
		sigStr := strings.Join(sigStrs, ", ")
		if sigStr == "" {
			sigStr = "-"
		}

		fmt.Printf("%-8s %-10s %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f %s\n",
			r.Code, last.Time.Format("2006-01-02"), last.Close, ma5, ma10, ma20, dif, kVal, jVal, sigStr)
	}

	return nil
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

// classifyCode 根据代码前缀分类证券
func classifyCode(code string) string {
	// 北交所: 8开头
	if strings.HasPrefix(code, "8") {
		return "北交所股票"
	}
	// 指数: 399开头
	if strings.HasPrefix(code, "399") {
		return "指数"
	}
	// 创业板: 300开头
	if strings.HasPrefix(code, "300") {
		return "创业板"
	}
	// 科创板: 688开头
	if strings.HasPrefix(code, "688") {
		return "科创板"
	}
	// 上证股票: 600/601/603开头
	if strings.HasPrefix(code, "6") {
		return "沪市A股"
	}
	// 深市主板: 000开头
	if strings.HasPrefix(code, "0") {
		return "深市主板"
	}
	// 基金: 1开头
	if strings.HasPrefix(code, "1") {
		return "基金"
	}
	// ETF: 5开头
	if strings.HasPrefix(code, "5") {
		return "ETF"
	}
	// 债券: 2开头
	if strings.HasPrefix(code, "2") {
		return "债券"
	}
	// REITs: 8开头(非北交所)
	if strings.HasPrefix(code, "9") {
		return "REITs"
	}

	return "其他"
}

var codesExchange string
var codesCategory string
var codesStats bool

var codesCmd = &cobra.Command{
	Use:   "codes",
	Short: "获取证券代码列表",
}

var codesListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出证券代码 (支持分类过滤)",
	RunE:  runCodesList,
}

var codesStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "显示证券分类统计",
	RunE:  runCodesStats,
}

func init() {
	codesCmd.AddCommand(codesListCmd)
	codesCmd.AddCommand(codesStatsCmd)

	codesCmd.PersistentFlags().StringVarP(&codesExchange, "exchange", "e", "sz", "交易所: sz/sh/bj")
	codesListCmd.Flags().StringVarP(&codesCategory, "category", "c", "", "分类过滤: stock/fund/etf/bond/index/gem/all")
	codesStatsCmd.Flags().BoolVarP(&codesStats, "all", "a", false, "显示所有交易所统计")

	companyContentCmd.Flags().Uint32VarP(&companyContentStart, "start", "s", 0, "起始位置")
	companyContentCmd.Flags().Uint32VarP(&companyContentLength, "length", "l", 10000, "内容长度")
	companyContentCmd.Flags().StringVarP(&companyContentBlock, "block", "b", "", "块名称（如：公司概况）")
}

func runCodesStats(cmd *cobra.Command, args []string) error {
	exchanges := []string{codesExchange}
	if codesStats {
		exchanges = []string{"sz", "sh", "bj"}
	}

	for _, exch := range exchanges {
		exchangeName := map[string]string{"sz": "深圳交易所", "sh": "上海交易所", "bj": "北京交易所"}[exch]
		fmt.Printf("\n=== %s ===\n", exchangeName)

		svc, err := dialService()
		if err != nil {
			fmt.Printf("连接失败: %v\n", err)
			continue
		}

		exch := protocol.ParseExchange(exch)
		codes, err := svc.FetchCodes(exch)
		svc.Close()
		if err != nil {
			fmt.Printf("获取失败: %v\n", err)
			continue
		}

		// 统计各类别
		stats := make(map[string]int)
		for _, c := range codes {
			cat := classifyCode(c.Code)
			stats[cat]++
		}

		// 输出统计
		total := 0
		for cat, count := range stats {
			fmt.Printf("  %-10s: %d\n", cat, count)
			total += count
		}
		fmt.Printf("  %-10s: %d\n", "总计", total)
	}

	return nil
}

func runCodesList(cmd *cobra.Command, args []string) error {
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

	// 过滤分类
	var filtered []*protocol.CodeItem
	if codesCategory != "" && codesCategory != "all" {
		for _, c := range codes {
			cat := classifyCode(c.Code)
			shouldInclude := false
			switch codesCategory {
			case "stock":
				shouldInclude = cat == "沪市A股" || cat == "深市主板" || cat == "创业板" || cat == "科创板" || cat == "北交所股票"
			case "fund":
				shouldInclude = cat == "基金"
			case "etf":
				shouldInclude = cat == "ETF"
			case "bond":
				shouldInclude = cat == "债券"
			case "index":
				shouldInclude = cat == "指数"
			case "gem":
				shouldInclude = cat == "创业板"
			}
			if shouldInclude {
				filtered = append(filtered, c)
			}
		}
	} else {
		filtered = codes
	}

	// 输出
	fmt.Printf("交易所: %s, 共 %d 条记录", codesExchange, len(filtered))
	if codesCategory != "" {
		fmt.Printf(" (分类: %s)", codesCategory)
	}
	fmt.Println()

	exchName := map[string]string{"sz": "深交所", "sh": "上交所", "bj": "北交所"}[codesExchange]
	for _, code := range filtered {
		cat := classifyCode(code.Code)
		fmt.Printf("%s %s [%s] %s\n", code.Code, code.Name, cat, exchName)
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
	_ = klineCmd.MarkFlagRequired("code")
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
	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	var resp *protocol.MinuteResp
	if minuteDate != "" {
		resp, err = client.GetHistoryMinute(minuteDate, args[0])
	} else {
		resp, err = client.GetMinute(args[0])
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
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	info, err := svc.FetchFinance(args[0])
	if err != nil {
		return fmt.Errorf("获取财务数据失败: %w", err)
	}

	fmt.Printf("总股本: %.2f万股  流通股本: %.2f万股\n", info.ZongGuBen/10000, info.LiuTongGuBen/10000)
	fmt.Printf("总资产: %.2f亿元  净资产: %.2f亿元\n", info.ZongZiChan/1000000000, info.JingZiChan/1000000000)
	fmt.Printf("主营收入: %.2f亿元  净利润: %.2f亿元\n", info.ZhuYingShouRu/1000000000, info.JingLiRun/1000000000)
	fmt.Printf("每股净资产: %.4f元  股东人数: %.0f\n", info.MeiGuJingZiChan, info.GuDongRenShu)
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
	_ = indexCmd.MarkFlagRequired("code")
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

var (
	blockFile     string
	blockType     string
	blockShowCode string
)

var blockSort bool

// blockStatItem 用于排序和过滤的板块统计结构
type blockStatItem struct {
	name      string
	blockType uint16
	count     int
	codes     []string
}

// isValidBlockName 检查板块名称是否有效 (过滤掉纯数字或异常名称)
func isValidBlockName(name string) bool {
	if name == "" {
		return false
	}
	// 检查是否纯数字 (可能是编码错误导致的截断)
	hasNonDigit := false
	for _, c := range name {
		if c < '0' || c > '9' {
			hasNonDigit = true
			break
		}
	}
	return hasNonDigit
}

// blockStats 结构用于按板块名称分组统计
type blockStats struct {
	blockType  uint16
	stockCodes []string
}

// groupByBlock 按板块名称分组
func groupByBlock(items []*protocol.BlockItem) map[string]*blockStats {
	result := make(map[string]*blockStats)
	for _, item := range items {
		if _, ok := result[item.BlockName]; !ok {
			result[item.BlockName] = &blockStats{blockType: item.BlockType, stockCodes: make([]string, 0)}
		}
		result[item.BlockName].stockCodes = append(result[item.BlockName].stockCodes, item.StockCode)
	}
	return result
}

// getCodeNameMap 获取股票代码到名称的映射
func getCodeNameMap(svc *tdx.Service) map[string]string {
	codeNameMap := make(map[string]string)

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

// showBlocksByCode 根据股票代码查询所属板块
func showBlocksByCode(svc *tdx.Service, items []*protocol.BlockItem, code string) error {
	var blocks []struct {
		name      string
		blockType uint16
		count     int
	}

	for name, stats := range groupByBlock(items) {
		if !isValidBlockName(name) {
			continue
		}
		for _, stockCode := range stats.stockCodes {
			if stockCode == code {
				blocks = append(blocks, struct {
					name      string
					blockType uint16
					count     int
				}{name: name, blockType: stats.blockType, count: len(stats.stockCodes)})
				break
			}
		}
	}

	if len(blocks) == 0 {
		return fmt.Errorf("股票 %s 未在任何板块中找到", code)
	}

	// 获取股票名称
	codeNameMap := getCodeNameMap(svc)
	stockName := codeNameMap[code]
	if stockName == "" {
		stockName = "未知"
	}

	fmt.Printf("股票: %s %s 所属板块:\n", code, stockName)
	fmt.Println(strings.Repeat("-", 50))
	for _, b := range blocks {
		fmt.Printf("  %s (type:%d, %d只成分股)\n", b.name, b.blockType, b.count)
	}

	return nil
}

// showBlockList 显示所有有效板块供选择
func showBlockList(items []*protocol.BlockItem) error {
	blockMap := groupByBlock(items)

	var validBlocks []blockStatItem
	for name, stats := range blockMap {
		if !isValidBlockName(name) {
			continue
		}
		validBlocks = append(validBlocks, blockStatItem{
			name:      name,
			blockType: stats.blockType,
			count:     len(stats.stockCodes),
		})
	}

	fmt.Printf("共 %d 个有效板块:\n", len(validBlocks))
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%-6s %-20s %-10s\n", "Type", "板块名称", "成分股数")
	fmt.Println(strings.Repeat("-", 60))

	for _, b := range validBlocks {
		fmt.Printf("%-6d %-20s %-10d\n", b.blockType, b.name, b.count)
	}

	fmt.Println("\n使用 block show <板块名称> 查看成分股")
	return nil
}

var blockCmd = &cobra.Command{
	Use:   "block",
	Short: "查询板块分类信息",
}

var blockFilesCmd = &cobra.Command{
	Use:   "files",
	Short: "列出所有可用的板块文件",
	RunE:  runBlockFiles,
}

var blockListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有板块 [type, 板块编码, 板块名称, 成分股数量]",
	RunE:  runBlockList,
}

var blockShowCmd = &cobra.Command{
	Use:   "show [板块名称]",
	Short: "显示指定板块的成分股 (支持模糊搜索)",
	Args:  cobra.RangeArgs(0, 1),
	RunE:  runBlockShow,
}

func init() {
	blockCmd.AddCommand(blockFilesCmd)
	blockCmd.AddCommand(blockListCmd)
	blockCmd.AddCommand(blockShowCmd)

	blockCmd.PersistentFlags().StringVarP(&blockFile, "file", "f", "block_zs.dat", "板块文件")
	blockListCmd.Flags().StringVarP(&blockFile, "file", "f", "block_zs.dat", "板块文件: block.dat/block_zs.dat/block_fg.dat/block_gn.dat")
	blockListCmd.Flags().StringVarP(&blockType, "type", "t", "", "按Type过滤 (如: 2)")
	blockListCmd.Flags().BoolVarP(&blockSort, "sort", "s", false, "按成分股数量排序")
	blockShowCmd.Flags().StringVarP(&blockFile, "file", "f", "block_zs.dat", "板块文件")
	blockShowCmd.Flags().StringVarP(&blockShowCode, "code", "c", "", "根据股票代码查询所属板块")
}

// availableBlockFiles 定义可用的板块文件
var availableBlockFiles = []struct {
	File string
	Name string
	Desc string
}{
	{"block.dat", "综合板块", "综合分类"},
	{"block_zs.dat", "指数板块", "主要指数成分股"},
	{"block_fg.dat", "行业板块", "行业分类"},
	{"block_gn.dat", "概念板块", "概念主题"},
}

func runBlockFiles(cmd *cobra.Command, args []string) error {
	fmt.Println("可用的板块文件:")
	fmt.Println(strings.Repeat("-", 60))
	for _, f := range availableBlockFiles {
		fmt.Printf("  %-15s %-10s %s\n", f.File, f.Name, f.Desc)
	}
	return nil
}

func runBlockList(cmd *cobra.Command, args []string) error {
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	items, err := svc.FetchBlock(blockFile)
	if err != nil {
		return fmt.Errorf("获取板块信息失败: %w", err)
	}

	// 按板块名称统计
	blockStats := make(map[string]struct {
		BlockType  uint16
		StockCount int
		StockCodes []string
	})
	for _, item := range items {
		if _, ok := blockStats[item.BlockName]; !ok {
			blockStats[item.BlockName] = struct {
				BlockType  uint16
				StockCount int
				StockCodes []string
			}{BlockType: item.BlockType, StockCount: 0, StockCodes: make([]string, 0)}
		}
		s := blockStats[item.BlockName]
		s.StockCount++
		s.StockCodes = append(s.StockCodes, item.StockCode)
		blockStats[item.BlockName] = s
	}

	// 过滤和排序
	var filteredBlocks []blockStatItem
	for name, stats := range blockStats {
		// 按type过滤
		if blockType != "" {
			if fmt.Sprintf("%d", stats.BlockType) != blockType {
				continue
			}
		}
		// 过滤掉无效板块名称 (纯数字或异常名称)
		if !isValidBlockName(name) {
			continue
		}
		filteredBlocks = append(filteredBlocks, blockStatItem{
			name:      name,
			blockType: stats.BlockType,
			count:     stats.StockCount,
			codes:     stats.StockCodes,
		})
	}

	// 排序
	if blockSort {
		sort.Slice(filteredBlocks, func(i, j int) bool {
			return filteredBlocks[i].count > filteredBlocks[j].count
		})
	}

	// 输出板块列表
	fmt.Printf("板块文件: %s\n", blockFile)
	if blockType != "" {
		fmt.Printf("Type过滤: %s\n", blockType)
	}
	fmt.Printf("共 %d 个板块 (已过滤无效名称), %d 条记录\n", len(filteredBlocks), len(items))
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-6s %-20s %-10s %s\n", "Type", "板块名称", "成分股数", "示例代码")
	fmt.Println(strings.Repeat("-", 70))

	for _, b := range filteredBlocks {
		exampleCodes := ""
		if len(b.codes) > 3 {
			exampleCodes = strings.Join(b.codes[:3], ", ") + "..."
		} else {
			exampleCodes = strings.Join(b.codes, ", ")
		}
		fmt.Printf("%-6d %-20s %-10d %s\n", b.blockType, b.name, b.count, exampleCodes)
	}

	return nil
}

func runBlockShow(cmd *cobra.Command, args []string) error {
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	items, err := svc.FetchBlock(blockFile)
	if err != nil {
		return fmt.Errorf("获取板块信息失败: %w", err)
	}

	// 如果指定了 --code 参数，查询股票所属板块
	if blockShowCode != "" {
		return showBlocksByCode(svc, items, blockShowCode)
	}

	// 没有参数时，列出所有有效板块供选择
	if len(args) == 0 {
		return showBlockList(items)
	}

	blockName := args[0]

	// 筛选指定板块的股票 (支持模糊匹配)
	var matchedBlocks []struct {
		name      string
		blockType uint16
		stocks    []string
	}

	for name, stats := range groupByBlock(items) {
		// 模糊匹配: 包含搜索关键词 或 完全匹配
		if strings.Contains(name, blockName) || name == blockName {
			// 过滤掉无效板块名称
			if !isValidBlockName(name) {
				continue
			}
			matchedBlocks = append(matchedBlocks, struct {
				name      string
				blockType uint16
				stocks    []string
			}{name: name, blockType: stats.blockType, stocks: stats.stockCodes})
		}
	}

	if len(matchedBlocks) == 0 {
		return fmt.Errorf("未找到板块: %s (可使用 block list 查看所有板块)", blockName)
	}

	// 获取股票名称
	codeNameMap := getCodeNameMap(svc)

	// 如果匹配多个板块，让用户选择
	if len(matchedBlocks) > 1 {
		fmt.Printf("找到多个匹配的板块:\n")
		fmt.Println(strings.Repeat("-", 50))
		for i, b := range matchedBlocks {
			fmt.Printf("  %d. %s (type:%d, %d只成分股)\n", i+1, b.name, b.blockType, len(b.stocks))
		}
		fmt.Println("\n请使用更精确的名称，或使用 list 命令查看所有板块")
		return nil
	}

	// 显示单个板块的成分股
	block := matchedBlocks[0]
	stocks := block.stocks

	// 输出
	fmt.Printf("板块: %s (type:%d) - 共 %d 只成分股\n", block.name, block.blockType, len(stocks))
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("%-10s %-15s %s\n", "代码", "名称", "交易所")
	fmt.Println(strings.Repeat("-", 50))

	for _, code := range stocks {
		name := codeNameMap[code]
		if name == "" {
			name = "未知"
		}
		exchange := "深交所"
		if strings.HasPrefix(code, "6") {
			exchange = "上交所"
		} else if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "3") {
			exchange = "北交所"
		}
		fmt.Printf("%-10s %-15s %s\n", code, name, exchange)
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

var countExchange string

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "查询证券数量",
	RunE:  runCount,
}

func init() {
	countCmd.Flags().StringVarP(&countExchange, "exchange", "e", "sz", "交易所: sz/sh/bj")
}

func runCount(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	exchange := protocol.ParseExchange(countExchange)
	count, err := client.GetSecurityCount(exchange)
	if err != nil {
		return fmt.Errorf("获取证券数量失败: %w", err)
	}

	fmt.Printf("%s 交易所证券数量: %d\n", countExchange, count)
	return nil
}

var auctionCmd = &cobra.Command{
	Use:   "auction [code]",
	Short: "查询集合竞价数据",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runAuction,
}

func runAuction(cmd *cobra.Command, args []string) error {
	client, err := dialClient()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer client.Close()

	resp, err := client.GetCallAuction(args[0])
	if err != nil {
		return fmt.Errorf("获取集合竞价数据失败: %w", err)
	}

	fmt.Printf("共获取 %d 条集合竞价数据\n", resp.Count)
	for _, a := range resp.List {
		dir := "买"
		if a.Flag < 0 {
			dir = "卖"
		}
		fmt.Printf("%s 价格: %.3f 匹配量: %d 未匹配量: %d (%s)\n",
			a.Time.Format("15:04:05"), a.Price, a.Match, a.Unmatched, dir)
	}
	return nil
}
