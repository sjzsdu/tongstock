package main

import (
	"github.com/sjzsdu/tongstock/internal/serverapp"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动 TongStock HTTP 服务",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		serverapp.Run()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
