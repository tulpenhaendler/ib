package main

import (
	"fmt"

	"github.com/johann/ib/internal/config"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [server-url]",
	Short: "Login to a backup server",
	Long:  "Login to a backup server. Token is optional for download-only access.",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogin,
}

var loginToken string

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "Authentication token for uploads")
}

func runLogin(cmd *cobra.Command, args []string) error {
	serverURL := args[0]

	cfg, err := config.LoadClient()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cfg.ServerURL = serverURL
	if loginToken != "" {
		cfg.Token = loginToken
	}

	if err := config.SaveClient(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if loginToken != "" {
		fmt.Printf("Logged in to %s with authentication token\n", serverURL)
	} else {
		fmt.Printf("Logged in to %s (download-only, no token provided)\n", serverURL)
	}

	return nil
}
