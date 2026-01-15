package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
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

// Progress tracks backup creation progress
type Progress struct {
	TotalFiles     int64
	ProcessedFiles int64
	SkippedFiles   int64 // Files unchanged from previous backup
	ErrorFiles     int64 // Files skipped due to errors (permission denied, etc)
	TotalBytes     int64
	UploadedBytes  int64
	SkippedBytes   int64 // Bytes from blocks that already existed
	BlocksUploaded int64
	BlocksSkipped  int64 // Blocks that already existed on server
	CurrentFile    atomic.Value
	StartTime      time.Time
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

	// Initialize progress tracking
	progress := &Progress{
		StartTime: time.Now(),
	}
	progress.CurrentFile.Store("")

	// Start progress reporter
	progressCtx, cancelProgress := context.WithCancel(ctx)
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		c.reportProgress(progressCtx, progress)
	}()

	// Scan directory
	fmt.Println("Scanning directory...")
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
		if result.Entry.Type == FileTypeFile {
			atomic.AddInt64(&progress.TotalFiles, 1)
			atomic.AddInt64(&progress.TotalBytes, result.Entry.Size)
		}
	}

	fmt.Printf("Found %d files (%s total)\n", progress.TotalFiles, formatBytes(progress.TotalBytes))

	// Process files concurrently
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.concurrency)
	var firstErr error
	var errOnce sync.Once

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
					atomic.AddInt64(&progress.ProcessedFiles, 1)
					atomic.AddInt64(&progress.SkippedFiles, 1)
					atomic.AddInt64(&progress.SkippedBytes, entry.Size)
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

			// Check for context cancellation or previous error
			select {
			case <-ctx.Done():
				errOnce.Do(func() { firstErr = ctx.Err() })
				return
			default:
			}

			if firstErr != nil {
				return
			}

			progress.CurrentFile.Store(e.Path)

			fullPath := filepath.Join(rootPath, e.Path)
			chunks := c.chunker.ChunkFile(fullPath)

			var blocks []string
			var fileUploadedBytes int64
			var fileSkippedBytes int64
			var fileError error

			for chunk := range chunks {
				if chunk.Error != nil {
					// Check if this is a permission error - skip file instead of failing
					if os.IsPermission(chunk.Error) {
						fileError = chunk.Error
						// Drain remaining chunks
						for range chunks {
						}
						break
					}
					errOnce.Do(func() { firstErr = fmt.Errorf("chunking %s: %w", e.Path, chunk.Error) })
					return
				}

				// Check if block exists on server
				exists, err := c.uploader.BlockExists(ctx, chunk.CID)
				if err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("checking block %s: %w", chunk.CID[:12], err) })
					return
				}

				if !exists {
					// Upload the block
					if err := c.uploader.UploadBlock(ctx, chunk.CID, chunk.Data, chunk.OriginalSize); err != nil {
						errOnce.Do(func() { firstErr = fmt.Errorf("uploading block %s: %w", chunk.CID[:12], err) })
						return
					}
					atomic.AddInt64(&progress.BlocksUploaded, 1)
					fileUploadedBytes += int64(len(chunk.Data))
				} else {
					atomic.AddInt64(&progress.BlocksSkipped, 1)
					fileSkippedBytes += chunk.OriginalSize
				}

				blocks = append(blocks, chunk.CID)
			}

			// Handle files that couldn't be read
			if fileError != nil {
				fmt.Printf("Warning: skipping %s: %v\n", e.Path, fileError)
				atomic.AddInt64(&progress.ErrorFiles, 1)
				atomic.AddInt64(&progress.ProcessedFiles, 1)
				e.Blocks = nil // Mark as unreadable
				return
			}

			e.Blocks = blocks
			atomic.AddInt64(&progress.ProcessedFiles, 1)
			atomic.AddInt64(&progress.UploadedBytes, fileUploadedBytes)
			atomic.AddInt64(&progress.SkippedBytes, fileSkippedBytes)
		}(entry)
	}

	wg.Wait()

	// Stop progress reporter
	cancelProgress()
	<-progressDone

	// Print final progress
	c.printFinalProgress(progress)

	if firstErr != nil {
		return nil, firstErr
	}

	// Add all file entries to manifest (skip files that had errors)
	for _, entry := range entries {
		if entry.Type == FileTypeFile && entry.Blocks != nil {
			manifest.AddEntry(entry)
		}
	}

	return manifest, nil
}

