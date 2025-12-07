package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChunkID_Deterministic(t *testing.T) {
	data := []byte("hello world")

	id1 := NewChunkID(data)
	id2 := NewChunkID(data)

	assert.True(t, id1.Equal(id2), "same data should produce same ChunkID")
	assert.Equal(t, id1.String(), id2.String(), "string representations should match")
}

func TestNewChunkID_DifferentContentDifferentID(t *testing.T) {
	data1 := []byte("chunk one")
	data2 := []byte("chunk two")

	id1 := NewChunkID(data1)
	id2 := NewChunkID(data2)

	assert.False(t, id1.Equal(id2), "different data should produce different ChunkIDs (with very high probability)")
}

func TestChunkID_StringAndParse(t *testing.T) {
	data := []byte("some content")
	id := NewChunkID(data)

	hexStr := id.String()
	parsed, err := ChunkIDFromHex(hexStr)
	require.NoError(t, err, "parsing valid ChunkID hex string should not error")

	assert.True(t, id.Equal(parsed), "parsed ID should equal original")
	assert.Equal(t, hexStr, parsed.String(), "String() after parse should round-trip")
}

func TestChunkIDFromHex_Invalid(t *testing.T) {
	// Not hex
	_, err := ChunkIDFromHex("not-hex")
	assert.Error(t, err, "invalid hex should error")

	// Wrong length (not 32 bytes == 64 hex chars)
	_, err = ChunkIDFromHex("abcd")
	assert.Error(t, err, "too short hex should error")
}

func TestChunkID_BytesReturnsCopy(t *testing.T) {
	data := []byte("immutable test")
	id := NewChunkID(data)

	b := id.Bytes()
	require.Len(t, b, 32)

	// Mutate returned slice; should not affect original ID.
	b[0] ^= 0xFF

	// If we re-hash the same data, it must equal the original ID,
	// meaning the internal array wasn't mutated.
	id2 := NewChunkID(data)
	assert.True(t, id.Equal(id2), "mutating Bytes() output should not change the ChunkID")
}
