package main

import (
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/spf13/cobra"
	"strings"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "同步本地行情数据",
}

var (
	syncCodes       string
	syncMode        string
	syncConcurrency int
)

var syncDailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "同步指定代码的日K数据",
	RunE: func(cmd *cobra.Command, args []string) error {
		codes := splitCodes(syncCodes)
		if len(codes) == 0 {
			return fmt.Errorf("请通过 --codes 指定股票代码")
		}
		svc, err := dialService()
		if err != nil {
			return fmt.Errorf("连接服务器失败: %w", err)
		}
		defer svc.Close()

		result := svc.SyncDailyKlines(codes, tdx.SyncMode(syncMode), syncConcurrency)
		fmt.Printf("同步完成: 总数 %d, 成功 %d, 失败 %d\n", result.Total, result.Success, result.Failed)
		for _, item := range result.Results {
			if item.Status == "ok" {
				last := ""
				rows := 0
				if item.State != nil {
					last = item.State.LastDate
					rows = item.State.RowCount
				}
				fmt.Printf("OK %s rows=%d last=%s\n", item.Code, rows, last)
			} else {
				fmt.Printf("FAIL %s %s\n", item.Code, item.Error)
			}
		}
		if result.Failed > 0 {
			return fmt.Errorf("部分股票同步失败")
		}
		return nil
	},
}

func init() {
	syncCmd.AddCommand(syncDailyCmd)
	syncDailyCmd.Flags().StringVar(&syncCodes, "codes", "", "股票代码列表，逗号/空格分隔")
	syncDailyCmd.Flags().StringVar(&syncMode, "mode", "auto", "同步模式: auto/full/incremental")
	syncDailyCmd.Flags().IntVar(&syncConcurrency, "concurrency", 3, "并发数")
}

func splitCodes(raw string) []string {
	seen := map[string]bool{}
	var codes []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		code := strings.TrimSpace(part)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		codes = append(codes, code)
	}
	return codes
}

// Indicator command and screen command
