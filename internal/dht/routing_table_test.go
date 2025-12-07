package dht

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingTable_AddContactAndBucketIndex(t *testing.T) {
	self := MustRandomID()
	rt := NewRoutingTable(self)

	// Create a contact with a different ID.
	other := MustRandomID()
	contact := Contact{
		ID:       other,
		Address:  "127.0.0.1:9001",
		LastSeen: time.Now(),
	}

	rt.AddContact(contact)

	// Compute bucket index manually and check that the bucket is non-empty.
	idx := rt.bucketIndex(other)
	b := rt.buckets[idx]
	require.NotNil(t, b, "bucket should not be nil")
	assert.Greater(t, len(b.contacts), 0, "bucket should contain at least one contact")
	assert.True(t, b.contacts[0].ID.Equals(other), "stored contact ID mismatch")
}

func TestRoutingTable_DoesNotStoreSelf(t *testing.T) {
	self := MustRandomID()
	rt := NewRoutingTable(self)

	contact := Contact{
		ID:       self,
		Address:  "127.0.0.1:9000",
		LastSeen: time.Now(),
	}

	rt.AddContact(contact)

	// Check that no bucket contains self.
	for i := 0; i < IDBits; i++ {
		for _, c := range rt.buckets[i].contacts {
			assert.False(t, self.Equals(c.ID), "routing table should not store self")
		}
	}
}

func TestRoutingTable_BucketCapacityAndLRU(t *testing.T) {
	self := MustRandomID()
	rt := NewRoutingTable(self)

	// We'll target the same bucket index by carefully constructing IDs.
	// Easiest: use self and flip the same bit for each contact.
	base := self

	// Pick a bit to flip (e.g., the last bit) to ensure they are at same distance bucket.
	// In practice, bucketIndex(self XOR mask) will be stable for a fixed mask pattern
	// across different contacts generated with that mask.
	var mask ID
	mask[IDBytes-1] = 0x01 // flip last bit.

	idx := rt.bucketIndex(base.XOR(mask))

	// Insert K+5 contacts that all map to this bucket index.
	for i := 0; i < K+5; i++ {
		cid := base.XOR(mask)
		cid[IDBytes-1] ^= byte(i) // tweak last byte, staying in a close range.

		c := Contact{
			ID:       cid,
			Address:  "127.0.0.1:9000",
			LastSeen: time.Now(),
		}
		rt.AddContact(c)
	}

	b := rt.buckets[idx]
	require.NotNil(t, b, "bucket should not be nil")

	// Capacity should not exceed K.
	assert.LessOrEqual(t, len(b.contacts), K, "bucket should not exceed capacity K")

	// Now pick one existing contact and re-add it, it should move to front (LRU behavior).
	if len(b.contacts) == 0 {
		t.Fatalf("bucket has no contacts; test setup failed")
	}
	oldLast := b.contacts[len(b.contacts)-1] // least recently used
	rt.AddContact(Contact{
		ID:       oldLast.ID,
		Address:  oldLast.Address,
		LastSeen: time.Now(),
	})

	// After re-adding, this ID should be at the front.
	assert.True(t, b.contacts[0].ID.Equals(oldLast.ID), "re-added contact should move to front")
}

func TestRoutingTable_FindClosest(t *testing.T) {
	self := MustRandomID()
	rt := NewRoutingTable(self)

	// Create a few contacts with varying distances.
	for i := 0; i < 10; i++ {
		id := MustRandomID()
		c := Contact{
			ID:       id,
			Address:  fmt.Sprintf("127.0.0.1:900%d", i),
			LastSeen: time.Now(),
		}
		rt.AddContact(c)
	}

	target := MustRandomID()

	closest := rt.FindClosest(target, 5)
	require.NotNil(t, closest)
	require.LessOrEqual(t, len(closest), 5, "should not return more than requested")

	// Verify that the slice is sorted by increasing distance.
	for i := 1; i < len(closest); i++ {
		prevDist := target.XOR(closest[i-1].ID)
		currDist := target.XOR(closest[i].ID)

		// prevDist should be <= currDist lexicographically
		assert.True(t, isDistanceLessOrEqual(prevDist, currDist), "closest contacts not sorted by distance")
	}
}

func isDistanceLessOrEqual(a, b ID) bool {
	for i := 0; i < IDBytes; i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return true // equal
}
