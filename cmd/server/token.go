package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/johann/ib/internal/config"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage authentication tokens",
}

var tokenShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show or generate authentication token",
	Long:  "Show the current authentication token, or generate one if it doesn't exist.",
	RunE:  runTokenShow,
}

func init() {
	tokenCmd.AddCommand(tokenShowCmd)
}

func runTokenShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadServer()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Token == "" {
		token, err := generateToken()
		if err != nil {
			return fmt.Errorf("failed to generate token: %w", err)
		}
		cfg.Token = token

		if err := config.SaveServer(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println("Generated new token:")
	}

	fmt.Println(cfg.Token)
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
