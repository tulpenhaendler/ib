package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/johann/ib/internal/backup"
	"github.com/johann/ib/internal/client"
	"github.com/johann/ib/internal/config"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create [flags] <path>",
	Short: "Create a new backup",
	Long: `Create a new backup of a directory.

The 'name' tag is required to identify and group related backups.
Additional tags can be specified as key=value pairs using the --tag flag.

Example: ib backup create --tag name=myapp --tag env=prod ./data`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

var (
	createTags        []string
	createConcurrency int
)

func init() {
	createCmd.Flags().StringArrayVar(&createTags, "tag", nil, "Tag in key=value format (can be repeated)")
	createCmd.Flags().IntVar(&createConcurrency, "concurrency", 16, "Number of concurrent upload workers")
}

func runCreate(cmd *cobra.Command, args []string) error {
	path := args[0]

	tags := make(map[string]string)
	for _, t := range createTags {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid tag format: %s (expected key=value)", t)
		}
		tags[parts[0]] = parts[1]
	}

	// Require the name tag for organizing backups
	if tags["name"] == "" {
		return fmt.Errorf("the 'name' tag is required: use --tag name=<backup-name>")
	}

	fmt.Printf("Creating backup: %s\n", tags["name"])
	fmt.Printf("Path: %s\n", path)
	fmt.Printf("Tags: %v\n", tags)
	fmt.Printf("Concurrency: %d workers\n", createConcurrency)
	fmt.Println()

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

	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()

	// Fetch previous manifest for incremental backup
	var prevManifest *backup.Manifest
	fmt.Println("Checking for previous backup...")
	prevManifest, err = c.GetLatestManifest(ctx, tags)
	if err != nil {
		fmt.Printf("Warning: could not fetch previous manifest: %v\n", err)
	} else if prevManifest != nil {
		fmt.Printf("Found previous backup: %s (will use for incremental)\n", prevManifest.ID)
	} else {
		fmt.Println("No previous backup found, creating full backup")
	}
	fmt.Println()

	// Create backup
	creator := backup.NewCreator(c, createConcurrency)
	manifest, err := creator.Create(ctx, path, tags, prevManifest)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Upload manifest
	fmt.Println("\nUploading manifest...")
	if err := c.UploadManifest(ctx, manifest); err != nil {
		return fmt.Errorf("failed to upload manifest: %w", err)
	}

	fmt.Printf("\nManifest ID: %s\n", manifest.ID)
	fmt.Printf("Total entries: %d\n", len(manifest.Entries))

	return nil
}
