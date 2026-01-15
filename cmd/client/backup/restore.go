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

var restoreCmd = &cobra.Command{
	Use:   "restore [flags] <output-path>",
	Short: "Restore a backup",
	Long: `Restore a backup to a directory.

Specify the backup to restore using either --id or --tag flags.
If using tags, the latest backup matching all tags will be restored.`,
	Args: cobra.ExactArgs(1),
	RunE: runRestore,
}

var (
	restoreID          string
	restoreTags        []string
	restoreConcurrency int
)

func init() {
	restoreCmd.Flags().StringVar(&restoreID, "id", "", "Manifest ID to restore")
	restoreCmd.Flags().StringArrayVar(&restoreTags, "tag", nil, "Restore latest backup matching tags (key=value format)")
	restoreCmd.Flags().IntVar(&restoreConcurrency, "concurrency", 4, "Number of concurrent download workers")
}

func runRestore(cmd *cobra.Command, args []string) error {
	outputPath := args[0]

	if restoreID == "" && len(restoreTags) == 0 {
		return fmt.Errorf("must specify either --id or --tag")
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

	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()

	// Fetch manifest
	var manifest *backup.Manifest
	if restoreID != "" {
		fmt.Printf("Fetching backup %s...\n", restoreID)
		manifest, err = c.GetManifest(ctx, restoreID)
		if err != nil {
			return fmt.Errorf("failed to fetch manifest: %w", err)
		}
	} else {
		tags := make(map[string]string)
		for _, t := range restoreTags {
			parts := strings.SplitN(t, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid tag format: %s (expected key=value)", t)
			}
			tags[parts[0]] = parts[1]
		}
		fmt.Printf("Fetching latest backup with tags %v...\n", tags)
		manifest, err = c.GetLatestManifest(ctx, tags)
		if err != nil {
			return fmt.Errorf("failed to fetch manifest: %w", err)
		}
		if manifest == nil {
			return fmt.Errorf("no backup found matching tags")
		}
	}

	fmt.Printf("Restoring backup %s to %s\n", manifest.ID, outputPath)
	fmt.Printf("Total entries: %d\n", len(manifest.Entries))
	fmt.Printf("Concurrency: %d workers\n", restoreConcurrency)

	// Create restorer with decompressing block fetcher
	fetcher := &decompressingFetcher{client: c}
	restorer := backup.NewRestorer(fetcher, restoreConcurrency)

	// Restore
	if err := restorer.Restore(ctx, manifest, outputPath); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	fmt.Println("Restore complete!")
	return nil
}

// decompressingFetcher wraps client to decompress blocks
type decompressingFetcher struct {
	client *client.Client
}

func (f *decompressingFetcher) DownloadBlock(ctx context.Context, cid string) ([]byte, error) {
	data, err := f.client.DownloadBlock(ctx, cid)
	if err != nil {
		return nil, err
	}
	// Note: The server should return decompressed data, or we need to track original size
	// For now, return as-is (server will handle decompression)
	return data, nil
}
