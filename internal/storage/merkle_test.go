package storage

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerkleRootForChunks_Empty(t *testing.T) {
	_, err := MerkleRootForChunks(nil)
	assert.Error(t, err, "empty slice should error")

	_, err = MerkleRootForChunks([]ChunkID{})
	assert.Error(t, err, "empty slice should error")
}

func TestMerkleRootForChunks_SingleLeaf(t *testing.T) {
	data := []byte("only chunk")
	id := NewChunkID(data)

	root, err := MerkleRootForChunks([]ChunkID{id})
	require.NoError(t, err)
	assert.True(t, root.Equal(id), "single leaf root should equal the leaf ID")
}

func TestMerkleRootForChunks_TwoLeaves(t *testing.T) {
	data1 := []byte("chunk-1")
	data2 := []byte("chunk-2")

	id1 := NewChunkID(data1)
	id2 := NewChunkID(data2)

	// Expected root = SHA256(id1 || id2)
	buf := append(id1.Bytes(), id2.Bytes()...)
	sum := sha256.Sum256(buf)
	expected := ChunkID(sum)

	root, err := MerkleRootForChunks([]ChunkID{id1, id2})
	require.NoError(t, err)
	assert.True(t, root.Equal(expected), "root for two leaves should match manual hash")
}

func TestMerkleRootForChunks_ThreeLeaves(t *testing.T) {
	// We'll build a small tree manually:
	// leaves: a, b, c
	// level1: h_ab = H(a||b), c promoted
	// root: H(h_ab || c)
	a := NewChunkID([]byte("A"))
	b := NewChunkID([]byte("B"))
	c := NewChunkID([]byte("C"))

	// h_ab
	buf := append(a.Bytes(), b.Bytes()...)
	hAb := sha256.Sum256(buf)

	// rootExpected = H(h_ab || c)
	buf2 := make([]byte, 0, 64)
	buf2 = append(buf2, hAb[:]...)
	buf2 = append(buf2, c.Bytes()...)
	rootExpected := ChunkID(sha256.Sum256(buf2))

	root, err := MerkleRootForChunks([]ChunkID{a, b, c})
	require.NoError(t, err)
	assert.True(t, root.Equal(rootExpected), "root should match manual 3-leaf computation")
}

func TestMerkleRootForChunks_OrderMatters(t *testing.T) {
	data1 := []byte("chunk-X")
	data2 := []byte("chunk-Y")

	id1 := NewChunkID(data1)
	id2 := NewChunkID(data2)

	root1, err := MerkleRootForChunks([]ChunkID{id1, id2})
	require.NoError(t, err)

	root2, err := MerkleRootForChunks([]ChunkID{id2, id1})
	require.NoError(t, err)

	assert.False(t, root1.Equal(root2), "Merkle root should depend on chunk order")
}

func TestMerkleRootForChunks_Deterministic(t *testing.T) {
	data := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
		[]byte("d"),
	}

	var ids []ChunkID
	for _, d := range data {
		ids = append(ids, NewChunkID(d))
	}

	root1, err := MerkleRootForChunks(ids)
	require.NoError(t, err)

	// Compute again with a fresh slice (same order).
	ids2 := make([]ChunkID, len(ids))
	copy(ids2, ids)

	root2, err := MerkleRootForChunks(ids2)
	require.NoError(t, err)

	assert.True(t, root1.Equal(root2), "Merkle root should be deterministic")

	// Make sure roots are not all zeros.
	assert.False(t, bytes.Equal(root1.Bytes(), make([]byte, 32)))
}
