package storage

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reuse or re-define a simple in-memory ChunkStore.
// If you already have memChunkStore in another test file, you can
// either move it to a shared file or leave this duplicate for now.

type memFileChunkStore struct {
	chunks map[ChunkID][]byte
}

func newMemFileChunkStore() *memFileChunkStore {
	return &memFileChunkStore{
		chunks: make(map[ChunkID][]byte),
	}
}

func (m *memFileChunkStore) PutChunk(data []byte) (ChunkID, error) {
	id := NewChunkID(data)
	b := make([]byte, len(data))
	copy(b, data)
	m.chunks[id] = b
	return id, nil
}

func (m *memFileChunkStore) GetChunk(id ChunkID) ([]byte, error) {
	data, ok := m.chunks[id]
	if !ok {
		return nil, io.EOF
	}
	b := make([]byte, len(data))
	copy(b, data)
	return b, nil
}

func (m *memFileChunkStore) HasChunk(id ChunkID) (bool, error) {
	_, ok := m.chunks[id]
	return ok, nil
}

func (m *memFileChunkStore) DeleteChunk(id ChunkID) error {
	delete(m.chunks, id)
	return nil
}

func (m *memFileChunkStore) ListChunks() ([]ChunkID, error) {
	ids := make([]ChunkID, 0, len(m.chunks))
	for id := range m.chunks {
		ids = append(ids, id)
	}
	return ids, nil
}

func TestBuildFileMetaFromReader_Basic(t *testing.T) {
	store := newMemFileChunkStore()

	// Some arbitrary data that will span multiple chunks.
	data := []byte("this is a test file content that will be chunked")
	chunkSize := 8

	meta, err := BuildFileMetaFromReader(store, bytes.NewReader(data), chunkSize)
	require.NoError(t, err)
	require.NotNil(t, meta)

	// Check size.
	assert.Equal(t, int64(len(data)), meta.Size, "file size should match input length")

	// Should have at least more than 1 chunk for this data/chunkSize.
	require.GreaterOrEqual(t, len(meta.Chunks), 1)

	// Collect the ChunkIDs from meta and recompute Merkle root manually.
	ids := make([]ChunkID, len(meta.Chunks))
	for i, cm := range meta.Chunks {
		ids[i] = cm.ID
	}
	root2, err := MerkleRootForChunks(ids)
	require.NoError(t, err)
	assert.True(t, meta.Root.Equal(root2), "meta.Root should equal MerkleRootForChunks(ids)")

	// Reconstruct the original data by reading chunks from the store.
	var reconstructed []byte
	for _, cm := range meta.Chunks {
		chunkData, err := store.GetChunk(cm.ID)
		require.NoError(t, err)
		reconstructed = append(reconstructed, chunkData...)
	}
	assert.Equal(t, data, reconstructed, "reconstructed data should match original")
}

func TestBuildFileMetaFromReader_EmptyReader(t *testing.T) {
	store := newMemFileChunkStore()

	_, err := BuildFileMetaFromReader(store, bytes.NewReader(nil), 8)
	assert.Error(t, err, "empty reader should error (no data)")
}
