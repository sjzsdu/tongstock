package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
)

var (
	companyJSON   bool
	companyBlock  string
	companyOutput bool
)

// companyCmd 获取个股 F10 公司信息目录
var companyCmd = &cobra.Command{
	Use:   "company <code>",
	Short: "查看个股 F10 公司信息目录",
	Long: `获取指定股票的 F10 公司信息分类目录，包括公司概况、财务分析、股东研究等。

示例:
  tongstock company 000001
  tongstock company 600519 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runCompany,
}

func init() {
	companyCmd.Flags().BoolVar(&companyJSON, "json", false, "以 JSON 输出")
}

func runCompany(cmd *cobra.Command, args []string) error {
	code := strings.TrimSpace(args[0])

	svc, err := dialService()
	if err != nil {
		return err
	}

	cats, err := svc.FetchCompanyCategory(code)
	if err != nil {
		return fmt.Errorf("获取 F10 目录失败: %w", err)
	}

	if companyJSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(cats)
	}

	if len(cats) == 0 {
		fmt.Printf("未找到 %s 的 F10 信息\n", code)
		return nil
	}

	fmt.Printf("F10 信息目录 (%s):\n", code)
	for i, cat := range cats {
		fmt.Printf("  %2d. %s\n", i+1, cat.Name)
	}
	fmt.Printf("\n使用 tongstock company-content %s --block \"<名称>\" 查看具体内容\n", code)
	return nil
}

// companyContentCmd 获取个股 F10 公司信息内容
var companyContentCmd = &cobra.Command{
	Use:   "company-content <code>",
	Short: "查看个股 F10 公司信息内容",
	Long: `获取指定股票的 F10 公司信息具体内容。

示例:
  tongstock company-content 000001 --block "公司概况"
  tongstock company-content 600519 --block "财务分析"
  tongstock company-content 000858 --block "股东研究"`,
	Args: cobra.ExactArgs(1),
	RunE: runCompanyContent,
}

func init() {
	companyContentCmd.Flags().StringVar(&companyBlock, "block", "", "信息块名称（如：公司概况、财务分析、股东研究）")
	companyContentCmd.Flags().BoolVar(&companyOutput, "json", false, "以 JSON 输出")
}

func runCompanyContent(cmd *cobra.Command, args []string) error {
	code := strings.TrimSpace(args[0])
	block := strings.TrimSpace(companyBlock)

	if block == "" {
		return fmt.Errorf("请指定 --block 参数，例如: --block \"公司概况\"")
	}

	svc, err := dialService()
	if err != nil {
		return err
	}

	// 获取目录找到对应块的文件信息
	cats, err := svc.FetchCompanyCategory(code)
	if err != nil {
		return fmt.Errorf("获取 F10 目录失败: %w", err)
	}

	var target *protocol.CompanyCategoryItem
	for _, cat := range cats {
		if cat.Name == block {
			target = cat
			break
		}
	}

	if target == nil {
		// 列出可用块
		var available []string
		for _, cat := range cats {
			available = append(available, cat.Name)
		}
		return fmt.Errorf("未找到块: %s\n可用的块: %s", block, strings.Join(available, "、"))
	}

	content, err := svc.FetchCompanyContent(code, target.Filename, target.Start, target.Length)
	if err != nil {
		return fmt.Errorf("获取 F10 内容失败: %w", err)
	}

	if companyOutput {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(map[string]string{
			"code":    code,
			"block":   block,
			"content": content,
		})
	}

	fmt.Printf("=== %s - %s ===\n\n", code, block)
	fmt.Println(content)
	return nil
}