func (c *Creator) reportProgress(ctx context.Context, p *Progress) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastUploadedBytes int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processed := atomic.LoadInt64(&p.ProcessedFiles)
			total := atomic.LoadInt64(&p.TotalFiles)
			uploaded := atomic.LoadInt64(&p.UploadedBytes)
			skipped := atomic.LoadInt64(&p.SkippedBytes)
			blocksUploaded := atomic.LoadInt64(&p.BlocksUploaded)
			blocksSkipped := atomic.LoadInt64(&p.BlocksSkipped)
			skippedFiles := atomic.LoadInt64(&p.SkippedFiles)
			errorFiles := atomic.LoadInt64(&p.ErrorFiles)
			currentFile, _ := p.CurrentFile.Load().(string)

			elapsed := time.Since(p.StartTime)
			speed := float64(uploaded-lastUploadedBytes) / 5.0 // bytes per second over last interval
			lastUploadedBytes = uploaded

			// Calculate percentage
			var pct float64
			if total > 0 {
				pct = float64(processed) / float64(total) * 100
			}

			fmt.Printf("\n[%s] Progress: %d/%d files (%.1f%%)\n",
				elapsed.Round(time.Second), processed, total, pct)
			fmt.Printf("  Uploaded: %s (%d blocks) | Dedup: %s (%d blocks)\n",
				formatBytes(uploaded), blocksUploaded,
				formatBytes(skipped), blocksSkipped)
			if skippedFiles > 0 {
				fmt.Printf("  Unchanged files: %d (reused from previous backup)\n", skippedFiles)
			}
			if errorFiles > 0 {
				fmt.Printf("  Skipped files: %d (permission denied or unreadable)\n", errorFiles)
			}
			if speed > 0 {
				fmt.Printf("  Speed: %s/s\n", formatBytes(int64(speed)))
			}
			if currentFile != "" {
				displayPath := currentFile
				if len(displayPath) > 60 {
					displayPath = "..." + displayPath[len(displayPath)-57:]
				}
				fmt.Printf("  Current: %s\n", displayPath)
			}
		}
	}
}

func (c *Creator) printFinalProgress(p *Progress) {
	elapsed := time.Since(p.StartTime)
	processed := atomic.LoadInt64(&p.ProcessedFiles)
	total := atomic.LoadInt64(&p.TotalFiles)
	uploaded := atomic.LoadInt64(&p.UploadedBytes)
	skipped := atomic.LoadInt64(&p.SkippedBytes)
	blocksUploaded := atomic.LoadInt64(&p.BlocksUploaded)
	blocksSkipped := atomic.LoadInt64(&p.BlocksSkipped)
	skippedFiles := atomic.LoadInt64(&p.SkippedFiles)
	errorFiles := atomic.LoadInt64(&p.ErrorFiles)

	fmt.Printf("\n=== Backup Complete ===\n")
	fmt.Printf("Duration: %s\n", elapsed.Round(time.Second))
	fmt.Printf("Files: %d/%d processed\n", processed, total)
	if skippedFiles > 0 {
		fmt.Printf("  - %d unchanged (reused from previous backup)\n", skippedFiles)
	}
	if errorFiles > 0 {
		fmt.Printf("  - %d skipped (permission denied)\n", errorFiles)
	}
	actualProcessed := processed - skippedFiles - errorFiles
	if actualProcessed > 0 {
		fmt.Printf("  - %d new/modified\n", actualProcessed)
	}
	fmt.Printf("Data: %s uploaded, %s deduplicated\n",
		formatBytes(uploaded), formatBytes(skipped))
	fmt.Printf("Blocks: %d uploaded, %d already existed\n", blocksUploaded, blocksSkipped)
	if elapsed.Seconds() > 0 && uploaded > 0 {
		avgSpeed := float64(uploaded) / elapsed.Seconds()
		fmt.Printf("Average speed: %s/s\n", formatBytes(int64(avgSpeed)))
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
