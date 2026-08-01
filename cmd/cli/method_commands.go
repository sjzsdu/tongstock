package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/spf13/cobra"
)

// methodCmd 暴露统一投资方法模型 (Unified Investment Method) 能力:
//   - compile: 将结构化候选 JSON 编译为稳定 AST + 诊断
//   - explain: 解释编译诊断/歧义/可执行性
//   - execute: 在 K 线上执行入场/出场规则 (生产集成点)
//   - suggest: 基于诊断给出候选修复建议
var methodCmd = &cobra.Command{
	Use:   "method",
	Short: "统一投资方法模型: 编译/解释/执行规则、哈希版本化",
	Long: `统一投资方法模型 (Unified Investment Method, UIM) 是所有策略的唯一入口。
任何自然语言、结构化规则或代码策略最终都被编译为 methods.CompiledMethod,
并通过确定性 executor 执行, 确保同一版本在任何环境下结果一致。`,
}

var methodCompileCmd = &cobra.Command{
	Use:   "compile [candidate.json]",
	Short: "将结构化候选编译为稳定 CompiledMethod JSON",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var c *methods.Candidate
		if len(args) == 0 {
			c = methods.DemoBreakout()
		} else {
			b, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			c = &methods.Candidate{}
			if err := json.Unmarshal(b, c); err != nil {
				return fmt.Errorf("candidate JSON 解析失败: %w", err)
			}
		}
		m, diags, err := methods.Compile(c)
		if err != nil {
			return fmt.Errorf("编译失败: %w", err)
		}
		pretty, _ := json.MarshalIndent(map[string]any{
			"method":        m,
			"diagnostics":   diags,
			"is_executable": m.IsExecutable(),
			"content_hash":  m.ContentHash,
			"compiler":      m.CompilerVersion,
			"human_version": methods.FormatVersion(m),
		}, "", "  ")
		fmt.Println(string(pretty))
		return nil
	},
}

var methodExplainCmd = &cobra.Command{
	Use:   "explain [compiled.json]",
	Short: "解释 CompiledMethod 的诊断、歧义与修复建议",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var m *methods.CompiledMethod
		if len(args) == 0 {
			c := methods.DemoBreakout()
			var err error
			m, _, err = methods.Compile(c)
			if err != nil {
				return err
			}
		} else {
			b, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			m = &methods.CompiledMethod{}
			if err := json.Unmarshal(b, m); err != nil {
				return fmt.Errorf("compiled JSON 解析失败: %w", err)
			}
		}
		fmt.Println(methods.ExplainDiagnostics(m))
		suggestions := methods.SuggestFix(m)
		if len(suggestions) > 0 {
			fmt.Println("\n## 修复建议")
			for i, s := range suggestions {
				fmt.Printf("  %d. %s\n", i+1, s)
			}
		}
		return nil
	},
}

var methodExecuteEntryCmd = &cobra.Command{
	Use:   "entry <compiled.json> <kline.json>",
	Short: "在 K 线历史上执行入场规则, 打印匹配日期和 trace",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		b1, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		m := &methods.CompiledMethod{}
		if err := json.Unmarshal(b1, m); err != nil {
			return err
		}
		b2, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		var bars []methods.Bar
		if err := json.Unmarshal(b2, &bars); err != nil {
			return err
		}
		for i, bar := range bars {
			r, err := m.Entry(bar, bars[:i+1])
			if err != nil {
				return err
			}
			if r.Matched {
				fmt.Printf("ENTRY MATCH %s:\n%s\n", bar.Date, methods.ExplainTrace(r.Trace))
			}
		}
		return nil
	},
}

func init() {
	methodCmd.AddCommand(methodCompileCmd)
	methodCmd.AddCommand(methodExplainCmd)
	methodCmd.AddCommand(methodExecuteEntryCmd)
	rootCmd.AddCommand(methodCmd)
}
