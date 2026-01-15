package main

import (
	"github.com/johann/ib/cmd/client/backup"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ib",
	Short: "Incremental backup tool",
	Long:  "ib is an incremental backup tool for efficiently backing up and restoring large directories.",
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(backup.Cmd)
}
