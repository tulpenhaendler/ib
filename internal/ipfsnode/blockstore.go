package ipfsnode

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/johann/ib/internal/backup"
)

// StorageBackend interface for the blockstore to access storage
type StorageBackend interface {
	GetBlock(ctx context.Context, cid string) ([]byte, error)
	GetNode(ctx context.Context, cid string) ([]byte, error)
	BlockExists(ctx context.Context, cid string) (bool, error)
	NodeExists(ctx context.Context, cid string) (bool, error)
}

// Blockstore implements the IPFS blockstore interface backed by our storage
type Blockstore struct {
	storage StorageBackend
}

// NewBlockstore creates a new blockstore
func NewBlockstore(storage StorageBackend) *Blockstore {
	return &Blockstore{storage: storage}
}

// Has returns whether the block is in the blockstore
func (bs *Blockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	cidStr := c.String()

	// Check if it's a dag-pb node first
	if c.Type() == cid.DagProtobuf {
		exists, err := bs.storage.NodeExists(ctx, cidStr)
		if err == nil && exists {
			return true, nil
		}
	}

	// Check raw blocks
	return bs.storage.BlockExists(ctx, cidStr)
}

// Get retrieves a block from the blockstore
func (bs *Blockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	cidStr := c.String()

	// Check if it's a dag-pb node first
	if c.Type() == cid.DagProtobuf {
		data, err := bs.storage.GetNode(ctx, cidStr)
		if err == nil {
			return blocks.NewBlockWithCid(data, c)
		}
	}

	// Get raw data block and decompress
	compressedData, err := bs.storage.GetBlock(ctx, cidStr)
	if err != nil {
		return nil, err
	}

	// Decompress the block
	data, err := backup.Decompress(compressedData, backup.ChunkSize)
	if err != nil {
		// Might not be compressed
		data = compressedData
	}

	return blocks.NewBlockWithCid(data, c)
}

// GetSize returns the size of a block
func (bs *Blockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	block, err := bs.Get(ctx, c)
	if err != nil {
		return 0, err
	}
	return len(block.RawData()), nil
}

// Put is not supported - blocks are added via the backup process
func (bs *Blockstore) Put(ctx context.Context, block blocks.Block) error {
	return errors.New("blockstore is read-only")
}

// PutMany is not supported
func (bs *Blockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	return errors.New("blockstore is read-only")
}

// DeleteBlock is not supported
func (bs *Blockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return errors.New("blockstore is read-only")
}

// AllKeysChan returns a channel of all CIDs in the blockstore
func (bs *Blockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	// Not implemented for now - would need to iterate over blocks and nodes tables
	ch := make(chan cid.Cid)
	close(ch)
	return ch, nil
}

// HashOnRead is not used
func (bs *Blockstore) HashOnRead(enabled bool) {}
