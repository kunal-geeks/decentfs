package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/kunal-geeks/decentfs/internal/dht"
	"github.com/kunal-geeks/decentfs/internal/p2p"
	"github.com/kunal-geeks/decentfs/internal/storage"
)

func main() {
	// CLI flags for configuration.
	listenAddr := flag.String("listen", "127.0.0.1:4000", "address to listen on")
	bootstrapStr := flag.String("bootstrap", "", "comma-separated list of bootstrap peers (host:port)")
	lookupIDStr := flag.String("lookup", "", "hex ID to lookup after bootstrap")

	// NEW: simple test flags for storage.
	storeTo := flag.String("store-to", "", "peer address (host:port) to send STORE_CHUNK to")
	storeData := flag.String("store-data", "", "string data to store as a chunk on the remote peer")
	fetchFrom := flag.String("fetch-from", "", "peer address (host:port) to send GET_CHUNK to")
	fetchChunkID := flag.String("fetch-chunk", "", "hex ChunkID to fetch from the remote peer")

	storeFile := flag.String("store-file", "", "path to file to store locally & announce through DHT")
	chunkSize := flag.Int("chunk-size", 1024*256, "chunk size for file storage")

	downloadRoot := flag.String("download-root", "", "file root hash to download (hex)")
	downloadOut := flag.String("out", "", "output file path for downloaded file")

	ecSpec := flag.String("ec", "", `erasure coding params "data+parity", e.g. 4+2 (4 data shards, 2 parity shards)`)

	flag.Parse()

	// Generate a random Kademlia ID for this node.
	selfID := dht.MustRandomID()
	log.Printf("[node] starting with ID: %s\n", selfID.String())

	// Create codec for framing + JSON encoding.
	codec := p2p.NewLengthPrefixedJSONCodec()

	// Configure TCP transport.
	transport := p2p.NewTCPTransport(p2p.TCPTransportOpts{
		ListenAddr: *listenAddr,
		Decoder:    codec,
		Encoder:    codec,
		HandshakeFunc: func(peer p2p.Peer) error {
			log.Printf("[p2p] handshake with %s (outbound=%v)\n", peer.Addr(), peer.Outbound())
			// Later: enforce TLS, JWT, protocol version, etc.
			return nil
		},
		OnPeer: func(peer p2p.Peer) error {
			log.Printf("[p2p] new peer connected: %s (outbound=%v)\n", peer.Addr(), peer.Outbound())
			return nil
		},
	})

	// Start listening for incoming connections.
	if err := transport.ListenAndAccept(); err != nil {
		log.Fatalf("failed to listen on %s: %v", *listenAddr, err)
	}
	log.Printf("[p2p] listening on %s\n", transport.Addr())

	// Create local filesystem chunk store for this node.
	// You can later make this configurable per-node via flags.
	store, err := storage.NewFSChunkStore("./data/chunks")
	if err != nil {
		log.Fatalf("failed to create chunk store: %v", err)
	}

	// Create DHT service (wires Node + Transport + Store).
	svc := dht.NewService(dht.ServiceOpts{
		ID:        selfID,
		Transport: transport,
		Store:     store,
	})

	// Bootstrap if peers provided.
	if *bootstrapStr != "" {
		peers := strings.Split(*bootstrapStr, ",")
		for _, addr := range peers {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			// Dial and send a PING.
			log.Printf("[bootstrap] dialing %s\n", addr)
			if err := transport.Dial(addr); err != nil {
				log.Printf("[bootstrap] dial error to %s: %v\n", addr, err)
				continue
			}

			// Give connection a moment to establish and handshake.
			time.Sleep(100 * time.Millisecond)

			log.Printf("[bootstrap] sending PING to %s\n", addr)
			if err := svc.SendPing(addr); err != nil {
				log.Printf("[bootstrap] SendPing error to %s: %v\n", addr, err)
			}

			// Also send a FIND_NODE for our own ID to exercise routing.
			log.Printf("[bootstrap] sending FIND_NODE(selfID=%s) to %s\n", selfID.String(), addr)
			if err := svc.SendFindNode(addr, selfID); err != nil {
				log.Printf("[bootstrap] SendFindNode error to %s: %v\n", addr, err)
			}
		}
	}

	// give time for PONG + NODES to be processed
	time.Sleep(300 * time.Millisecond)

	// Optional lookup after bootstrap.
	if *lookupIDStr != "" {
		id, err := dht.IDFromHex(*lookupIDStr)
		if err != nil {
			log.Printf("[node] invalid lookup ID %q: %v\n", *lookupIDStr, err)
		} else {
			// Give bootstrap some time to populate routing table
			time.Sleep(1 * time.Second)

			log.Printf("[node] performing lookup for ID: %s\n", id.String())
			contacts, err := svc.Lookup(id)
			if err != nil {
				log.Printf("[node] lookup error: %v\n", err)
			} else {
				log.Printf("[node] lookup result (%d contacts):\n", len(contacts))
				for i, c := range contacts {
					log.Printf("[node]   %d) %s at %s\n", i+1, c.ID.String(), c.Address)
				}
			}
		}
	}

	// Optional simple chunk store RPCs for testing.

	if *storeTo != "" && *storeData != "" {
		log.Printf("[node] storing remote chunk on %s: %q\n", *storeTo, *storeData)
		cid, err := svc.StoreChunkRemote(*storeTo, []byte(*storeData))
		if err != nil {
			log.Printf("[node] StoreChunkRemote error: %v\n", err)
		} else {
			log.Printf("[node] stored chunk with ID: %s\n", cid.String())
		}
	}

	if *fetchFrom != "" && *fetchChunkID != "" {
		log.Printf("[node] fetching remote chunk %s from %s\n", *fetchChunkID, *fetchFrom)

		cid, err := storage.ChunkIDFromHex(*fetchChunkID)
		if err != nil {
			log.Printf("[node] invalid fetch-chunk ID %q: %v\n", *fetchChunkID, err)
		} else {
			data, err := svc.FetchChunkRemote(*fetchFrom, cid)
			if err != nil {
				log.Printf("[node] FetchChunkRemote error: %v\n", err)
			} else {
				log.Printf("[node] fetched chunk %s, data=%q\n", cid.String(), string(data))
			}
		}
	}

	// --- FILE UPLOAD (store + announce) ---
	if *storeFile != "" {
		log.Printf("[node] storing file: %s\n", *storeFile)

		if *ecSpec != "" {
			// EC MODE: use erasure coding for file storage (local only for now).
			params, err := parseECParams(*ecSpec)
			if err != nil {
				log.Fatalf("[node] invalid -ec value %q: %v\n", *ecSpec, err)
			}

			ecMeta, err := svc.StoreFileWithEC(*storeFile, *chunkSize, params)
			if err != nil {
				log.Fatalf("[node] store-file (EC) error: %v\n", err)
			}

			log.Printf("[node] EC file stored successfully (LOCAL ONLY, no DHT announce yet).")
			log.Printf("[node] name:   %s", ecMeta.Name)
			log.Printf("[node] size:   %d bytes", ecMeta.Size)
			log.Printf("[node] chunks: %d", len(ecMeta.Chunks))
			log.Printf("[node] EC:     data=%d parity=%d", ecMeta.Params.DataShards, ecMeta.Params.ParityShards)

			// You might later serialize ecMeta (JSON) and store it somewhere,
			// or announce it into the DHT. For now we just log and exit.
			return
		}

		// NON-EC MODE: existing Merkle + DHT path.
		meta, err := svc.StoreFileLocalAndAnnounce(*storeFile, *chunkSize)
		if err != nil {
			log.Fatalf("[node] store-file error: %v\n", err)
		}

		log.Printf("[node] file stored successfully.")
		log.Printf("[node] file root: %s", meta.Root.String())
		log.Printf("[node] chunks: %d", len(meta.Chunks))
		log.Printf("[node] size: %d bytes", meta.Size)
		return
	}

	// --- FILE DOWNLOAD (via DHT) ---
	if *downloadRoot != "" {
		if *downloadOut == "" {
			log.Fatalf("[download] -out is required")
		}
		rootID, err := storage.ChunkIDFromHex(*downloadRoot)
		if err != nil {
			log.Fatalf("[download] invalid root: %v", err)
		}
		log.Printf("[download] downloading file root=%s -> %s\n",
			rootID.String(), *downloadOut)

		err = svc.DownloadFile(rootID, *downloadOut)
		if err != nil {
			log.Printf("[download] DownloadFile via DHT failed: %v", err)

			// Fallback: if we have a bootstrap peer, try directly.
			if *bootstrapStr != "" {
				peers := strings.Split(*bootstrapStr, ",")
				addr := strings.TrimSpace(peers[0])
				log.Printf("[download] falling back to direct provider %s\n", addr)

				if err2 := svc.DownloadFileFromProvider(rootID, addr, *downloadOut); err2 != nil {
					log.Fatalf("[download] fallback download error: %v", err2)
				}
				log.Printf("[download] DONE via fallback.")
			} else {
				log.Fatalf("[download] error: %v", err)
			}
		} else {
			log.Printf("[download] DONE.")
		}
	}

	// Block forever (or until Ctrl+C). This node now:
	// - Accepts connections
	// - Handles DHT messages
	// - Maintains a routing table
	select {}
}

func parseECParams(spec string) (storage.ECParams, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return storage.ECParams{}, fmt.Errorf("empty EC spec")
	}

	parts := strings.Split(spec, "+")
	if len(parts) != 2 {
		return storage.ECParams{}, fmt.Errorf("invalid EC spec %q, expected format data+parity (e.g. 4+2)", spec)
	}

	dataShards, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return storage.ECParams{}, fmt.Errorf("invalid data shards in %q: %w", spec, err)
	}
	parityShards, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return storage.ECParams{}, fmt.Errorf("invalid parity shards in %q: %w", spec, err)
	}

	params := storage.ECParams{
		DataShards:   dataShards,
		ParityShards: parityShards,
	}

	if err := params.Validate(); err != nil {
		return storage.ECParams{}, err
	}
	return params, nil
}
