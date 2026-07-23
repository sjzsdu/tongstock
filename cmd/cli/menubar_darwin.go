//go:build darwin

package main

import (
	"github.com/sjzsdu/tongstock/internal/menubar"
	"github.com/spf13/cobra"
)

var menubarCmd = &cobra.Command{
	Use:   "menubar",
	Short: "启动 TongStock macOS 菜单栏",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		menubar.Run()
	},
}

func init() {
	rootCmd.AddCommand(menubarCmd)
}
