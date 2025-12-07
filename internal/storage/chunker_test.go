package storage

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simple in-memory ChunkStore for testing.
type memChunkStore struct {
	chunks map[ChunkID][]byte
}

func newMemChunkStore() *memChunkStore {
	return &memChunkStore{
		chunks: make(map[ChunkID][]byte),
	}
}

func (m *memChunkStore) PutChunk(data []byte) (ChunkID, error) {
	id := NewChunkID(data)
	b := make([]byte, len(data))
	copy(b, data)
	m.chunks[id] = b
	return id, nil
}

func (m *memChunkStore) GetChunk(id ChunkID) ([]byte, error) {
	data, ok := m.chunks[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	b := make([]byte, len(data))
	copy(b, data)
	return b, nil
}

func (m *memChunkStore) HasChunk(id ChunkID) (bool, error) {
	_, ok := m.chunks[id]
	return ok, nil
}

func (m *memChunkStore) DeleteChunk(id ChunkID) error {
	delete(m.chunks, id)
	return nil
}

func (m *memChunkStore) ListChunks() ([]ChunkID, error) {
	ids := make([]ChunkID, 0, len(m.chunks))
	for id := range m.chunks {
		ids = append(ids, id)
	}
	return ids, nil
}

func TestStoreFromReader_SplitsIntoChunks(t *testing.T) {
	store := newMemChunkStore()

	// Build some data: e.g., 2.5 chunks of size 4.
	chunkSize := 4
	data := []byte("abcdefghij") // len=10 -> 4 + 4 + 2

	r := bytes.NewReader(data)

	chunks, err := StoreFromReader(store, r, chunkSize)
	require.NoError(t, err)
	require.Len(t, chunks, 3, "expected 3 chunks (4,4,2 bytes)")

	// Check sizes.
	assert.Equal(t, int64(4), chunks[0].Size)
	assert.Equal(t, int64(4), chunks[1].Size)
	assert.Equal(t, int64(2), chunks[2].Size)

	// Reassemble from store and verify content.
	var reconstructed []byte
	for _, cm := range chunks {
		part, err := store.GetChunk(cm.ID)
		require.NoError(t, err)
		reconstructed = append(reconstructed, part...)
	}

	assert.Equal(t, data, reconstructed, "reconstructed data should match original")
}

func TestStoreFromReader_EmptyInput(t *testing.T) {
	store := newMemChunkStore()

	r := bytes.NewReader(nil)
	chunks, err := StoreFromReader(store, r, 4)
	require.NoError(t, err)
	assert.Len(t, chunks, 0, "no chunks should be produced for empty input")
}

func TestStoreFromReader_InvalidArgs(t *testing.T) {
	_, err := StoreFromReader(nil, bytes.NewReader([]byte("data")), 4)
	assert.Error(t, err, "nil store should error")

	store := newMemChunkStore()
	_, err = StoreFromReader(store, bytes.NewReader([]byte("data")), 0)
	assert.Error(t, err, "chunkSize <= 0 should error")
}
