package storage

import (
	"fmt"
	"io"
)

// ECChunkMeta describes one *logical* file chunk and how it is
// represented by EC shards inside the ChunkStore.
type ECChunkMeta struct {
	Index  int           `json:"index"`  // 0-based chunk index in the file
	Layout ECChunkLayout `json:"layout"` // the shard layout for this chunk
}

// ECFileMeta describes a whole file stored using erasure-coded chunks.
//
// This is intentionally separate from your existing FileMeta so we
// don't break the Merkle-based path. Later we can unify them if we want.
type ECFileMeta struct {
	Name      string        `json:"name"`       // logical file name (optional)
	Size      int64         `json:"size"`       // total file size in bytes
	ChunkSize int           `json:"chunk_size"` // target chunk size in bytes
	Params    ECParams      `json:"params"`     // EC parameters used for all chunks
	Chunks    []ECChunkMeta `json:"chunks"`     // one entry per logical file chunk
}

// BuildECFileMetaFromReader reads from r, splits the input into chunks
// of at most chunkSize bytes, encodes each chunk into EC shards,
// stores them in the ChunkStore, and returns ECFileMeta.
//
// It does NOT compute a Merkle root or interact with the DHT. This is
// purely storage-level EC logic.
func BuildECFileMetaFromReader(
	store ChunkStore,
	name string,
	r io.Reader,
	chunkSize int,
	params ECParams,
) (*ECFileMeta, error) {
	if store == nil {
		return nil, fmt.Errorf("BuildECFileMetaFromReader: store is nil")
	}
	if chunkSize <= 0 {
		return nil, fmt.Errorf("BuildECFileMetaFromReader: invalid chunkSize %d", chunkSize)
	}
	if err := params.Validate(); err != nil {
		return nil, fmt.Errorf("BuildECFileMetaFromReader: invalid EC params: %w", err)
	}

	meta := &ECFileMeta{
		Name:      name,
		Size:      0,
		ChunkSize: chunkSize,
		Params:    params,
		Chunks:    make([]ECChunkMeta, 0, 16),
	}

	buf := make([]byte, chunkSize)
	index := 0

	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := buf[:n]

			// Encode and store this chunk using EC.
			layout, err := EncodeChunkWithEC(store, chunk, params)
			if err != nil {
				return nil, fmt.Errorf("BuildECFileMetaFromReader: chunk %d: %w", index, err)
			}

			meta.Chunks = append(meta.Chunks, ECChunkMeta{
				Index:  index,
				Layout: *layout,
			})

			meta.Size += int64(n)
			index++
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("BuildECFileMetaFromReader: read error: %w", err)
		}
	}

	return meta, nil
}

// ReconstructFileFromECMeta reconstructs a file described by ECFileMeta
// from EC shards stored in ChunkStore and writes the full file contents
// to w.
//
// This does not interact with the DHT. We assume all shards live in the
// provided store (or enough of them to reconstruct each chunk).
func ReconstructFileFromECMeta(
	store ChunkStore,
	meta *ECFileMeta,
	w io.Writer,
) error {
	if store == nil {
		return fmt.Errorf("ReconstructFileFromECMeta: store is nil")
	}
	if meta == nil {
		return fmt.Errorf("ReconstructFileFromECMeta: meta is nil")
	}
	if err := meta.Params.Validate(); err != nil {
		return fmt.Errorf("ReconstructFileFromECMeta: invalid EC params: %w", err)
	}

	for _, cm := range meta.Chunks {
		chunkData, err := DecodeChunkWithEC(store, &cm.Layout)
		if err != nil {
			return fmt.Errorf("ReconstructFileFromECMeta: chunk %d: %w", cm.Index, err)
		}

		if _, err := w.Write(chunkData); err != nil {
			return fmt.Errorf("ReconstructFileFromECMeta: write chunk %d: %w", cm.Index, err)
		}
	}

	return nil
}
