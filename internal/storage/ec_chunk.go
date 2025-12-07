package storage

import "fmt"

// ECChunkLayout describes how a single logical chunk is split into
// Reed–Solomon data+parity shards stored as individual chunks.
type ECChunkLayout struct {
	Params   ECParams  `json:"params"`
	ShardIDs []ChunkID `json:"shard_ids"`
	OrigSize int       `json:"orig_size"`
}

// EncodeChunkWithEC takes a logical chunk of data, erasure-encodes it
// into data+parity shards, stores each shard in the ChunkStore, and
// returns the ECChunkLayout metadata.
//
// The caller is responsible for persisting this layout somewhere
// (e.g. in FileMeta) so the chunk can be reconstructed later.
func EncodeChunkWithEC(store ChunkStore, data []byte, params ECParams) (*ECChunkLayout, error) {
	if store == nil {
		return nil, fmt.Errorf("EncodeChunkWithEC: store is nil")
	}

	shards, err := EncodeToShards(data, params)
	if err != nil {
		return nil, fmt.Errorf("EncodeChunkWithEC: %w", err)
	}

	shardIDs := make([]ChunkID, len(shards))
	for i, shard := range shards {
		id, err := store.PutChunk(shard)
		if err != nil {
			return nil, fmt.Errorf("EncodeChunkWithEC: PutChunk shard %d: %w", i, err)
		}
		shardIDs[i] = id
	}

	return &ECChunkLayout{
		Params:   params,
		ShardIDs: shardIDs,
		OrigSize: len(data),
	}, nil
}

// DecodeChunkWithEC reconstructs the original chunk data from the shards
// described in layout using the ChunkStore.
//
// It attempts to load all shards; shards that fail to load are treated
// as missing. Reconstruction succeeds as long as at least DataShards
// shards are available.
func DecodeChunkWithEC(store ChunkStore, layout *ECChunkLayout) ([]byte, error) {
	if store == nil {
		return nil, fmt.Errorf("DecodeChunkWithEC: store is nil")
	}
	if layout == nil {
		return nil, fmt.Errorf("DecodeChunkWithEC: layout is nil")
	}
	if err := layout.Params.Validate(); err != nil {
		return nil, fmt.Errorf("DecodeChunkWithEC: invalid params: %w", err)
	}
	if len(layout.ShardIDs) != layout.Params.TotalShards() {
		return nil, fmt.Errorf("DecodeChunkWithEC: expected %d shard IDs, got %d",
			layout.Params.TotalShards(), len(layout.ShardIDs))
	}

	// Load shards from store. Missing shards become nil.
	shards := make([][]byte, len(layout.ShardIDs))
	for i, id := range layout.ShardIDs {
		data, err := store.GetChunk(id)
		if err != nil {
			// Treat as missing shard; log/track at higher layers if needed.
			shards[i] = nil
			continue
		}
		shards[i] = data
	}

	// Reconstruct original data of OrigSize.
	return ReconstructFromShards(shards, layout.Params, layout.OrigSize)
}
