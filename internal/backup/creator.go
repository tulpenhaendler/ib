package backup

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// BlockUploader is an interface for checking and uploading blocks
type BlockUploader interface {
	BlockExists(ctx context.Context, cid string) (bool, error)
	UploadBlock(ctx context.Context, cid string, data []byte, originalSize int64) error
}

// Creator handles backup creation
type Creator struct {
	uploader    BlockUploader
	concurrency int
	chunker     *Chunker
}

// NewCreator creates a new backup creator
func NewCreator(uploader BlockUploader, concurrency int) *Creator {
	return &Creator{
		uploader:    uploader,
		concurrency: concurrency,
		chunker:     NewChunker(),
	}
}

// CreateProgress represents backup creation progress
type CreateProgress struct {
	TotalFiles      int64
	ProcessedFiles  int64
	SkippedFiles    int64
	TotalBytes      int64
	UploadedBytes   int64
	CurrentFile     string
}

// Create creates a backup of the given path with the specified tags
func (c *Creator) Create(ctx context.Context, rootPath string, tags map[string]string, prevManifest *Manifest) (*Manifest, error) {
	// Build index of previous manifest for incremental backup
	var prevIndex map[string]*Entry
	if prevManifest != nil {
		prevIndex = prevManifest.BuildEntryIndex()
	}

	// Create new manifest
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}
	manifest := NewManifest(tags, absPath)

	// Scan directory
	scanner := NewScanner(rootPath)
	scanResults := scanner.Scan()

	// Collect all entries first
	var entries []Entry
	for result := range scanResults {
		if result.Error != nil {
			fmt.Printf("Warning: scan error: %v\n", result.Error)
			continue
		}
		entries = append(entries, result.Entry)
	}

	// Process files concurrently
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.concurrency)
	errChan := make(chan error, 1)
	var firstErr error
	var errOnce sync.Once

	var processedFiles int64

	for i := range entries {
		entry := &entries[i]

		if entry.Type != FileTypeFile {
			// Add directories and symlinks directly
			manifest.AddEntry(*entry)
			continue
		}

		// Check if file changed since last backup
		if prevIndex != nil {
			if prevEntry, ok := prevIndex[entry.Path]; ok {
				if prevEntry.Mtime == entry.Mtime && prevEntry.Size == entry.Size {
					// File unchanged, reuse blocks from previous manifest
					entry.Blocks = prevEntry.Blocks
					manifest.AddEntry(*entry)
					atomic.AddInt64(&processedFiles, 1)
					continue
				}
			}
		}

		// Process file
		wg.Add(1)
		go func(e *Entry) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			// Check for context cancellation
			select {
			case <-ctx.Done():
				errOnce.Do(func() { firstErr = ctx.Err() })
				return
			default:
			}

			fullPath := filepath.Join(rootPath, e.Path)
			chunks := c.chunker.ChunkFile(fullPath)

			var blocks []string
			for chunk := range chunks {
				if chunk.Error != nil {
					errOnce.Do(func() { firstErr = chunk.Error })
					return
				}

				// Check if block exists on server
				exists, err := c.uploader.BlockExists(ctx, chunk.CID)
				if err != nil {
					errOnce.Do(func() { firstErr = err })
					return
				}

				if !exists {
					// Upload the block
					if err := c.uploader.UploadBlock(ctx, chunk.CID, chunk.Data, chunk.OriginalSize); err != nil {
						errOnce.Do(func() { firstErr = err })
						return
					}
				}

				blocks = append(blocks, chunk.CID)
			}

			e.Blocks = blocks
			atomic.AddInt64(&processedFiles, 1)
		}(entry)
	}

	wg.Wait()
	close(errChan)

	if firstErr != nil {
		return nil, firstErr
	}

	// Add all file entries to manifest
	for _, entry := range entries {
		if entry.Type == FileTypeFile {
			manifest.AddEntry(entry)
		}
	}

	return manifest, nil
}
