package dht

import (
	"log"
	// "sort"
	// "sync"
	"time"
)

// // A single lookup operation for target ID.
// type Lookup struct {
// 	target    ID
// 	shortlist []Contact
// 	queried   map[string]bool // addr → true if already queried
// 	svc       *Service
// 	mutex     sync.Mutex // protect shortlist
// }

// func (l *Lookup) sortShortlist() {
// 	sort.Slice(l.shortlist, func(i, j int) bool {
// 		di := l.target.XOR(l.shortlist[i].ID)
// 		dj := l.target.XOR(l.shortlist[j].ID)
// 		return di.Less(dj)
// 	})
// }

// func (l *Lookup) addContacts(cs []Contact) {
// 	l.mutex.Lock()
// 	defer l.mutex.Unlock()

// 	seen := make(map[string]bool)

// 	// mark existing contacts
// 	for _, c := range l.shortlist {
// 		seen[c.ID.String()] = true
// 	}

// 	// add new contacts
// 	for _, c := range cs {
// 		if l.svc.node.id.Equals(c.ID) {
// 			continue
// 		}
// 		if seen[c.ID.String()] {
// 			continue
// 		}
// 		// Append and add to routing table too
// 		l.shortlist = append(l.shortlist, c)
// 		l.svc.node.rt.AddContact(c)
// 	}

// 	// Sort by closeness to target
// 	l.sortShortlist()

// 	// Trim to K
// 	if len(l.shortlist) > K {
// 		l.shortlist = l.shortlist[:K]
// 	}
// }

// func (l *Lookup) nextToQuery() []Contact {
// 	l.mutex.Lock()
// 	defer l.mutex.Unlock()

// 	var out []Contact

// 	for _, c := range l.shortlist {
// 		if len(out) >= Alpha {
// 			break
// 		}
// 		if l.queried[c.Address] {
// 			continue
// 		}
// 		out = append(out, c)
// 	}

// 	// Mark them as queried
// 	for _, c := range out {
// 		l.queried[c.Address] = true
// 	}

// 	return out
// }

// func (s *Service) Lookup(target ID) ([]Contact, error) {
// 	log.Printf("[dht] starting lookup for %s\n", target.String())

// 	lookup := &Lookup{
// 		target:    target,
// 		svc:       s,
// 		shortlist: s.node.rt.FindClosest(target, K),
// 		queried:   make(map[string]bool),
// 	}

// 	lookup.sortShortlist()

// 	round := 0

// 	for {
// 		round++
// 		log.Printf("[dht] lookup round %d — shortlist size=%d\n",
// 			round, len(lookup.shortlist))

// 		// Pick next Alpha contacts to ask
// 		toQuery := lookup.nextToQuery()
// 		if len(toQuery) == 0 {
// 			log.Printf("[dht] lookup reached convergence\n")
// 			break
// 		}

// 		log.Printf("[dht] querying %d peers for FIND_NODE(%s)\n",
// 			len(toQuery), target.String())

// 		var wg sync.WaitGroup
// 		respChan := make(chan []Contact, len(toQuery))

// 		for _, c := range toQuery {
// 			wg.Add(1)
// 			go func(contact Contact) {
// 				defer wg.Done()
// 				contacts := s.queryFindNode(contact, target)
// 				if contacts != nil {
// 					respChan <- contacts
// 				}
// 			}(c)
// 		}

// 		wg.Wait()
// 		close(respChan)

// 		previousClosest := lookup.shortlist[0].ID

// 		// Merge all responses
// 		for contacts := range respChan {
// 			lookup.addContacts(contacts)
// 		}

// 		// Check if closest changed
// 		if len(lookup.shortlist) > 0 {
// 			if lookup.shortlist[0].ID.Equals(previousClosest) {
// 				// No improvement — converged
// 				log.Printf("[dht] lookup converged, closest unchanged\n")
// 				break
// 			}
// 		}

// 		// otherwise loop again
// 	}

