package storage

import (
	"bytes"
	"strings"
	"testing"
)

func TestECFileMeta_BuildAndReconstruct_SmallText(t *testing.T) {
	store := newInMemoryChunkStore()
	params := ECParams{
		DataShards:   4,
		ParityShards: 2,
	}

	original := "Hello EC file storage!\nThis is a test across multiple chunks."
	r := strings.NewReader(original)

	// Use a small chunkSize to force multiple chunks.
	chunkSize := 16

	meta, err := BuildECFileMetaFromReader(store, "test.txt", r, chunkSize, params)
	if err != nil {
		t.Fatalf("BuildECFileMetaFromReader error: %v", err)
	}

	if meta.Name != "test.txt" {
		t.Fatalf("unexpected name: got %q, want %q", meta.Name, "test.txt")
	}
	if meta.Size != int64(len(original)) {
		t.Fatalf("unexpected size: got %d, want %d", meta.Size, len(original))
	}
	if len(meta.Chunks) == 0 {
		t.Fatalf("expected at least 1 chunk")
	}

	// Reconstruct into a buffer.
	var buf bytes.Buffer
	if err := ReconstructFileFromECMeta(store, meta, &buf); err != nil {
		t.Fatalf("ReconstructFileFromECMeta error: %v", err)
	}

	if got := buf.String(); got != original {
		t.Fatalf("reconstructed content mismatch:\n got: %q\nwant: %q", got, original)
	}
}
