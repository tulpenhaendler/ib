package ipfsnode

import (
	"context"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/johann/ib/internal/backup"
)

// NodeCollector wraps a NodeSaver to collect saved node CIDs
type NodeCollector struct {
	saver    NodeSaver
	mu       sync.Mutex
	nodeCIDs []string
}

// NewNodeCollector creates a new NodeCollector wrapping the given saver
func NewNodeCollector(saver NodeSaver) *NodeCollector {
	return &NodeCollector{
		saver:    saver,
		nodeCIDs: make([]string, 0),
	}
}

// SaveNode saves a node and records its CID
func (nc *NodeCollector) SaveNode(ctx context.Context, cid string, data []byte) error {
	if err := nc.saver.SaveNode(ctx, cid, data); err != nil {
		return err
	}
	nc.mu.Lock()
	nc.nodeCIDs = append(nc.nodeCIDs, cid)
	nc.mu.Unlock()
	return nil
}

// NodeCIDs returns all collected node CIDs
func (nc *NodeCollector) NodeCIDs() []string {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	result := make([]string, len(nc.nodeCIDs))
	copy(result, nc.nodeCIDs)
	return result
}

// NodeSaver interface for saving dag-pb nodes
type NodeSaver interface {
	SaveNode(ctx context.Context, cid string, data []byte) error
}

// dirNode represents a directory in the tree structure during DAG building
type dirNode struct {
	children map[string]*dirNode
	files    map[string]*backup.Entry
}

// BuildManifestDAG builds the UnixFS DAG structure for a manifest.
// It creates file nodes for multi-block files and directory nodes,
// saving them via the NodeSaver and updating the manifest with CIDs.
// Returns the root CID.
func BuildManifestDAG(ctx context.Context, manifest *backup.Manifest, saver NodeSaver) (cid.Cid, error) {
	// Step 1: Process all file entries - create file nodes for multi-block files
	for i := range manifest.Entries {
		entry := &manifest.Entries[i]
		if entry.Type != backup.FileTypeFile {
			continue
		}

		if len(entry.Blocks) == 0 {
			continue
		}

		if len(entry.Blocks) == 1 {
			// Single block file - the block CID IS the file CID
			entry.CID = entry.Blocks[0]
		} else {
			// Multi-block file - create a file node
			blockSizes := make([]uint64, len(entry.Blocks))
			for j := range blockSizes {
				// We store 8MB chunks, but the last one might be smaller
				// For simplicity, use ChunkSize for all but estimate from total size
				if j < len(entry.Blocks)-1 {
					blockSizes[j] = uint64(backup.ChunkSize)
				} else {
					// Last block
					remaining := uint64(entry.Size) - uint64(j)*uint64(backup.ChunkSize)
					blockSizes[j] = remaining
				}
			}

			fileNode, err := BuildFileNode(entry.Blocks, blockSizes, uint64(entry.Size))
			if err != nil {
				return cid.Undef, err
			}

			if fileNode != nil {
				// Save the file node
				if err := saver.SaveNode(ctx, fileNode.Cid.String(), fileNode.Data); err != nil {
					return cid.Undef, err
				}
				entry.CID = fileNode.Cid.String()
			}
		}
	}

	// Step 2: Build directory tree from entries
	rootCID, err := buildDirectoryTree(ctx, manifest.Entries, saver)
	if err != nil {
		return cid.Undef, err
	}

	// Update manifest with root CID
	manifest.RootCID = rootCID.String()

	return rootCID, nil
}

// buildDirectoryTree builds the directory node hierarchy from entries
func buildDirectoryTree(ctx context.Context, entries []backup.Entry, saver NodeSaver) (cid.Cid, error) {
	root := &dirNode{
		children: make(map[string]*dirNode),
		files:    make(map[string]*backup.Entry),
	}

	// Populate tree
	for i := range entries {
		entry := &entries[i]
		parts := strings.Split(entry.Path, "/")

		if len(parts) == 0 {
			continue
		}

		current := root
		for j := 0; j < len(parts)-1; j++ {
			part := parts[j]
			if part == "" {
				continue
			}
			if current.children[part] == nil {
				current.children[part] = &dirNode{
					children: make(map[string]*dirNode),
					files:    make(map[string]*backup.Entry),
				}
			}
			current = current.children[part]
		}

		name := parts[len(parts)-1]
		if name == "" {
			continue
		}

		if entry.Type == backup.FileTypeDir {
			if current.children[name] == nil {
				current.children[name] = &dirNode{
					children: make(map[string]*dirNode),
					files:    make(map[string]*backup.Entry),
				}
			}
		} else {
			current.files[name] = entry
		}
	}

	// Recursively build directory nodes bottom-up
	return buildDirNodeRecursive(ctx, root, saver)
}

func buildDirNodeRecursive(ctx context.Context, dir *dirNode, saver NodeSaver) (cid.Cid, error) {
	var dirEntries []DirEntry

	// Process child directories first
	childNames := make([]string, 0, len(dir.children))
	for name := range dir.children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)

	for _, name := range childNames {
		child := dir.children[name]
		childCID, err := buildDirNodeRecursive(ctx, child, saver)
		if err != nil {
			return cid.Undef, err
		}

		// Calculate directory size (sum of all contents)
		dirSize := calculateDirSize(child)

		dirEntries = append(dirEntries, DirEntry{
			Name: name,
			Cid:  childCID,
			Size: dirSize,
		})
	}

	// Process files
	fileNames := make([]string, 0, len(dir.files))
	for name := range dir.files {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)

	for _, name := range fileNames {
		entry := dir.files[name]
		if entry.CID == "" {
			continue // Skip entries without CID (shouldn't happen)
		}

		entryCID, err := cid.Decode(entry.CID)
		if err != nil {
			continue
		}

		dirEntries = append(dirEntries, DirEntry{
			Name: name,
			Cid:  entryCID,
			Size: uint64(entry.Size),
		})
	}

	// Build directory node
	node, err := BuildDirNode(dirEntries)
	if err != nil {
		return cid.Undef, err
	}

	// Save the node
	if err := saver.SaveNode(ctx, node.Cid.String(), node.Data); err != nil {
		return cid.Undef, err
	}

	return node.Cid, nil
}

func calculateDirSize(dir *dirNode) uint64 {
	var size uint64
	for _, child := range dir.children {
		size += calculateDirSize(child)
	}
	for _, file := range dir.files {
		size += uint64(file.Size)
	}
	return size
}

// GetEntryCID returns the CID for an entry (either its own CID or the single block CID)
func GetEntryCID(entry *backup.Entry) string {
	if entry.CID != "" {
		return entry.CID
	}
	if len(entry.Blocks) == 1 {
		return entry.Blocks[0]
	}
	return ""
}

// GetEntryPath returns the full IPFS path for an entry given the manifest root CID
func GetEntryPath(rootCID string, entryPath string) string {
	return "/ipfs/" + rootCID + "/" + path.Clean(entryPath)
}
