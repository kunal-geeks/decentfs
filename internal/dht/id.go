package dht

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// IDBits is the length of a Kademlia ID in bits.
// We'll use 160 bits (20 bytes), similar to SHA-1.
const IDBits = 160

// IDBytes is the length of an ID in bytes.
const IDBytes = IDBits / 8

// ID represents a Kademlia node or key identifier.
// It's a fixed-size 160-bit value.
type ID [IDBytes]byte

// NewRandomID generates a cryptographically random ID.
func NewRandomID() (ID, error) {
	var id ID
	_, err := rand.Read(id[:])
	if err != nil {
		return ID{}, fmt.Errorf("NewRandomID: %w", err)
	}
	return id, nil
}

// MustRandomID is a helper for tests or places where you want to panic on error.
// We'll mostly use this in tests.
func MustRandomID() ID {
	id, err := NewRandomID()
	if err != nil {
		panic(err)
	}
	return id
}

// IDFromBytes constructs an ID from a byte slice.
// Returns an error if the slice length is not IDBytes.
func IDFromBytes(b []byte) (ID, error) {
	if len(b) != IDBytes {
		return ID{}, fmt.Errorf("IDFromBytes: invalid length %d, want %d", len(b), IDBytes)
	}
	var id ID
	copy(id[:], b)
	return id, nil
}

// IDFromHex constructs an ID from a hex string.
// The hex string must decode to IDBytes bytes.
func IDFromHex(s string) (ID, error) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		return ID{}, fmt.Errorf("IDFromHex: decode error: %w", err)
	}
	return IDFromBytes(raw)
}

// String returns the hex encoding of the ID (for logging/debugging).
func (id ID) String() string {
	return hex.EncodeToString(id[:])
}

// XOR computes the bitwise XOR distance between two IDs.
// In Kademlia, "distance" is defined as XOR of the two node IDs.
func (id ID) XOR(other ID) ID {
	var out ID
	for i := 0; i < IDBytes; i++ {
		out[i] = id[i] ^ other[i]
	}
	return out
}

// Equals reports whether two IDs are identical.
func (id ID) Equals(other ID) bool {
	for i := 0; i < IDBytes; i++ {
		if id[i] != other[i] {
			return false
		}
	}
	return true
}

// Less reports whether id is lexicographically less than other.
// It compares byte-by-byte from most-significant to least-significant
// which is suitable for deterministic ordering of IDs (used by sorting).
func (id ID) Less(other ID) bool {
	for i := 0; i < IDBytes; i++ {
		if id[i] < other[i] {
			return true
		}
		if id[i] > other[i] {
			return false
		}
	}
	return false // equal IDs => not less
}

// PrefixLen returns the number of leading zero bits in the ID.
// This is useful in Kademlia to decide which bucket an ID belongs to.
//
// Example:
//
//	ID: 00010010.... (in bits)
//	PrefixLen = 3    (first 3 bits are zero, 4th is 1)
func (id ID) PrefixLen() int {
	count := 0
	for i := 0; i < IDBytes; i++ {
		b := id[i]
		// For each full zero byte, we add 8 and continue.
		if b == 0x00 {
			count += 8
			continue
		}
		// For the first non-zero byte, count leading zeros.
		for j := 7; j >= 0; j-- {
			if (b & (1 << uint(j))) == 0 {
				count++
			} else {
				// First 1 bit encountered, stop here.
				return count
			}
		}
	}
	return count
}
