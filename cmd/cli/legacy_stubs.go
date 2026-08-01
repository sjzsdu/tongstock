package main

import (
	"strings"

	"github.com/spf13/cobra"
)

// ===== CLI command stubs: keep the Cobra root register list in main.go valid.
// These are intentionally minimal placeholders; production code for the
// referenced commands is expected to be provided by separate packages.

var klineCmd = &cobra.Command{Use: "kline", Short: "查询K线数据（占位）", RunE: func(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}}

var minuteCmd = &cobra.Command{Use: "minute", Short: "查询分时数据（占位）", RunE: func(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}}

var tradeCmd = &cobra.Command{Use: "trade", Short: "查询分笔成交（占位）", RunE: func(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}}

var xdxrCmd = &cobra.Command{Use: "xdxr", Short: "查询除权除息（占位）", RunE: func(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}}

var financeCmd = &cobra.Command{Use: "finance", Short: "查询财务数据（占位）", RunE: func(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}}

var indexCmd = &cobra.Command{Use: "index", Short: "查询指数K线（占位）", RunE: func(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}}

var (
	companyCode string
	companyKind string
)

var companyCmd = &cobra.Command{
	Use:   "company",
	Short: "查询公司信息（占位）",
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

var (
	companyContentStart  uint32
	companyContentLength uint32
	companyContentBlock  string
)

var companyContentCmd = &cobra.Command{
	Use:   "company-content [code]",
	Short: "查询公司F10内容（占位）",
	Args:  cobra.MaximumNArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

func init() {
	companyCmd.Flags().StringVarP(&companyCode, "code", "c", "", "股票代码")
	companyCmd.Flags().StringVarP(&companyKind, "kind", "k", "overview", "信息类型: overview / finance / holder / ...")
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
