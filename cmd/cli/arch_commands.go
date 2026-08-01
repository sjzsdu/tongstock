package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sjzsdu/tongstock/internal/archguard"
	"github.com/spf13/cobra"
)

var archCmd = &cobra.Command{
	Use:   "arch",
	Short: "架构质量门: 依赖方向、循环引用、死代码、重复领域类型、Mock/合成结果扫描",
}

var archCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "执行架构质量门检查；任何门失败都返回非零退出码",
	RunE:  runArchCheck,
}

func init() {
	archCmd.AddCommand(archCheckCmd)
	archCheckCmd.Flags().Bool("json", false, "输出 JSON 格式")
	archCheckCmd.Flags().String("root", ".", "Go 仓库根目录")
}

func runArchCheck(cmd *cobra.Command, args []string) error {
	root, _ := cmd.Flags().GetString("root")
	asJSON, _ := cmd.Flags().GetBool("json")
	rep := archguard.Run(archguard.WithGoRoot(root))
	if asJSON {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		_, _ = os.Stdout.Write(data)
		_, _ = os.Stdout.Write([]byte{'\n'})
	} else {
		fmt.Fprint(os.Stdout, rep.Text())
	}
	if !rep.Passed {
		os.Exit(1)
	}
	return nil
}
