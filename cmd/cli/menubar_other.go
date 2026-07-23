//go:build !darwin

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var menubarCmd = &cobra.Command{
	Use:   "menubar",
	Short: "启动 TongStock 菜单栏",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("menubar is currently supported on macOS only")
	},
}

func init() {
	rootCmd.AddCommand(menubarCmd)
}
