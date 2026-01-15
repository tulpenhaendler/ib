package backup

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// FileType represents the type of a file entry
type FileType string

const (
	FileTypeFile    FileType = "file"
	FileTypeDir     FileType = "dir"
	FileTypeSymlink FileType = "symlink"
)

// Manifest represents a backup manifest
type Manifest struct {
	ID        string            `json:"id"`
	Tags      map[string]string `json:"tags"`
	CreatedAt time.Time         `json:"created_at"`
	RootPath  string            `json:"root_path"`
	Entries   []Entry           `json:"entries"`
}

// Entry represents a single file/directory/symlink in a manifest
type Entry struct {
	Path       string   `json:"path"`                  // Relative path from backup root
	Type       FileType `json:"type"`                  // file, dir, symlink
	Mode       uint32   `json:"mode"`                  // Unix permissions
	Mtime      int64    `json:"mtime"`                 // Unix timestamp (nanoseconds)
	Size       int64    `json:"size,omitempty"`        // Original size (files only)
	Blocks     []string `json:"blocks,omitempty"`      // CID list (files only)
	LinkTarget string   `json:"link_target,omitempty"` // Symlink target (symlinks only)
}

// Block represents a content-addressed data block
type Block struct {
	CID          string `json:"cid"`
	Data         []byte `json:"-"` // Compressed data, not serialized
	Size         int64  `json:"size"`
	OriginalSize int64  `json:"original_size"`
}

// NewManifest creates a new manifest with the given tags
func NewManifest(tags map[string]string, rootPath string) *Manifest {
	return &Manifest{
		ID:        generateID(),
		Tags:      tags,
		CreatedAt: time.Now().UTC(),
		RootPath:  rootPath,
		Entries:   make([]Entry, 0),
	}
}

// AddEntry adds an entry to the manifest
func (m *Manifest) AddEntry(entry Entry) {
	m.Entries = append(m.Entries, entry)
}

// FindEntry finds an entry by path
func (m *Manifest) FindEntry(path string) *Entry {
	for i := range m.Entries {
		if m.Entries[i].Path == path {
			return &m.Entries[i]
		}
	}
	return nil
}

// BuildEntryIndex creates a map for fast entry lookups by path
func (m *Manifest) BuildEntryIndex() map[string]*Entry {
	index := make(map[string]*Entry, len(m.Entries))
	for i := range m.Entries {
		index[m.Entries[i].Path] = &m.Entries[i]
	}
	return index
}

func generateID() string {
	return time.Now().UTC().Format("20060102-150405") + "-" + randomSuffix()
}

func randomSuffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
