package storage

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

// DefaultChunkSize is the default chunk size used when splitting files.
// 1 MiB is a reasonable starting point; we can make this configurable later.
const DefaultChunkSize = 1 << 20 // 1 MiB

// ChunkMeta describes a stored chunk that belongs to a file.
type ChunkMeta struct {
	Index int     // index of the chunk in the file (0-based)
	ID    ChunkID // content-addressed ID of the chunk
	Size  int64   // number of bytes in this chunk
}

// StoreFromReader reads data from r, splits it into fixed-size chunks,
// stores each chunk in the given ChunkStore, and returns metadata for each chunk.
//
// It does NOT build a Merkle tree yet; it only stores the chunks and
// returns their IDs and sizes in order.
func StoreFromReader(store ChunkStore, r io.Reader, chunkSize int) ([]ChunkMeta, error) {
	if store == nil {
		return nil, fmt.Errorf("StoreFromReader: store is nil")
	}
	if chunkSize <= 0 {
		return nil, fmt.Errorf("StoreFromReader: invalid chunkSize %d", chunkSize)
	}

	br := bufio.NewReader(r)
	buf := make([]byte, chunkSize)

	var chunks []ChunkMeta
	index := 0

	for {
		n, err := io.ReadFull(br, buf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				// Last partial chunk (or empty if n == 0)
				if n == 0 {
					break
				}
				// Use only the bytes read.
				cid, err := store.PutChunk(buf[:n])
				if err != nil {
					return nil, fmt.Errorf("StoreFromReader: PutChunk: %w", err)
				}
				chunks = append(chunks, ChunkMeta{
					Index: index,
					ID:    cid,
					Size:  int64(n),
				})
				break
			}
			return nil, fmt.Errorf("StoreFromReader: read error: %w", err)
		}

		// Full chunk.
		cid, err := store.PutChunk(buf[:n])
		if err != nil {
			return nil, fmt.Errorf("StoreFromReader: PutChunk: %w", err)
		}
		chunks = append(chunks, ChunkMeta{
			Index: index,
			ID:    cid,
			Size:  int64(n),
		})
		index++
	}

	return chunks, nil
}

// StoreFile opens the file at the given path, reads it, and stores it as
// a sequence of chunks in the store using the provided chunk size.
// It returns the ordered list of ChunkMeta for the file.
func StoreFile(store ChunkStore, path string, chunkSize int) ([]ChunkMeta, error) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("StoreFile: open: %w", err)
	}
	defer f.Close()

	return StoreFromReader(store, f, chunkSize)
}