// 	return lookup.shortlist, nil
// }

// func (s *Service) queryFindNode(c Contact, target ID) []Contact {
// 	log.Printf("[dht] sending FIND_NODE(%s) to %s\n",
// 		target.String(), c.Address)

// 	err := s.SendFindNode(c.Address, target)
// 	if err != nil {
// 		log.Printf("[dht] error sending FIND_NODE to %s: %v\n",
// 			c.Address, err)
// 		return nil
// 	}

// 	// Now wait for a NODES response coming from that address
// 	timeout := time.After(500 * time.Millisecond)

// 	for {
// 		select {
// 		case rpc := <-s.transport.Consume():
// 			msg, err := DecodeMessage(rpc.Payload)
// 			if err != nil {
// 				continue
// 			}
// 			if msg.Type == MsgNodes && rpc.From == c.Address {
// 				return msg.Nodes
// 			}

// 		case <-timeout:
// 			log.Printf("[dht] timeout waiting for NODES from %s\n", c.Address)
// 			return nil
// 		}
// 	}
// }

// Lookup performs a simplified iterative Kademlia FIND_NODE lookup.
// It tries to find up to K contacts closest to the given target ID.
//
// Algorithm (simplified):
//  1. Start from the current closest contacts in our routing table.
//  2. In each round, pick up to Alpha closest contacts we haven't queried yet.
//  3. Send FIND_NODE(target) to them.
//  4. Wait briefly so their NODES responses can be processed by Service.readLoop
//     and Node.HandleRPC (which updates the routing table).
//  5. Recompute the closest contacts.
//  6. Stop when:
//     - there are no new peers to query, or
//     - the closest contact doesn't change between rounds, or
//     - we hit a maximum number of rounds.
func (s *Service) Lookup(target ID) ([]Contact, error) {
	const maxRounds = 5

	log.Printf("[dht] lookup: starting lookup for target %s\n", target.String())

	queried := make(map[string]bool)

	var prevClosest ID
	havePrev := false

	for round := 0; round < maxRounds; round++ {
		shortlist := s.node.rt.FindClosest(target, K)
		if len(shortlist) == 0 {
			log.Printf("[dht] lookup: no peers in routing table\n")
			return nil, nil
		}

		log.Printf("[dht] lookup: round %d, shortlist size=%d, closest=%s\n",
			round+1, len(shortlist), shortlist[0].ID.String())

		// Check convergence: if closest hasn't changed since last round, stop.
		if havePrev && shortlist[0].ID.Equals(prevClosest) {
			log.Printf("[dht] lookup: converged (closest unchanged)\n")
			break
		}
		prevClosest = shortlist[0].ID
		havePrev = true

		// Pick up to Alpha closest contacts that we haven't queried yet.
		toQuery := make([]Contact, 0, Alpha)
		for _, c := range shortlist {
			if len(toQuery) >= Alpha {
				break
			}
			if queried[c.Address] {
				continue
			}
			toQuery = append(toQuery, c)
		}

		if len(toQuery) == 0 {
			log.Printf("[dht] lookup: no new peers to query, stopping\n")
			break
		}

		// Send FIND_NODE to the selected contacts.
		for _, c := range toQuery {
			queried[c.Address] = true
			log.Printf("[dht] lookup: sending FIND_NODE(%s) to %s\n",
				target.String(), c.Address)

			if err := s.SendFindNode(c.Address, target); err != nil {
				log.Printf("[dht] lookup: error sending FIND_NODE to %s: %v\n",
					c.Address, err)
			}
		}

		// Wait a bit so that any NODES responses can arrive and be processed
		// by Service.readLoop + Node.HandleRPC, which will update the routing table.
		time.Sleep(500 * time.Millisecond)
	}

	finalShortlist := s.node.rt.FindClosest(target, K)
	log.Printf("[dht] lookup: finished with %d contacts\n", len(finalShortlist))

	return finalShortlist, nil
}
