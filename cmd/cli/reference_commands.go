package main

import (
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
	"sort"
	"strings"
)

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
	for _, c := range codesSZ {
		codeNameMap[c.Code] = c.Name
	}

	codesSH, _ := svc.FetchCodes(protocol.ExchangeSH)
	for _, c := range codesSH {
		codeNameMap[c.Code] = c.Name
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

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "查询证券数量",
	RunE:  runCount,
}

func init() {
	countCmd.Flags().StringVarP(&countExchange, "exchange", "e", "sz", "交易所: sz/sh/bj")
}

func runCount(cmd *cobra.Command, args []string) error {
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	exchange := protocol.ParseExchange(countExchange)
	count, err := svc.GetSecurityCount(exchange)
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
	svc, err := dialService()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer svc.Close()

	resp, err := svc.GetCallAuction(args[0])
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
