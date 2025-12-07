package storage

import (
	"fmt"

	"github.com/klauspost/reedsolomon"
)

// ECParams defines how many data & parity shards we use.
type ECParams struct {
	DataShards   int
	ParityShards int
}

func (p ECParams) TotalShards() int {
	return p.DataShards + p.ParityShards
}

// Validate checks that the parameters make sense.
func (p ECParams) Validate() error {
	if p.DataShards <= 0 {
		return fmt.Errorf("ECParams: DataShards must be > 0")
	}
	if p.ParityShards <= 0 {
		return fmt.Errorf("ECParams: ParityShards must be > 0")
	}
	if p.TotalShards() > 255 {
		// Limit from reedsolomon implementation.
		return fmt.Errorf("ECParams: total shards must be <= 255")
	}
	return nil
}

// EncodeToShards takes a byte slice and splits it into data+parity shards.
// - It pads the last data shard with zeros if needed so that all shards are equal size.
// - Returns a slice of len(DataShards+ParityShards).
func EncodeToShards(data []byte, params ECParams) ([][]byte, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// Create Reed–Solomon encoder.
	enc, err := reedsolomon.New(params.DataShards, params.ParityShards)
	if err != nil {
		return nil, fmt.Errorf("EncodeToShards: create encoder: %w", err)
	}

	totalShards := params.TotalShards()
	if len(data) == 0 {
		// Edge case: empty input. Still create shards of size 0.
		shards := make([][]byte, totalShards)
		for i := range shards {
			shards[i] = []byte{}
		}
		return shards, nil
	}

	// Size of each data shard (ceil division).
	shardSize := (len(data) + params.DataShards - 1) / params.DataShards

	shards := make([][]byte, totalShards)
	// Fill data shards.
	for i := 0; i < params.DataShards; i++ {
		start := i * shardSize
		end := start + shardSize
		if end > len(data) {
			end = len(data)
		}

		// Allocate full shardSize and copy actual data; padding with zeros.
		shard := make([]byte, shardSize)
		if start < len(data) {
			copy(shard, data[start:end])
		}
		shards[i] = shard
	}

	// Allocate parity shards (empty but correct size).
	for i := params.DataShards; i < totalShards; i++ {
		shards[i] = make([]byte, shardSize)
	}

	// Compute parity.
	if err := enc.Encode(shards); err != nil {
		return nil, fmt.Errorf("EncodeToShards: encode: %w", err)
	}

	return shards, nil
}

// ReconstructFromShards takes a slice of shards (some may be nil) and
// reconstructs the original data of length origSize (un-padded).
//
// - shards must have length == DataShards+ParityShards
// - At least DataShards of them must be non-nil for successful reconstruction.
func ReconstructFromShards(shards [][]byte, params ECParams, origSize int) ([]byte, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if len(shards) != params.TotalShards() {
		return nil, fmt.Errorf("ReconstructFromShards: expected %d shards, got %d",
			params.TotalShards(), len(shards))
	}
	if origSize < 0 {
		return nil, fmt.Errorf("ReconstructFromShards: invalid origSize %d", origSize)
	}
	if origSize == 0 {
		return []byte{}, nil
	}

	enc, err := reedsolomon.New(params.DataShards, params.ParityShards)
	if err != nil {
		return nil, fmt.Errorf("ReconstructFromShards: create encoder: %w", err)
	}

	// Determine shardSize from any non-nil shard and verify consistency.
	shardSize := -1
	for _, s := range shards {
		if s != nil {
			if shardSize == -1 {
				shardSize = len(s)
			} else if len(s) != shardSize {
				return nil, fmt.Errorf("ReconstructFromShards: inconsistent shard sizes")
			}
		}
	}
	if shardSize == -1 {
		return nil, fmt.Errorf("ReconstructFromShards: no shard data present")
	}

	// IMPORTANT: Do NOT allocate for nil shards here.
	// The reedsolomon library interprets nil slices as "missing shards"
	// and will reconstruct them in-place.
	if err := enc.Reconstruct(shards); err != nil {
		return nil, fmt.Errorf("ReconstructFromShards: reconstruct: %w", err)
	}

	// Now all shards[i] should be non-nil of equal length.
	// Join the first DataShards and trim to origSize.
	data := make([]byte, params.DataShards*shardSize)
	offset := 0
	for i := 0; i < params.DataShards; i++ {
		if shards[i] == nil {
			return nil, fmt.Errorf("ReconstructFromShards: shard %d still nil after reconstruct", i)
		}
		copy(data[offset:], shards[i])
		offset += shardSize
	}

	if origSize > len(data) {
		return nil, fmt.Errorf("ReconstructFromShards: origSize %d > reconstructed %d",
			origSize, len(data))
	}
	return data[:origSize], nil
}
