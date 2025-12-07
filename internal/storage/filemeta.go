package storage

import (
	"fmt"
	"io"
	"os"
)

// FileMeta describes a stored file in terms of
// its Merkle root, its chunks, and total size.
type FileMeta struct {
	Root   ChunkID     `json:"root"`   // Merkle root over chunk IDs
	Chunks []ChunkMeta `json:"chunks"` // ordered chunks
	Size   int64       `json:"size"`   // total bytes in the file
}

// BuildFileMetaFromReader reads data from r, splits it into chunks,
// stores them in the given store, computes the Merkle root over the
// chunk IDs, and returns a FileMeta.
//
// This does NOT store FileMeta itself anywhere; it just returns it.
// You can later persist it (e.g., as JSON) or distribute it via the DHT.
func BuildFileMetaFromReader(store ChunkStore, r io.Reader, chunkSize int) (*FileMeta, error) {
	if store == nil {
		return nil, fmt.Errorf("BuildFileMetaFromReader: store is nil")
	}
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	chunks, err := StoreFromReader(store, r, chunkSize)
	if err != nil {
		return nil, fmt.Errorf("BuildFileMetaFromReader: %w", err)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("BuildFileMetaFromReader: no data (empty file/reader)")
	}

	ids := make([]ChunkID, len(chunks))
	var totalSize int64
	for i, cm := range chunks {
		ids[i] = cm.ID
		totalSize += cm.Size
	}

	root, err := MerkleRootForChunks(ids)
	if err != nil {
		return nil, fmt.Errorf("BuildFileMetaFromReader: MerkleRootForChunks: %w", err)
	}

	return &FileMeta{
		Root:   root,
		Chunks: chunks,
		Size:   totalSize,
	}, nil
}

// StoreFileWithMerkle opens the file at path, stores its contents
// as chunks in the store, and returns a FileMeta with the Merkle root.
func StoreFileWithMerkle(store ChunkStore, path string, chunkSize int) (*FileMeta, error) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("StoreFileWithMerkle: open: %w", err)
	}
	defer f.Close()

	return BuildFileMetaFromReader(store, f, chunkSize)
}
