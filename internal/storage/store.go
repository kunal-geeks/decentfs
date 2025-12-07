package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ChunkStore defines the basic operations for a local content-addressable
// chunk storage. It uses ChunkID (SHA-256 of content) as the key.
type ChunkStore interface {
	// PutChunk stores the given data and returns its ChunkID.
	// If the chunk already exists (same content), it should be a no-op.
	PutChunk(data []byte) (ChunkID, error)

	// GetChunk retrieves the data for the given ChunkID.
	// Returns os.ErrNotExist if the chunk is not found.
	GetChunk(id ChunkID) ([]byte, error)

	// HasChunk reports whether the given ChunkID is present.
	HasChunk(id ChunkID) (bool, error)

	// DeleteChunk removes the chunk if it exists.
	// It should not error if the chunk is already missing (idempotent).
	DeleteChunk(id ChunkID) error

	// ListChunks returns all known ChunkIDs in this store.
	ListChunks() ([]ChunkID, error)
}

// FSChunkStore is a filesystem-based implementation of ChunkStore.
// Each chunk is stored as a file named by its hex ChunkID under baseDir.
type FSChunkStore struct {
	baseDir string
}

// NewFSChunkStore creates a new FSChunkStore rooted at baseDir.
// It ensures the directory exists.
func NewFSChunkStore(baseDir string) (*FSChunkStore, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("NewFSChunkStore: baseDir is empty")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("NewFSChunkStore: mkdir: %w", err)
	}
	return &FSChunkStore{baseDir: baseDir}, nil
}

// chunkPath returns the full path on disk for the given ChunkID.
func (s *FSChunkStore) chunkPath(id ChunkID) string {
	// Simple layout: baseDir/<hex-id>
	// Later we can use subdirectories by prefix if needed.
	return filepath.Join(s.baseDir, id.String())
}

// PutChunk computes the ChunkID of data and writes it to disk.
func (s *FSChunkStore) PutChunk(data []byte) (ChunkID, error) {
	id := NewChunkID(data)
	path := s.chunkPath(id)

	// If file already exists, we just return the ID.
	// This makes PutChunk idempotent for the same content.
	if _, err := os.Stat(path); err == nil {
		return id, nil
	}

	// Write atomically: write to temp file then rename.
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return ChunkID{}, fmt.Errorf("PutChunk: write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return ChunkID{}, fmt.Errorf("PutChunk: rename: %w", err)
	}

	return id, nil
}

// GetChunk reads the chunk data for the given ID.
func (s *FSChunkStore) GetChunk(id ChunkID) ([]byte, error) {
	path := s.chunkPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fs.ErrNotExist
		}
		return nil, fmt.Errorf("GetChunk: read: %w", err)
	}
	return data, nil
}

// HasChunk checks whether a file exists for the given ChunkID.
func (s *FSChunkStore) HasChunk(id ChunkID) (bool, error) {
	path := s.chunkPath(id)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("HasChunk: stat: %w", err)
}

// DeleteChunk removes the file if it exists, otherwise it's a no-op.
func (s *FSChunkStore) DeleteChunk(id ChunkID) error {
	path := s.chunkPath(id)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("DeleteChunk: remove: %w", err)
	}
	return nil
}

// ListChunks scans the baseDir and returns all ChunkIDs it finds.
// It assumes each regular file name is a hex-encoded ChunkID.
func (s *FSChunkStore) ListChunks() ([]ChunkID, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("ListChunks: readdir: %w", err)
	}

	var ids []ChunkID

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()

		id, err := ChunkIDFromHex(name)
		if err != nil {
			// Skip files that don't look like valid ChunkIDs.
			continue
		}
		ids = append(ids, id)
	}

	return ids, nil
}
