package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BlockFetcher is an interface for fetching blocks
type BlockFetcher interface {
	DownloadBlock(ctx context.Context, cid string) ([]byte, error)
}

// Restorer handles backup restoration
type Restorer struct {
	fetcher     BlockFetcher
	concurrency int
}

// NewRestorer creates a new restorer
func NewRestorer(fetcher BlockFetcher, concurrency int) *Restorer {
	return &Restorer{
		fetcher:     fetcher,
		concurrency: concurrency,
	}
}

// Restore restores a manifest to the given output path
func (r *Restorer) Restore(ctx context.Context, manifest *Manifest, outputPath string) error {
	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// First pass: create directories
	for _, entry := range manifest.Entries {
		if entry.Type == FileTypeDir {
			dirPath := filepath.Join(outputPath, entry.Path)
			if err := os.MkdirAll(dirPath, os.FileMode(entry.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", entry.Path, err)
			}
		}
	}

	// Second pass: restore files and symlinks
	for _, entry := range manifest.Entries {
		fullPath := filepath.Join(outputPath, entry.Path)

		switch entry.Type {
		case FileTypeFile:
			if err := r.restoreFile(ctx, &entry, fullPath); err != nil {
				return fmt.Errorf("failed to restore file %s: %w", entry.Path, err)
			}

		case FileTypeSymlink:
			if err := os.Symlink(entry.LinkTarget, fullPath); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", entry.Path, err)
			}
		}
	}

	// Third pass: restore permissions and timestamps
	for _, entry := range manifest.Entries {
		fullPath := filepath.Join(outputPath, entry.Path)

		if entry.Type != FileTypeSymlink {
			if err := os.Chmod(fullPath, os.FileMode(entry.Mode)); err != nil {
				fmt.Printf("Warning: failed to set permissions for %s: %v\n", entry.Path, err)
			}
		}

		// Set mtime (don't follow symlinks)
		mtime := time.Unix(0, entry.Mtime)
		if entry.Type == FileTypeSymlink {
			// For symlinks, we can't easily set mtime on all platforms
			continue
		}
		if err := os.Chtimes(fullPath, mtime, mtime); err != nil {
			fmt.Printf("Warning: failed to set mtime for %s: %v\n", entry.Path, err)
		}
	}

	return nil
}

func (r *Restorer) restoreFile(ctx context.Context, entry *Entry, outputPath string) error {
	if len(entry.Blocks) == 0 {
		// Empty file
		return os.WriteFile(outputPath, nil, os.FileMode(entry.Mode))
	}

	// Download and assemble blocks concurrently
	blocks := make([][]byte, len(entry.Blocks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(entry.Blocks))

	sem := make(chan struct{}, r.concurrency)

	for i, cid := range entry.Blocks {
		wg.Add(1)
		go func(idx int, blockCID string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			data, err := r.fetcher.DownloadBlock(ctx, blockCID)
			if err != nil {
				errChan <- fmt.Errorf("failed to download block %s: %w", blockCID, err)
				return
			}

			mu.Lock()
			blocks[idx] = data
			mu.Unlock()
		}(i, cid)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return err
	}

	// Create the file
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(entry.Mode))
	if err != nil {
		return err
	}
	defer file.Close()

	// Write blocks in order
	// Note: blocks are compressed, we need to track original sizes
	// For now, assume blocks are already decompressed by the client
	for _, block := range blocks {
		if _, err := file.Write(block); err != nil {
			return err
		}
	}

	return nil
}

// RestoreProgress represents progress information
type RestoreProgress struct {
	TotalFiles     int
	CompletedFiles int
	TotalBytes     int64
	DownloadedBytes int64
	CurrentFile    string
}
