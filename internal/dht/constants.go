package dht

const (
	// K is the maximum number of contacts in a single Kademlia bucket.
	// Classic Kademlia uses K=20, we'll stick with that.
	K     = 20 // bucket size (already defined in your system)
	Alpha = 3  // parallelism factor for Kademlia lookups
)
