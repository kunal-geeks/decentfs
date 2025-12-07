package storage

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncodeAndReconstruct_Simple(t *testing.T) {
	params := ECParams{
		DataShards:   4,
		ParityShards: 2,
	}

	data := []byte("hello erasure coding in decentfs")

	shards, err := EncodeToShards(data, params)
	if err != nil {
		t.Fatalf("EncodeToShards error: %v", err)
	}

	if got, want := len(shards), params.TotalShards(); got != want {
		t.Fatalf("expected %d shards, got %d", want, got)
	}

	// Simulate losing up to 2 shards (since parity=2).
	// Let's drop shard 1 and 4.
	shardsLost := make([][]byte, len(shards))
	copy(shardsLost, shards)
	shardsLost[1] = nil
	shardsLost[4] = nil

	rec, err := ReconstructFromShards(shardsLost, params, len(data))
	if err != nil {
		t.Fatalf("ReconstructFromShards error: %v", err)
	}

	if !bytes.Equal(rec, data) {
		t.Fatalf("reconstructed data mismatch:\n got: %q\nwant: %q", rec, data)
	}
}

func TestEncodeAndReconstruct_Random(t *testing.T) {
	params := ECParams{
		DataShards:   5,
		ParityShards: 3,
	}

	dataSize := 1024 * 64 // 64 KB
	data := make([]byte, dataSize)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand.Read error: %v", err)
	}

	shards, err := EncodeToShards(data, params)
	if err != nil {
		t.Fatalf("EncodeToShards error: %v", err)
	}

	// Lose up to ParityShards shards at random positions.
	lostCount := 0
	for i := 0; i < len(shards) && lostCount < params.ParityShards; i += 2 {
		shards[i] = nil
		lostCount++
	}

	rec, err := ReconstructFromShards(shards, params, len(data))
	if err != nil {
		t.Fatalf("ReconstructFromShards error: %v", err)
	}

	if !bytes.Equal(rec, data) {
		t.Fatalf("reconstructed data mismatch for random test")
	}
}

func TestECParams_Validate(t *testing.T) {
	cases := []struct {
		name    string
		params  ECParams
		wantErr bool
	}{
		{"ok", ECParams{DataShards: 4, ParityShards: 2}, false},
		{"no data", ECParams{DataShards: 0, ParityShards: 2}, true},
		{"no parity", ECParams{DataShards: 4, ParityShards: 0}, true},
		{"too many", ECParams{DataShards: 300, ParityShards: 1}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.params.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
