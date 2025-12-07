package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ChunkID is a 256-bit content identifier for a chunk (SHA-256).
// It is derived purely from the chunk's content, so if two chunks
// have identical bytes, they have the same ChunkID.
type ChunkID [32]byte

// NewChunkID computes the SHA-256 hash of the given data and
// returns it as a ChunkID.
func NewChunkID(data []byte) ChunkID {
	sum := sha256.Sum256(data)
	return ChunkID(sum)
}

// Bytes returns the raw 32-byte slice of the ChunkID.
func (id ChunkID) Bytes() []byte {
	// Return a copy to avoid caller mutating internal array.
	b := make([]byte, len(id))
	copy(b, id[:])
	return b
}

// String returns the ChunkID as a lowercase hex string,
// e.g. "9f2c7a...".
func (id ChunkID) String() string {
	return hex.EncodeToString(id[:])
}

// ChunkIDFromHex parses a 64-char hex string into a ChunkID.
// Returns an error if the string is invalid.
func ChunkIDFromHex(s string) (ChunkID, error) {
	var id ChunkID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, fmt.Errorf("ChunkIDFromHex: decode error: %w", err)
	}
	if len(b) != len(id) {
		return id, fmt.Errorf("ChunkIDFromHex: invalid length: got %d, want %d", len(b), len(id))
	}
	copy(id[:], b)
	return id, nil
}

// MustChunkIDFromHex is a convenience for tests / hard-coded IDs.
// It panics if parsing fails.
func MustChunkIDFromHex(s string) ChunkID {
	id, err := ChunkIDFromHex(s)
	if err != nil {
		panic(err)
	}
	return id
}

// Equal reports whether two ChunkIDs are identical.
func (id ChunkID) Equal(other ChunkID) bool {
	return id == other
}
