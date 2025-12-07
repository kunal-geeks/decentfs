package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSChunkStore_PutGetHasDelete(t *testing.T) {
	dir := t.TempDir()

	store, err := NewFSChunkStore(dir)
	require.NoError(t, err, "NewFSChunkStore should not error")

	data := []byte("this is a test chunk")

	// Put the chunk.
	id, err := store.PutChunk(data)
	require.NoError(t, err, "PutChunk should not error")

	// HasChunk should report true.
	has, err := store.HasChunk(id)
	require.NoError(t, err)
	assert.True(t, has, "HasChunk should be true after PutChunk")

	// GetChunk should return the same data.
	got, err := store.GetChunk(id)
	require.NoError(t, err)
	assert.Equal(t, data, got, "GetChunk should return original data")

	// ListChunks should include this ID.
	all, err := store.ListChunks()
	require.NoError(t, err)
	assert.Len(t, all, 1, "ListChunks should return one chunk")
	assert.True(t, all[0].Equal(id), "ListChunks returned ID should match stored ID")

	// DeleteChunk removes it.
	err = store.DeleteChunk(id)
	require.NoError(t, err)

	// HasChunk should now be false.
	has, err = store.HasChunk(id)
	require.NoError(t, err)
	assert.False(t, has, "HasChunk should be false after DeleteChunk")

	// GetChunk should now error with not-exist.
	_, err = store.GetChunk(id)
	assert.Error(t, err, "GetChunk should error for deleted chunk")
}

func TestFSChunkStore_PutChunk_Idempotent(t *testing.T) {
	dir := t.TempDir()

	store, err := NewFSChunkStore(dir)
	require.NoError(t, err)

	data := []byte("same content")

	id1, err := store.PutChunk(data)
	require.NoError(t, err)

	id2, err := store.PutChunk(data)
	require.NoError(t, err)

	assert.True(t, id1.Equal(id2), "same content should yield same ChunkID")
}
