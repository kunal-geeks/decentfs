package dht

// findContactInRT searches the routing table for a contact with the given ID.
// Returns a pointer to the Contact if found, or nil otherwise.

func findContactInRT(rt *RoutingTable, id ID) *Contact {
	for i := 0; i < IDBits; i++ {
		for _, c := range rt.buckets[i].contacts {
			if c.ID.Equals(id) {
				return &c
			}
		}
	}
	return nil
}
