package backup

import (
	"io/fs"
	"os"
	"path/filepath"
)

// ScanResult represents a scanned file entry
type ScanResult struct {
	Entry Entry
	Error error
}

// Scanner handles directory traversal with ignore file support
type Scanner struct {
	rootPath      string
	ignoreMatcher *IgnoreMatcher
}

// NewScanner creates a new scanner for the given root path
func NewScanner(rootPath string) *Scanner {
	return &Scanner{
		rootPath:      rootPath,
		ignoreMatcher: NewIgnoreMatcher(),
	}
}

// Scan traverses the directory and streams results via channel
func (s *Scanner) Scan() <-chan ScanResult {
	results := make(chan ScanResult, 100)

	go func() {
		defer close(results)

		// Load root-level ignore files
		s.ignoreMatcher.LoadFile(filepath.Join(s.rootPath, ".gitignore"))
		s.ignoreMatcher.LoadFile(filepath.Join(s.rootPath, ".ibignore"))

		err := filepath.WalkDir(s.rootPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				results <- ScanResult{Error: err}
				return nil // Continue walking
			}

			// Get relative path
			relPath, err := filepath.Rel(s.rootPath, path)
			if err != nil {
				results <- ScanResult{Error: err}
				return nil
			}

			// Skip root directory itself
			if relPath == "." {
				return nil
			}

			// Normalize to forward slashes for consistent matching
			relPath = filepath.ToSlash(relPath)

			info, err := d.Info()
			if err != nil {
				results <- ScanResult{Error: err}
				return nil
			}

			isDir := d.IsDir()

			// Check if ignored
			if s.ignoreMatcher.Match(relPath, isDir) {
				if isDir {
					return filepath.SkipDir
				}
				return nil
			}

			// Load nested ignore files for directories
			if isDir {
				gitignorePath := filepath.Join(path, ".gitignore")
				ibignorePath := filepath.Join(path, ".ibignore")
				s.ignoreMatcher.LoadFile(gitignorePath)
				s.ignoreMatcher.LoadFile(ibignorePath)
			}

			// Determine file type
			mode := info.Mode()
			var entry Entry

			switch {
			case mode.IsDir():
				entry = Entry{
					Path:  relPath,
					Type:  FileTypeDir,
					Mode:  uint32(mode.Perm()),
					Mtime: info.ModTime().UnixNano(),
				}

			case mode&os.ModeSymlink != 0:
				// Handle symlink - store target, don't follow
				target, err := os.Readlink(path)
				if err != nil {
					results <- ScanResult{Error: err}
					return nil
				}
				entry = Entry{
					Path:       relPath,
					Type:       FileTypeSymlink,
					Mode:       uint32(mode.Perm()),
					Mtime:      info.ModTime().UnixNano(),
					LinkTarget: target,
				}

			case mode.IsRegular():
				entry = Entry{
					Path:  relPath,
					Type:  FileTypeFile,
					Mode:  uint32(mode.Perm()),
					Mtime: info.ModTime().UnixNano(),
					Size:  info.Size(),
				}

			default:
				// Skip special files (sockets, devices, pipes)
				return nil
			}

			results <- ScanResult{Entry: entry}
			return nil
		})

		if err != nil {
			results <- ScanResult{Error: err}
		}
	}()

	return results
}

// IsSpecialFile checks if a file mode represents a special file
func IsSpecialFile(mode fs.FileMode) bool {
	return mode&(os.ModeSocket|os.ModeDevice|os.ModeNamedPipe|os.ModeCharDevice) != 0
}
