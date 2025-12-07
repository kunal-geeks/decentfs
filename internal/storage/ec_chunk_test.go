package storage

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
)

// inMemoryChunkStore is a simple in-memory ChunkStore for tests.
type inMemoryChunkStore struct {
	data map[ChunkID][]byte
	next uint64
}

func newInMemoryChunkStore() *inMemoryChunkStore {
	return &inMemoryChunkStore{
		data: make(map[ChunkID][]byte),
	}
}

func (s *inMemoryChunkStore) PutChunk(b []byte) (ChunkID, error) {
	// Use a simple counter-based ID and convert it via ChunkIDFromHex.
	s.next++

	// 64 hex chars → 32 bytes; ChunkIDFromHex will parse this into a ChunkID.
	hexID := fmt.Sprintf("%064x", s.next)

	id, err := ChunkIDFromHex(hexID)
	if err != nil {
		return ChunkID{}, fmt.Errorf("PutChunk: ChunkIDFromHex failed: %w", err)
	}

	// Copy data to avoid aliasing.
	cp := make([]byte, len(b))
	copy(cp, b)
	s.data[id] = cp

	return id, nil
}

func (s *inMemoryChunkStore) GetChunk(id ChunkID) ([]byte, error) {
	b, ok := s.data[id]
	if !ok {
		return nil, fmt.Errorf("chunk %s not found", id.String())
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp, nil
}

func (s *inMemoryChunkStore) HasChunk(id ChunkID) (bool, error) {
	_, ok := s.data[id]
	return ok, nil
}

func (s *inMemoryChunkStore) DeleteChunk(id ChunkID) error {
	delete(s.data, id)
	return nil
}

func (s *inMemoryChunkStore) ListChunks() ([]ChunkID, error) {
	out := make([]ChunkID, 0, len(s.data))
	for id := range s.data {
		out = append(out, id)
	}
	return out, nil
}

func TestEncodeDecodeChunkWithEC_Simple(t *testing.T) {
	store := newInMemoryChunkStore()
	params := ECParams{
		DataShards:   4,
		ParityShards: 2,
	}

	original := []byte("this is a small logical chunk for EC testing")

	layout, err := EncodeChunkWithEC(store, original, params)
	if err != nil {
		t.Fatalf("EncodeChunkWithEC error: %v", err)
	}

	if got, want := len(layout.ShardIDs), params.TotalShards(); got != want {
		t.Fatalf("expected %d shard IDs, got %d", want, got)
	}
	if layout.OrigSize != len(original) {
		t.Fatalf("OrigSize mismatch: got %d, want %d", layout.OrigSize, len(original))
	}

	// Decode with all shards present.
	rec, err := DecodeChunkWithEC(store, layout)
	if err != nil {
		t.Fatalf("DecodeChunkWithEC error: %v", err)
	}

	if !bytes.Equal(rec, original) {
		t.Fatalf("decoded data mismatch:\n got: %q\nwant: %q", rec, original)
	}
}

func TestEncodeDecodeChunkWithEC_MissingShards(t *testing.T) {
	store := newInMemoryChunkStore()
	params := ECParams{
		DataShards:   4,
		ParityShards: 2,
	}

	// Random data
	original := make([]byte, 1024)
	if _, err := rand.Read(original); err != nil {
		t.Fatalf("rand.Read error: %v", err)
	}

	layout, err := EncodeChunkWithEC(store, original, params)
	if err != nil {
		t.Fatalf("EncodeChunkWithEC error: %v", err)
	}

	// Simulate missing shards by deleting up to ParityShards entries from the store.
	// We'll remove shard 0 and 4 (two shards, parity=2 -> recoverable).
	if len(layout.ShardIDs) < 5 {
		t.Fatalf("expected at least 5 shards, got %d", len(layout.ShardIDs))
	}

	// Use DeleteChunk so we go through the same interface.
	_ = store.DeleteChunk(layout.ShardIDs[0])
	_ = store.DeleteChunk(layout.ShardIDs[4])

	rec, err := DecodeChunkWithEC(store, layout)
	if err != nil {
		t.Fatalf("DecodeChunkWithEC error with missing shards: %v", err)
	}

	if !bytes.Equal(rec, original) {
		t.Fatalf("decoded data mismatch with missing shards")
	}
}
