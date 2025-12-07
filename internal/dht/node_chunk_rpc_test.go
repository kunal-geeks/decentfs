package dht

import (
	"io/fs"
	"testing"
	"time"

	"github.com/kunal-geeks/decentfs/internal/p2p"
	"github.com/kunal-geeks/decentfs/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reuse or define a simple in-memory chunk store for tests.
type testChunkStore struct {
	chunks map[storage.ChunkID][]byte
}

func newTestChunkStore() *testChunkStore {
	return &testChunkStore{
		chunks: make(map[storage.ChunkID][]byte),
	}
}

func (m *testChunkStore) PutChunk(data []byte) (storage.ChunkID, error) {
	id := storage.NewChunkID(data)
	b := make([]byte, len(data))
	copy(b, data)
	m.chunks[id] = b
	return id, nil
}

func (m *testChunkStore) GetChunk(id storage.ChunkID) ([]byte, error) {
	data, ok := m.chunks[id]
	if !ok {
		return nil, fs.ErrNotExist
	}
	b := make([]byte, len(data))
	copy(b, data)
	return b, nil
}

func (m *testChunkStore) HasChunk(id storage.ChunkID) (bool, error) {
	_, ok := m.chunks[id]
	return ok, nil
}

func (m *testChunkStore) DeleteChunk(id storage.ChunkID) error {
	delete(m.chunks, id)
	return nil
}

func (m *testChunkStore) ListChunks() ([]storage.ChunkID, error) {
	ids := make([]storage.ChunkID, 0, len(m.chunks))
	for id := range m.chunks {
		ids = append(ids, id)
	}
	return ids, nil
}

func TestNode_HandleRPC_StoreChunk(t *testing.T) {
	self := MustRandomID()
	node := NewNode(self)
	store := newTestChunkStore()
	node.store = store

	data := []byte("hello remote chunk")

	msg := &Message{
		Type:      MsgStoreChunk,
		From:      MustRandomID(),
		Timestamp: time.Now().Unix(),
		ChunkData: data,
	}

	payload, err := msg.Encode()
	require.NoError(t, err)

	rpc := p2p.RPC{
		From:    "127.0.0.1:9001",
		Stream:  false,
		Payload: payload,
	}

	resp, err := node.HandleRPC(rpc)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, MsgStoreChunkResult, resp.Type)
	assert.Empty(t, resp.Error, "expected no error in StoreChunkResult")
	assert.NotEmpty(t, resp.ChunkID, "expected ChunkID in response")

	// Verify store has the chunk.
	cid, err := storage.ChunkIDFromHex(resp.ChunkID)
	require.NoError(t, err)

	has, err := store.HasChunk(cid)
	require.NoError(t, err)
	assert.True(t, has, "store should contain chunk after STORE_CHUNK")
}

func TestNode_HandleRPC_GetChunk(t *testing.T) {
	self := MustRandomID()
	node := NewNode(self)
	store := newTestChunkStore()
	node.store = store

	data := []byte("chunk to retrieve")
	// Pre-store locally.
	cid, err := store.PutChunk(data)
	require.NoError(t, err)

	msg := &Message{
		Type:      MsgGetChunk,
		From:      MustRandomID(),
		Timestamp: time.Now().Unix(),
		ChunkID:   cid.String(),
	}

	payload, err := msg.Encode()
	require.NoError(t, err)

	rpc := p2p.RPC{
		From:    "127.0.0.1:9002",
		Stream:  false,
		Payload: payload,
	}

	resp, err := node.HandleRPC(rpc)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, MsgChunkData, resp.Type)
	assert.Empty(t, resp.Error, "expected no error in ChunkData")
	assert.Equal(t, cid.String(), resp.ChunkID)
	assert.Equal(t, data, resp.ChunkData)
}
