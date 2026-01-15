package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/johann/ib/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize server configuration",
	Long:  "Interactive wizard to configure the server settings.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("ib-server configuration wizard")
	fmt.Println("==============================")
	fmt.Println()

	// Load existing config or create default
	cfg, _ := config.LoadServer()
	if cfg == nil {
		cfg = config.DefaultServerConfig()
	}

	// S3 Configuration
	fmt.Println("S3 Storage Configuration")
	fmt.Println("------------------------")

	cfg.S3Endpoint = prompt(reader, "S3 Endpoint URL", cfg.S3Endpoint, "http://localhost:9000")
	cfg.S3Bucket = prompt(reader, "S3 Bucket Name", cfg.S3Bucket, "ib-backups")
	cfg.S3AccessKey = prompt(reader, "S3 Access Key", cfg.S3AccessKey, "")
	cfg.S3SecretKey = promptSecret(reader, "S3 Secret Key", cfg.S3SecretKey)
	cfg.S3Region = prompt(reader, "S3 Region", cfg.S3Region, "us-east-1")

	fmt.Println()

	// Server Configuration
	fmt.Println("Server Configuration")
	fmt.Println("--------------------")

	cfg.ListenAddr = prompt(reader, "HTTP Listen Address", cfg.ListenAddr, ":8080")
	cfg.DBPath = prompt(reader, "Database Path", cfg.DBPath, "")

	retentionStr := prompt(reader, "Retention Days", fmt.Sprintf("%d", cfg.RetentionDays), "90")
	if days, err := strconv.Atoi(retentionStr); err == nil {
		cfg.RetentionDays = days
	}

	fmt.Println()

	// IPFS Configuration
	fmt.Println("IPFS Configuration")
	fmt.Println("------------------")

	enableIPFS := promptYesNo(reader, "Enable IPFS support?", cfg.IPFSEnabled)
	cfg.IPFSEnabled = enableIPFS

	if enableIPFS {
		cfg.IPFSGatewayAddr = prompt(reader, "IPFS Gateway Address", cfg.IPFSGatewayAddr, ":8081")
		fmt.Println()
		fmt.Println("IPFS will also listen on:")
		fmt.Println("  - TCP  :4001 (libp2p)")
		fmt.Println("  - UDP  :4001 (QUIC)")
	}

	fmt.Println()

	// Token
	fmt.Println("Authentication")
	fmt.Println("--------------")

	if cfg.Token == "" {
		token, err := generateToken()
		if err != nil {
			return fmt.Errorf("failed to generate token: %w", err)
		}
		cfg.Token = token
		fmt.Printf("Generated new token: %s\n", token)
	} else {
		if promptYesNo(reader, "Regenerate authentication token?", false) {
			token, err := generateToken()
			if err != nil {
				return fmt.Errorf("failed to generate token: %w", err)
			}
			cfg.Token = token
			fmt.Printf("New token: %s\n", token)
		} else {
			fmt.Printf("Keeping existing token: %s\n", cfg.Token)
		}
	}

	fmt.Println()

	// Save configuration
	if err := config.SaveServer(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	configDir, _ := config.Dir()
	fmt.Println("Configuration saved!")
	fmt.Printf("Config file: %s/server.json\n", configDir)
	fmt.Println()
	fmt.Println("Start the server with:")
	fmt.Println("  ib-server serve")

	return nil
}

func prompt(reader *bufio.Reader, label, current, defaultVal string) string {
	displayDefault := current
	if displayDefault == "" {
		displayDefault = defaultVal
	}

	if displayDefault != "" {
		fmt.Printf("%s [%s]: ", label, displayDefault)
	} else {
		fmt.Printf("%s: ", label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		if current != "" {
			return current
		}
		return defaultVal
	}
	return input
}

func promptSecret(reader *bufio.Reader, label, current string) string {
	if current != "" {
		fmt.Printf("%s [****hidden****]: ", label)
	} else {
		fmt.Printf("%s: ", label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return current
	}
	return input
}

func promptYesNo(reader *bufio.Reader, label string, defaultVal bool) bool {
	defaultStr := "y/N"
	if defaultVal {
		defaultStr = "Y/n"
	}

	fmt.Printf("%s [%s]: ", label, defaultStr)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultVal
	}

	return input == "y" || input == "yes"
}
