package backup

import (
	"io"
	"os"

	"github.com/johann/ib/internal/cid"
	"github.com/pierrec/lz4/v4"
)

const (
	// ChunkSize is the maximum size of a chunk (8MB)
	ChunkSize = 8 * 1024 * 1024
)

// ChunkResult represents a processed chunk
type ChunkResult struct {
	CID          string
	Data         []byte // Compressed data
	OriginalSize int64
	Error        error
}

// Chunker splits files into content-addressed chunks
type Chunker struct{}

// NewChunker creates a new chunker
func NewChunker() *Chunker {
	return &Chunker{}
}

// ChunkFile splits a file into chunks and returns them via channel
func (c *Chunker) ChunkFile(path string) <-chan ChunkResult {
	results := make(chan ChunkResult, 4)

	go func() {
		defer close(results)

		file, err := os.Open(path)
		if err != nil {
			results <- ChunkResult{Error: err}
			return
		}
		defer file.Close()

		buffer := make([]byte, ChunkSize)

		for {
			n, err := io.ReadFull(file, buffer)
			if err == io.EOF {
				break
			}
			if err != nil && err != io.ErrUnexpectedEOF {
				results <- ChunkResult{Error: err}
				return
			}

			chunk := buffer[:n]

			// Generate CID from original data
			chunkCID, err := cid.Generate(chunk)
			if err != nil {
				results <- ChunkResult{Error: err}
				return
			}

			// Compress the chunk
			compressed := make([]byte, lz4.CompressBlockBound(n))
			compressedSize, err := lz4.CompressBlock(chunk, compressed, nil)
			if err != nil {
				results <- ChunkResult{Error: err}
				return
			}

			// If compression didn't help, store uncompressed
			var data []byte
			if compressedSize > 0 && compressedSize < n {
				data = compressed[:compressedSize]
			} else {
				data = chunk
			}

			results <- ChunkResult{
				CID:          chunkCID,
				Data:         data,
				OriginalSize: int64(n),
			}

			if err == io.ErrUnexpectedEOF {
				break
			}
		}
	}()

	return results
}

// ChunkData splits data into chunks (for small files or in-memory data)
func (c *Chunker) ChunkData(data []byte) ([]ChunkResult, error) {
	var results []ChunkResult

	for offset := 0; offset < len(data); offset += ChunkSize {
		end := offset + ChunkSize
		if end > len(data) {
			end = len(data)
		}

		chunk := data[offset:end]

		chunkCID, err := cid.Generate(chunk)
		if err != nil {
			return nil, err
		}

		compressed := make([]byte, lz4.CompressBlockBound(len(chunk)))
		compressedSize, err := lz4.CompressBlock(chunk, compressed, nil)
		if err != nil {
			return nil, err
		}

		var resultData []byte
		if compressedSize > 0 && compressedSize < len(chunk) {
			resultData = compressed[:compressedSize]
		} else {
			resultData = chunk
		}

		results = append(results, ChunkResult{
			CID:          chunkCID,
			Data:         resultData,
			OriginalSize: int64(len(chunk)),
		})
	}

	return results, nil
}

// Decompress decompresses LZ4 compressed data
func Decompress(compressed []byte, originalSize int64) ([]byte, error) {
	decompressed := make([]byte, originalSize)
	n, err := lz4.UncompressBlock(compressed, decompressed)
	if err != nil {
		// Data might not be compressed, return as-is
		if int64(len(compressed)) == originalSize {
			return compressed, nil
		}
		return nil, err
	}
	return decompressed[:n], nil
}

// CompressBlock compresses data using LZ4
func CompressBlock(src, dst []byte) (int, error) {
	return lz4.CompressBlock(src, dst, nil)
}
