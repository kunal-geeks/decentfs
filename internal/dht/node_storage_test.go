package dht

import (
	"io/fs"
	"testing"

	"github.com/kunal-geeks/decentfs/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockChunkStore is an in-memory implementation of storage.ChunkStore
// for testing Node's integration with the store.
type mockChunkStore struct {
	chunks map[storage.ChunkID][]byte
}

func newMockChunkStore() *mockChunkStore {
	return &mockChunkStore{
		chunks: make(map[storage.ChunkID][]byte),
	}
}

func (m *mockChunkStore) PutChunk(data []byte) (storage.ChunkID, error) {
	id := storage.NewChunkID(data)
	// store a copy
	b := make([]byte, len(data))
	copy(b, data)
	m.chunks[id] = b
	return id, nil
}

func (m *mockChunkStore) GetChunk(id storage.ChunkID) ([]byte, error) {
	data, ok := m.chunks[id]
	if !ok {
		return nil, fs.ErrNotExist
	}
	b := make([]byte, len(data))
	copy(b, data)
	return b, nil
}

func (m *mockChunkStore) HasChunk(id storage.ChunkID) (bool, error) {
	_, ok := m.chunks[id]
	return ok, nil
}

func (m *mockChunkStore) DeleteChunk(id storage.ChunkID) error {
	delete(m.chunks, id)
	return nil
}

func (m *mockChunkStore) ListChunks() ([]storage.ChunkID, error) {
	ids := make([]storage.ChunkID, 0, len(m.chunks))
	for id := range m.chunks {
		ids = append(ids, id)
	}
	return ids, nil
}

func TestNode_StoreAndGetLocalChunk(t *testing.T) {
	self := MustRandomID()
	node := NewNode(self)

	store := newMockChunkStore()
	node.store = store

	data := []byte("hello chunk storage")

	id, err := node.StoreLocalChunk(data)
	require.NoError(t, err)

	// Check that store has it.
	has, err := store.HasChunk(id)
	require.NoError(t, err)
	assert.True(t, has, "store should have chunk after StoreLocalChunk")

	// Get it back.
	got, err := node.GetLocalChunk(id)
	require.NoError(t, err)
	assert.Equal(t, data, got, "GetLocalChunk should return original data")
}

func TestNode_StoreLocalChunk_NoStoreConfigured(t *testing.T) {
	self := MustRandomID()
	node := NewNode(self)

	_, err := node.StoreLocalChunk([]byte("data"))
	assert.Error(t, err, "expected error when no store is configured")
}
