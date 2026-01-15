package cid

import (
	"crypto/sha256"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

// Generate creates an IPFS CIDv1 from data using SHA-256
func Generate(data []byte) (string, error) {
	hash := sha256.Sum256(data)

	mh, err := multihash.Encode(hash[:], multihash.SHA2_256)
	if err != nil {
		return "", err
	}

	// CIDv1 with raw codec (0x55)
	c := cid.NewCidV1(cid.Raw, mh)
	return c.String(), nil
}

// Validate checks if a string is a valid CID
func Validate(s string) bool {
	_, err := cid.Decode(s)
	return err == nil
}
