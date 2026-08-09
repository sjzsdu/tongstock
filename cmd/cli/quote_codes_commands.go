package main

import (
	"fmt"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
	"strings"
)

var quoteCmd = &cobra.Command{
	Use:   "quote [codes...]",
	Short: "查询股票行情",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runQuote,
}

func runQuote(cmd *cobra.Command, args []string) error {
	service, cleanup, err := dialStockData(cmd.Context())
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer cleanup()

	for _, code := range args {
		spec := stockdata.DataSpec{
			Type: stockdata.DataQuote, Market: cliMarketForCode(code), Code: code,
		}
		result, err := service.Query(cmd.Context(), cliDataRequest(spec))
		if err != nil {
			return fmt.Errorf("获取行情失败: %w", cliDataError(err, spec))
		}
		q := result.Quote
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
}

// cliMarketForCode 根据 code 前缀推断子市场（用于 DataSpec.Market 字段）。
// "SH" / "SZ" / "BJ" 三种结果，分别对应 6/0 开头，以及 4/8 开头（北交所）。
// 其他代码回退到 "CN-A"，避免 panic。
func cliMarketForCode(code string) string {
	c := strings.TrimSpace(code)
	switch {
	case strings.HasPrefix(c, "6"):
		return "SH"
	case strings.HasPrefix(c, "0") || strings.HasPrefix(c, "3"):
		return "SZ"
	case strings.HasPrefix(c, "8") || strings.HasPrefix(c, "4"):
		return "BJ"
	}
	return "CN-A"
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
