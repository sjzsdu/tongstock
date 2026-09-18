package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	financeJSON bool
	xdxrJSON    bool
)

// financeCmd 获取个股财务数据
var financeCmd = &cobra.Command{
	Use:   "finance <code>",
	Short: "查看个股财务数据",
	Long: `获取指定股票的财务数据，包括总股本、流通股本、净资产、净利润、每股净资产等。

示例:
  tongstock finance 000001
  tongstock finance 600519 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runFinance,
}

func init() {
	financeCmd.Flags().BoolVar(&financeJSON, "json", false, "以 JSON 输出")
}

func runFinance(cmd *cobra.Command, args []string) error {
	code := strings.TrimSpace(args[0])

	svc, err := dialService()
	if err != nil {
		return err
	}

	info, err := svc.FetchFinance(code)
	if err != nil {
		return fmt.Errorf("获取财务数据失败: %w", err)
	}

	if financeJSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(info)
	}

	fmt.Printf("财务数据 (%s):\n", code)
	fmt.Printf("  总股本:       %.2f 万股\n", info.ZongGuBen)
	fmt.Printf("  流通股本:     %.2f 万股\n", info.LiuTongGuBen)
	fmt.Printf("  总资产:       %.2f 万元\n", info.ZongZiChan)
	fmt.Printf("  净资产:       %.2f 万元\n", info.JingZiChan)
	fmt.Printf("  主营收入:     %.2f 万元\n", info.ZhuYingShouRu)
	fmt.Printf("  净利润:       %.2f 万元\n", info.JingLiRun)
	fmt.Printf("  每股净资产:   %.2f 元\n", info.MeiGuJingZiChan)
	fmt.Printf("  股东人数:     %.0f 人\n", info.GuDongRenShu)
	return nil
}

// xdxrCmd 获取个股除权除息数据
var xdxrCmd = &cobra.Command{
	Use:   "xdxr <code>",
	Short: "查看个股除权除息数据",
	Long: `获取指定股票的除权除息历史记录。

示例:
  tongstock xdxr 000001
  tongstock xdxr 600519 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runXdXr,
}

func init() {
	xdxrCmd.Flags().BoolVar(&xdxrJSON, "json", false, "以 JSON 输出")
}

func runXdXr(cmd *cobra.Command, args []string) error {
	code := strings.TrimSpace(args[0])

	svc, err := dialService()
	if err != nil {
		return err
	}

	items, err := svc.FetchXdXr(code)
	if err != nil {
		return fmt.Errorf("获取除权除息数据失败: %w", err)
	}

	if xdxrJSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(items)
	}

	if len(items) == 0 {
		fmt.Printf("未找到 %s 的除权除息记录\n", code)
		return nil
	}

	fmt.Printf("除权除息记录 (%s):\n", code)
	for i, item := range items {
		if i >= 20 {
			fmt.Printf("  ... 共 %d 条记录，仅显示前 20 条\n", len(items))
			break
		}
		fmt.Printf("  %s  %s\n", item.Date.Format("2006-01-02"), item.Category)
		if item.FenHong > 0 {
			fmt.Printf("    分红: %.4f 元/股\n", item.FenHong)
		}
		if item.SongZhuanGu > 0 {
			fmt.Printf("    送转: %.2f 股/10股\n", item.SongZhuanGu)
		}
	}
	return nil
}
