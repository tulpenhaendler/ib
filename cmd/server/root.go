package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ib-server",
	Short: "Incremental backup server",
	Long:  "ib-server is the server component for the incremental backup tool.",
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(tokenCmd)
}
