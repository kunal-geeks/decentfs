package storage

import (
	"crypto/sha256"
	"fmt"
)

// MerkleRootForChunks computes a Merkle root over an ordered list
// of chunk IDs. The leaves of the tree are the given ChunkIDs.
// Internal nodes are computed as SHA256(left || right).
// If a level has an odd number of nodes, the last one is promoted
// unchanged to the next level.
//
// If ids is empty, an error is returned.
func MerkleRootForChunks(ids []ChunkID) (ChunkID, error) {
	var zero ChunkID

	if len(ids) == 0 {
		return zero, fmt.Errorf("MerkleRootForChunks: no chunks provided")
	}
	if len(ids) == 1 {
		// Single leaf: root is the leaf itself.
		return ids[0], nil
	}

	// Work on a copy so we don't mutate caller's slice.
	cur := make([]ChunkID, len(ids))
	copy(cur, ids)

	buf := make([]byte, 0, 64) // we will reuse this buffer

	for len(cur) > 1 {
		var next []ChunkID

		for i := 0; i < len(cur); i += 2 {
			if i+1 >= len(cur) {
				// Odd element: promote as-is.
				next = append(next, cur[i])
				continue
			}

			// Hash left || right.
			buf = buf[:0]
			buf = append(buf, cur[i][:]...)
			buf = append(buf, cur[i+1][:]...)

			sum := sha256.Sum256(buf)
			next = append(next, ChunkID(sum))
		}

		cur = next
	}

	return cur[0], nil
}
