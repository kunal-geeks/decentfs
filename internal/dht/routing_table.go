package dht

import (
	"log"
	"sort"
	"sync"
	"time"
)

// Contact represents a node in the DHT.
type Contact struct {
	ID       ID
	Address  string
	LastSeen time.Time
}

// bucket holds up to K contacts with "least recently seen at the back" semantics.
type bucket struct {
	contacts []Contact
}

// touch moves the contact with given ID to the front if it exists.
// Returns true if found, false otherwise.
func (b *bucket) touch(id ID) bool {
	for i, c := range b.contacts {
		if c.ID.Equals(id) {
			// Move to front: [c] + others (without old position)
			if i == 0 {
				return true
			}
			b.contacts = append([]Contact{c}, append(b.contacts[:i], b.contacts[i+1:]...)...)
			return true
		}
	}
	return false
}

// add inserts a new contact at the front, evicting the least recently seen
// if the bucket is full (size > K).
func (b *bucket) add(c Contact) {
	// Prepend new contact.
	b.contacts = append([]Contact{c}, b.contacts...)

	// Evict least recently seen if over capacity.
	if len(b.contacts) > K {
		b.contacts = b.contacts[:K]
	}
}

// all returns a copy of all contacts in this bucket.
func (b *bucket) all() []Contact {
	out := make([]Contact, len(b.contacts))
	copy(out, b.contacts)
	return out
}

// RoutingTable is a Kademlia routing table for a single node.
// It maintains IDBits buckets, each bucket storing up to K contacts.
type RoutingTable struct {
	self    ID
	buckets [IDBits]*bucket
	mu      sync.RWMutex
}

// NewRoutingTable initializes a routing table for the given local ID.
func NewRoutingTable(self ID) *RoutingTable {
	rt := &RoutingTable{
		self: self,
	}
	for i := 0; i < IDBits; i++ {
		rt.buckets[i] = &bucket{}
	}
	return rt
}

// bucketIndex returns the index of the bucket for the given ID.
// It is based on the XOR distance between the local ID and the target ID.
//
// Kademlia uses the prefix length of the distance. We clamp it to
// [0, IDBits-1] so we always have a valid bucket index.
func (rt *RoutingTable) bucketIndex(id ID) int {
	dist := rt.self.XOR(id)
	prefix := dist.PrefixLen()
	if prefix >= IDBits {
		// Same ID as self → special case; we won't store self in the table.
		return IDBits - 1
	}
	return prefix
}

// AddContact adds or updates a contact in the routing table.
func (rt *RoutingTable) AddContact(c Contact) {
	// Don't add ourselves to the table.
	if rt.self.Equals(c.ID) {
		return
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	idx := rt.bucketIndex(c.ID)
	b := rt.buckets[idx]

	now := time.Now()

	// If contact already exists, move it to front and update LastSeen.
	if b.touch(c.ID) {
		// touch moved the existing contact to index 0.
		b.contacts[0].LastSeen = now
		log.Printf("[dht] refreshed contact %s at %s in bucket %d\n",
			c.ID.String(), c.Address, idx)
		return
	}

	// New contact: set LastSeen here so we don't rely on caller,
	// then add to bucket (with capacity enforcement).
	c.LastSeen = now
	b.add(c)

	log.Printf("[dht] added contact %s at %s to bucket %d (size=%d)\n",
		c.ID.String(), c.Address, idx, len(b.contacts))
}

// FindClosest returns up to n contacts closest to the target ID,
// based on XOR distance. It merges contacts from all buckets and sorts them.
func (rt *RoutingTable) FindClosest(target ID, n int) []Contact {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Gather all contacts from all buckets.
	var allContacts []Contact
	for i := 0; i < IDBits; i++ {
		allContacts = append(allContacts, rt.buckets[i].all()...)
	}

	if len(allContacts) == 0 {
		return nil
	}

	// Sort by XOR distance to target.
	sort.Slice(allContacts, func(i, j int) bool {
		di := target.XOR(allContacts[i].ID)
		dj := target.XOR(allContacts[j].ID)
		// Compare byte-by-byte lexicographically (smaller = closer).
		for k := 0; k < IDBytes; k++ {
			if di[k] < dj[k] {
				return true
			}
			if di[k] > dj[k] {
				return false
			}
		}
		// Equal distance, arbitrary order.
		return false
	})

	if n > len(allContacts) {
		n = len(allContacts)
	}
	return allContacts[:n]
}
