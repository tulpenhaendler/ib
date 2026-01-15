package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/johann/ib/internal/config"
	"github.com/johann/ib/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the backup server",
	Long:  "Start the backup server with API and web UI.",
	RunE:  runServe,
}

var (
	serveListenAddr  string
	serveMetricsPort int
	serveTitle       string
)

func init() {
	serveCmd.Flags().StringVar(&serveListenAddr, "listen", "", "Listen address (default from config or :8080)")
	serveCmd.Flags().IntVar(&serveMetricsPort, "metrics-port", 0, "Port for Prometheus metrics (disabled if 0)")
	serveCmd.Flags().StringVar(&serveTitle, "title", "ib Backup", "Title for the web UI")
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadServer()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if serveListenAddr != "" {
		cfg.ListenAddr = serveListenAddr
	}

	// Environment variable for title
	if v := os.Getenv("IB_TITLE"); v != "" && serveTitle == "ib Backup" {
		serveTitle = v
	}
	// Environment variable for metrics port
	if v := os.Getenv("IB_METRICS_PORT"); v != "" && serveMetricsPort == 0 {
		if port, err := strconv.Atoi(v); err == nil {
			serveMetricsPort = port
		}
	}

	// Validate required config
	if cfg.S3Bucket == "" {
		return fmt.Errorf("S3 bucket not configured. Run 'ib-server token show' and edit %s", configPath())
	}

	srv, err := server.New(cfg, serveMetricsPort, serveTitle)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	fmt.Printf("Starting server on %s\n", cfg.ListenAddr)
	if serveMetricsPort > 0 {
		fmt.Printf("Prometheus metrics on :%d\n", serveMetricsPort)
	}

	return srv.Run()
}

func configPath() string {
	dir, _ := config.Dir()
	return dir + "/server.json"
}
