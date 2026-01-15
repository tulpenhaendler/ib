package ipfsnode

import (
	"crypto/sha256"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

// DAGNode represents a UnixFS node (file or directory)
type DAGNode struct {
	Cid  cid.Cid
	Data []byte // The raw dag-pb encoded data
}

// DirEntry represents an entry in a directory
type DirEntry struct {
	Name string
	Cid  cid.Cid
	Size uint64
}

// UnixFS protobuf type constants
const (
	unixfsTypeFile      = 2
	unixfsTypeDirectory = 1
)

// BuildFileNode creates a UnixFS file node for a multi-block file.
// For single-block files, just use the raw block CID directly (returns nil).
func BuildFileNode(blockCIDs []string, blockSizes []uint64, totalSize uint64) (*DAGNode, error) {
	if len(blockCIDs) == 0 {
		return nil, nil
	}

	// Single block file - no wrapper needed
	if len(blockCIDs) == 1 {
		return nil, nil
	}

	// Build UnixFS Data field manually (protobuf)
	// Type = 2 (File), filesize = totalSize, blocksizes = blockSizes
	unixfsData := encodeUnixFSFile(totalSize, blockSizes)

	// Build dag-pb node with links
	links := make([]pbLink, len(blockCIDs))
	for i, cidStr := range blockCIDs {
		c, err := cid.Decode(cidStr)
		if err != nil {
			return nil, err
		}
		links[i] = pbLink{
			Hash:  c.Bytes(),
			Tsize: blockSizes[i],
		}
	}

	pbData := encodePBNode(links, unixfsData)
	nodeCid := computeDagPBCid(pbData)

	return &DAGNode{
		Cid:  nodeCid,
		Data: pbData,
	}, nil
}

// BuildDirNode creates a UnixFS directory node
func BuildDirNode(entries []DirEntry) (*DAGNode, error) {
	// Build UnixFS Data field for directory
	unixfsData := encodeUnixFSDirectory()

	// Build dag-pb node with links
	links := make([]pbLink, len(entries))
	for i, entry := range entries {
		links[i] = pbLink{
			Hash:  entry.Cid.Bytes(),
			Name:  entry.Name,
			Tsize: entry.Size,
		}
	}

	pbData := encodePBNode(links, unixfsData)
	nodeCid := computeDagPBCid(pbData)

	return &DAGNode{
		Cid:  nodeCid,
		Data: pbData,
	}, nil
}

// pbLink represents a link in a dag-pb node
type pbLink struct {
	Hash  []byte
	Name  string
	Tsize uint64
}

// encodeUnixFSFile encodes UnixFS file data
func encodeUnixFSFile(filesize uint64, blocksizes []uint64) []byte {
	// UnixFS protobuf:
	// message Data {
	//   required DataType Type = 1;
	//   optional bytes Data = 2;
	//   optional uint64 filesize = 3;
	//   repeated uint64 blocksizes = 4;
	// }
	var buf []byte

	// Type = 2 (File), field 1, varint
	buf = append(buf, 0x08) // field 1, wire type 0 (varint)
	buf = appendVarint(buf, unixfsTypeFile)

	// filesize, field 3, varint
	buf = append(buf, 0x18) // field 3, wire type 0 (varint)
	buf = appendVarint(buf, filesize)

	// blocksizes, field 4, repeated varint
	for _, size := range blocksizes {
		buf = append(buf, 0x20) // field 4, wire type 0 (varint)
		buf = appendVarint(buf, size)
	}

	return buf
}

// encodeUnixFSDirectory encodes UnixFS directory data
func encodeUnixFSDirectory() []byte {
	var buf []byte
	buf = append(buf, 0x08) // field 1, wire type 0 (varint)
	buf = appendVarint(buf, unixfsTypeDirectory)
	return buf
}

// encodePBNode encodes a dag-pb node
func encodePBNode(links []pbLink, data []byte) []byte {
	// PBNode protobuf:
	// message PBNode {
	//   repeated PBLink Links = 2;
	//   optional bytes Data = 1;
	// }
	// PBLink:
	// message PBLink {
	//   optional bytes Hash = 1;
	//   optional string Name = 2;
	//   optional uint64 Tsize = 3;
	// }
	var buf []byte

	// Links come first (field 2, repeated)
	for _, link := range links {
		linkData := encodePBLink(link)
		buf = append(buf, 0x12) // field 2, wire type 2 (length-delimited)
		buf = appendVarint(buf, uint64(len(linkData)))
		buf = append(buf, linkData...)
	}

	// Data field (field 1)
	if len(data) > 0 {
		buf = append(buf, 0x0a) // field 1, wire type 2 (length-delimited)
		buf = appendVarint(buf, uint64(len(data)))
		buf = append(buf, data...)
	}

	return buf
}

func encodePBLink(link pbLink) []byte {
	var buf []byte

	// Hash (field 1)
	if len(link.Hash) > 0 {
		buf = append(buf, 0x0a) // field 1, wire type 2
		buf = appendVarint(buf, uint64(len(link.Hash)))
		buf = append(buf, link.Hash...)
	}

	// Name (field 2)
	if link.Name != "" {
		buf = append(buf, 0x12) // field 2, wire type 2
		buf = appendVarint(buf, uint64(len(link.Name)))
		buf = append(buf, link.Name...)
	}

	// Tsize (field 3)
	if link.Tsize > 0 {
		buf = append(buf, 0x18) // field 3, wire type 0
		buf = appendVarint(buf, link.Tsize)
	}

	return buf
}

func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

func computeDagPBCid(data []byte) cid.Cid {
	hash := sha256.Sum256(data)
	multihash, _ := mh.Encode(hash[:], mh.SHA2_256)
	return cid.NewCidV1(cid.DagProtobuf, multihash)
}
