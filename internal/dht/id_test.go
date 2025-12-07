package dht

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDFromBytesAndString(t *testing.T) {
	raw := make([]byte, IDBytes)
	for i := 0; i < IDBytes; i++ {
		raw[i] = byte(i)
	}

	id, err := IDFromBytes(raw)
	require.NoError(t, err)

	// String() should give us a hex representation of raw.
	expectedHex := hex.EncodeToString(raw)
	assert.Equal(t, expectedHex, id.String(), "hex string mismatch")

	// Converting back from hex should give the same ID.
	id2, err := IDFromHex(id.String())
	require.NoError(t, err)
	assert.True(t, id.Equals(id2), "IDs should be equal after hex round-trip")
}

func TestIDFromBytesInvalidLength(t *testing.T) {
	_, err := IDFromBytes([]byte{1, 2, 3})
	require.Error(t, err, "expected error for invalid length")
}

func TestXORDistance(t *testing.T) {
	// Simple case:
	// a = 00001111
	// b = 11110000
	// a XOR b = 11111111
	var a, b ID
	a[IDBytes-1] = 0x0F // last byte = 00001111
	b[IDBytes-1] = 0xF0 // last byte = 11110000

	dist := a.XOR(b)
	expectedLast := byte(0xFF)

	assert.Equal(t, expectedLast, dist[IDBytes-1], "XOR of last byte mismatch")

	// XOR with itself should give zero ID.
	zero := a.XOR(a)
	for i := 0; i < IDBytes; i++ {
		assert.Equal(t, byte(0x00), zero[i], "XOR with self should yield zero")
	}
}

func TestPrefixLen(t *testing.T) {
	var id ID

	// All zeros => PrefixLen should be IDBits.
	assert.Equal(t, IDBits, id.PrefixLen(), "all-zero ID should have full prefix length")

	// First bit 1, rest 0 => PrefixLen = 0
	id = ID{}
	id[0] = 0x80 // 1000 0000
	assert.Equal(t, 0, id.PrefixLen(), "first bit 1 should give prefix len 0")

	// 0001 0000 => 3 leading zeros.
	id = ID{}
	id[0] = 0x10 // 0001 0000
	assert.Equal(t, 3, id.PrefixLen(), "0x10 should have 3 leading zeros in the first byte")

	// First byte zero, second byte 0000 1000 => 8 + 3 = 11 leading zeros.
	id = ID{}
	id[0] = 0x00
	id[1] = 0x08 // 0000 1000
	assert.Equal(t, 12, id.PrefixLen(), "0x0008 should have 12 leading zeros")
}
