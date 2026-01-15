package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/johann/ib/internal/client"
	"github.com/johann/ib/internal/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups",
	Long:  "List all available backups, optionally filtered by tags.",
	RunE:  runList,
}

var listTags []string

func init() {
	listCmd.Flags().StringArrayVar(&listTags, "tag", nil, "Filter by tag in key=value format (can be repeated)")
}

func runList(cmd *cobra.Command, args []string) error {
	tags := make(map[string]string)
	for _, t := range listTags {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid tag format: %s (expected key=value)", t)
		}
		tags[parts[0]] = parts[1]
	}

	// Load client config
	cfg, err := config.LoadClient()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create client
	c, err := client.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch manifests
	manifests, err := c.ListManifests(ctx, tags)
	if err != nil {
		return fmt.Errorf("failed to list manifests: %w", err)
	}

	if len(manifests) == 0 {
		fmt.Println("No backups found")
		return nil
	}

	fmt.Printf("Found %d backup(s):\n\n", len(manifests))
	for _, m := range manifests {
		fmt.Printf("ID: %s\n", m.ID)
		fmt.Printf("  Created: %s\n", m.CreatedAt.Format(time.RFC3339))
		if len(m.Tags) > 0 {
			fmt.Printf("  Tags: ")
			first := true
			for k, v := range m.Tags {
				if !first {
					fmt.Printf(", ")
				}
				fmt.Printf("%s=%s", k, v)
				first = false
			}
			fmt.Println()
		}
		fmt.Println()
	}

	return nil
}
