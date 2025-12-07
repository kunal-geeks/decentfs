package dht

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kunal-geeks/decentfs/internal/p2p"
	"github.com/kunal-geeks/decentfs/internal/storage"
)

const DefaultReplicaCount = 3

// Service ties together a DHT Node and a p2p.Transport.
// It continuously reads RPCs from the transport and lets the Node handle them.
type Service struct {
	node      *Node
	transport p2p.Transport
	store     storage.ChunkStore

	mu      sync.Mutex
	pending map[string]chan *Message
}

// ServiceOpts configures a DHT Service.
type ServiceOpts struct {
	ID        ID
	Transport p2p.Transport
	Store     storage.ChunkStore // optional local chunk store
}

// NewService creates a new DHT Service and starts its read loop in a goroutine.
// The transport is assumed to already be configured (codec, handshake, etc.).
func NewService(opts ServiceOpts) *Service {
	n := NewNode(opts.ID)
	n.store = opts.Store

	// NEW: tell the node its own listening address
	// so it can populate Message.NodeAddr in outgoing messages.
	n.SetNodeAddr(opts.Transport.Addr())

	s := &Service{
		node:      n,
		transport: opts.Transport,
		store:     opts.Store,
		pending:   make(map[string]chan *Message),
	}

	go s.readLoop()
	return s
}

func (s *Service) registerPending(reqID string) chan *Message {
	ch := make(chan *Message, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending[reqID] = ch
	return ch
}

func (s *Service) takePending(reqID string) chan *Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.pending[reqID]
	if ok {
		delete(s.pending, reqID)
	}
	return ch
}

func isResponseType(t MessageType) bool {
	switch t {
	case MsgStoreChunkResult, MsgChunkData, MsgFileProviders, MsgFileMeta:
		return true
	default:
		return false
	}
}

// Node returns the underlying DHT Node (for tests or advanced use).
func (s *Service) Node() *Node {
	return s.node
}

// readLoop consumes RPCs from the transport channel and passes them to the Node.
// If the Node returns a response Message, we encode it and send it back to rpc.From.
func (s *Service) readLoop() {
	for rpc := range s.transport.Consume() {
		// First decode the message so we can inspect it.
		msg, err := DecodeMessage(rpc.Payload)
		if err != nil {
			log.Printf("dht: decode error from %s: %v\n", rpc.From, err)
			continue
		}

		// If this is a response with a RequestID, try to deliver it
		// to a waiting goroutine instead of passing it to Node.
		if msg.RequestID != "" && isResponseType(msg.Type) {
			if ch := s.takePending(msg.RequestID); ch != nil {
				// Deliver response and skip Node.HandleRPC.
				ch <- msg
				close(ch)
				continue
			}
		}

		// For everything else, delegate to the Node.
		resp, err := s.node.HandleRPC(p2p.RPC{
			From:    rpc.From,
			Stream:  rpc.Stream,
			Payload: rpc.Payload,
			Meta:    rpc.Meta,
		})
		if err != nil {
			log.Printf("dht: HandleRPC error from %s: %v\n", rpc.From, err)
			continue
		}
		if resp == nil {
			continue
		}

		payload, err := resp.Encode()
		if err != nil {
			log.Printf("dht: encode response error to %s: %v\n", rpc.From, err)
			continue
		}

		log.Printf("[dht] sending %s to %s\n", resp.Type, rpc.From)

		out := p2p.RPC{
			Stream:  false,
			Payload: payload,
		}

		if err := s.transport.Send(rpc.From, out); err != nil {
			log.Printf("dht: send response error to %s: %v\n", rpc.From, err)
		}
	}
}

// StoreChunkRemote sends a STORE_CHUNK request to the given peer address,
// waits for a STORE_CHUNK_RESULT, and returns the ChunkID on success.
func (s *Service) StoreChunkRemote(addr string, data []byte) (storage.ChunkID, error) {
	reqID := MustRandomID().String() // reuse your Kademlia ID generator

	msg := &Message{
		Type:      MsgStoreChunk,
		From:      s.node.id,
		Timestamp: time.Now().Unix(),
		RequestID: reqID,
		ChunkData: data,
	}

	payload, err := msg.Encode()
	if err != nil {
		return storage.ChunkID{}, fmt.Errorf("StoreChunkRemote: encode: %w", err)
	}

	respCh := s.registerPending(reqID)

	rpc := p2p.RPC{
		Stream:  false,
		Payload: payload,
	}

	if err := s.transport.Send(addr, rpc); err != nil {
		// Clean up pending entry.
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return storage.ChunkID{}, fmt.Errorf("StoreChunkRemote: send: %w", err)
	}

	// Wait for response with timeout.
	select {
	case resp := <-respCh:
		if resp == nil {
			return storage.ChunkID{}, fmt.Errorf("StoreChunkRemote: nil response")
		}
		if resp.Error != "" {
			return storage.ChunkID{}, fmt.Errorf("StoreChunkRemote: remote error: %s", resp.Error)
		}
		if resp.ChunkID == "" {
			return storage.ChunkID{}, fmt.Errorf("StoreChunkRemote: missing ChunkID in response")
		}

		cid, err := storage.ChunkIDFromHex(resp.ChunkID)
		if err != nil {
			return storage.ChunkID{}, fmt.Errorf("StoreChunkRemote: invalid ChunkID: %w", err)
		}
		return cid, nil

	case <-time.After(3 * time.Second):
		// Timeout: clean pending.
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return storage.ChunkID{}, fmt.Errorf("StoreChunkRemote: timeout waiting for response")
	}
}

// FetchChunkRemote sends a GET_CHUNK request to the given peer address,
// waits for a CHUNK_DATA response, and returns the data.
func (s *Service) FetchChunkRemote(addr string, id storage.ChunkID) ([]byte, error) {
	reqID := MustRandomID().String()

	msg := &Message{
		Type:      MsgGetChunk,
		From:      s.node.id,
		Timestamp: time.Now().Unix(),
		RequestID: reqID,
		ChunkID:   id.String(),
	}

	payload, err := msg.Encode()
	if err != nil {
		return nil, fmt.Errorf("FetchChunkRemote: encode: %w", err)
	}

	respCh := s.registerPending(reqID)

	rpc := p2p.RPC{
		Stream:  false,
		Payload: payload,
	}

	if err := s.transport.Send(addr, rpc); err != nil {
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return nil, fmt.Errorf("FetchChunkRemote: send: %w", err)
	}

	select {
	case resp := <-respCh:
		if resp == nil {
			return nil, fmt.Errorf("FetchChunkRemote: nil response")
		}
		if resp.Error != "" {
			return nil, fmt.Errorf("FetchChunkRemote: remote error: %s", resp.Error)
		}
		if resp.ChunkID == "" {
			return nil, fmt.Errorf("FetchChunkRemote: missing ChunkID in response")
		}
		if resp.ChunkData == nil {
			return nil, fmt.Errorf("FetchChunkRemote: missing ChunkData in response")
		}
		// Optional sanity check: ensure ID matches.
		if resp.ChunkID != id.String() {
			log.Printf("[dht] FetchChunkRemote: warning: response ChunkID %s != requested %s\n",
				resp.ChunkID, id.String())
		}
		return resp.ChunkData, nil

	case <-time.After(3 * time.Second):
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return nil, fmt.Errorf("FetchChunkRemote: timeout waiting for response")
	}
}

// idFromBytes maps arbitrary bytes into the DHT keyspace by hashing
// them and truncating to the ID length.
func idFromBytes(b []byte) ID {
	sum := sha256.Sum256(b)
	var key ID
	copy(key[:], sum[:len(key)])
	return key
}

// fileKeyFromRoot maps a file root (Merkle root, 32 bytes) into the DHT keyspace.
func fileKeyFromRoot(root storage.ChunkID) ID {
	return idFromBytes(root.Bytes())
}

// chunkKeyFromID maps a chunk ID into the DHT keyspace.
func chunkKeyFromID(id storage.ChunkID) ID {
	return idFromBytes(id.Bytes())
}

// AnnounceFile tells the DHT that THIS node provides the file with the
// given root. It:
//  1. records a local provider record for this root
//  2. looks up the DHT key for the root
//  3. sends STORE_FILE_PROVIDER to closest nodes
func (s *Service) AnnounceFile(root storage.ChunkID) error {
	rootHex := root.String()

	// Mark self as a provider locally.
	s.node.addFileProvider(rootHex, Contact{
		ID:       s.node.id,
		Address:  s.transport.Addr(),
		LastSeen: time.Now(),
	})

	// Find closest nodes in the DHT for this file's key.
	key := fileKeyFromRoot(root)

	closest, err := s.Lookup(key)
	if err != nil {
		return fmt.Errorf("AnnounceFile: Lookup: %w", err)
	}
	if len(closest) == 0 {
		log.Printf("[dht] AnnounceFile: no peers found to store provider record for %s\n", rootHex)
		return nil
	}

	// Build STORE_FILE_PROVIDER message (fire-and-forget).
	msg := &Message{
		Type:      MsgStoreFileProvider,
		From:      s.node.id,
		Timestamp: time.Now().Unix(),
		FileRoot:  rootHex,
	}

	payload, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("AnnounceFile: encode: %w", err)
	}

	rpc := p2p.RPC{
		Stream:  false,
		Payload: payload,
	}

	// Send to K closest nodes (or all we found).
	for _, c := range closest {
		log.Printf("[dht] AnnounceFile: sending STORE_FILE_PROVIDER(root=%s) to %s\n",
			rootHex, c.Address)
		if err := s.transport.Send(c.Address, rpc); err != nil {
			log.Printf("[dht] AnnounceFile: send to %s error: %v\n", c.Address, err)
		}
	}

	return nil
}

// requestFileProvidersFrom sends GET_FILE_PROVIDERS(root) to a single peer
// and waits for FILE_PROVIDERS response.
func (s *Service) requestFileProvidersFrom(addr string, root storage.ChunkID) ([]Contact, error) {
	reqID := MustRandomID().String()

	msg := &Message{
		Type:      MsgGetFileProviders,
		From:      s.node.id,
		Timestamp: time.Now().Unix(),
		RequestID: reqID,
		FileRoot:  root.String(),
	}

	payload, err := msg.Encode()
	if err != nil {
		return nil, fmt.Errorf("requestFileProvidersFrom: encode: %w", err)
	}

	respCh := s.registerPending(reqID)

	rpc := p2p.RPC{
		Stream:  false,
		Payload: payload,
	}

	if err := s.transport.Send(addr, rpc); err != nil {
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return nil, fmt.Errorf("requestFileProvidersFrom: send: %w", err)
	}

	select {
	case resp := <-respCh:
		if resp == nil {
			return nil, fmt.Errorf("requestFileProvidersFrom: nil response")
		}
		if resp.Error != "" {
			return nil, fmt.Errorf("requestFileProvidersFrom: remote error: %s", resp.Error)
		}
		return resp.Providers, nil

	case <-time.After(3 * time.Second):
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return nil, fmt.Errorf("requestFileProvidersFrom: timeout waiting for response")
	}
}

// FindFileProviders looks up nodes that provide the file with the given root.
// It uses the DHT to find nodes close to the file key, then queries a few of
// them for FILE_PROVIDERS.
func (s *Service) FindFileProviders(root storage.ChunkID) ([]Contact, error) {
	key := fileKeyFromRoot(root)

	shortlist, err := s.Lookup(key)
	if err != nil {
		return nil, fmt.Errorf("FindFileProviders: Lookup: %w", err)
	}
	if len(shortlist) == 0 {
		return nil, nil
	}

	// Query up to Alpha closest nodes.
	n := len(shortlist)
	if n > Alpha {
		n = Alpha
	}

	var wg sync.WaitGroup
	providersCh := make(chan []Contact, n)

	for i := 0; i < n; i++ {
		c := shortlist[i]
		wg.Add(1)
		go func(c Contact) {
			defer wg.Done()
			ps, err := s.requestFileProvidersFrom(c.Address, root)
			if err != nil {
				log.Printf("[dht] FindFileProviders: error from %s: %v\n", c.Address, err)
				return
			}
			if len(ps) > 0 {
				providersCh <- ps
			}
		}(c)
	}

	wg.Wait()
	close(providersCh)

	// Merge and deduplicate providers.
	seen := make(map[string]Contact)
	for group := range providersCh {
		for _, c := range group {
			key := c.ID.String() + "@" + c.Address
			if _, ok := seen[key]; !ok {
				seen[key] = c
			}
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	result := make([]Contact, 0, len(seen))
	for _, c := range seen {
		result = append(result, c)
	}

	return result, nil
}

// StoreFileLocalAndAnnounce reads the file at path, stores its chunks
// in the local ChunkStore, builds FileMeta with a Merkle root, registers
// the FileMeta locally on this node, and announces the file root via DHT.
func (s *Service) StoreFileLocalAndAnnounce(path string, chunkSize int) (*storage.FileMeta, error) {
	if s.store == nil {
		return nil, fmt.Errorf("StoreFileLocalAndAnnounce: no local ChunkStore configured")
	}

	meta, err := storage.StoreFileWithMerkle(s.store, path, chunkSize)
	if err != nil {
		return nil, fmt.Errorf("StoreFileLocalAndAnnounce: %w", err)
	}

	// Register in local metadata index so we can answer GET_FILE_META.
	s.node.addFileMeta(meta)

	// Replicate chunks to other peers (best-effort).
	if err := s.ReplicateFileChunks(meta, DefaultReplicaCount); err != nil {
		log.Printf("[dht] StoreFileLocalAndAnnounce: replication error: %v\n", err)
	}

	// Announce file root via DHT.
	if err := s.AnnounceFile(meta.Root); err != nil {
		log.Printf("[dht] StoreFileLocalAndAnnounce: AnnounceFile error: %v\n", err)
	}

	log.Printf("[dht] stored file %s with root %s (%d chunks, %d bytes)\n",
		path, meta.Root.String(), len(meta.Chunks), meta.Size)

	return meta, nil
}

// FetchFileMetaFromProvider asks a specific peer for the FileMeta of the given root.
func (s *Service) FetchFileMetaFromProvider(addr string, root storage.ChunkID) (*storage.FileMeta, error) {
	reqID := MustRandomID().String()

	msg := &Message{
		Type:      MsgGetFileMeta,
		From:      s.node.id,
		Timestamp: time.Now().Unix(),
		RequestID: reqID,
		FileRoot:  root.String(),
	}

	payload, err := msg.Encode()
	if err != nil {
		return nil, fmt.Errorf("FetchFileMetaFromProvider: encode: %w", err)
	}

	respCh := s.registerPending(reqID)

	rpc := p2p.RPC{
		Stream:  false,
		Payload: payload,
	}

	if err := s.transport.Send(addr, rpc); err != nil {
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return nil, fmt.Errorf("FetchFileMetaFromProvider: send: %w", err)
	}

	select {
	case resp := <-respCh:
		if resp == nil {
			return nil, fmt.Errorf("FetchFileMetaFromProvider: nil response")
		}
		if resp.Error != "" {
			return nil, fmt.Errorf("FetchFileMetaFromProvider: remote error: %s", resp.Error)
		}
		if resp.FileMeta == nil {
			return nil, fmt.Errorf("FetchFileMetaFromProvider: missing FileMeta in response")
		}
		// Optional: sanity check root matches.
		if resp.FileRoot != "" && resp.FileRoot != root.String() {
			log.Printf("[dht] FetchFileMetaFromProvider: warning: response root %s != requested %s\n",
				resp.FileRoot, root.String())
		}
		return resp.FileMeta, nil

	case <-time.After(5 * time.Second):
		if ch := s.takePending(reqID); ch != nil {
			close(ch)
		}
		return nil, fmt.Errorf("FetchFileMetaFromProvider: timeout waiting for response")
	}
}

// DownloadFileFromProvider downloads a file (identified by root) from a specific
// provider address and writes it to destPath. It fetches FileMeta first, then
// streams all chunks in order.
func (s *Service) DownloadFileFromProvider(root storage.ChunkID, providerAddr, destPath string) error {
	meta, err := s.FetchFileMetaFromProvider(providerAddr, root)
	if err != nil {
		return fmt.Errorf("DownloadFileFromProvider: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("DownloadFileFromProvider: create dest: %w", err)
	}
	defer f.Close()

	var written int64

	for _, cm := range meta.Chunks {
		data, err := s.FetchChunkRemote(providerAddr, cm.ID)
		if err != nil {
			return fmt.Errorf("DownloadFileFromProvider: FetchChunkRemote(chunk %d) error: %w",
				cm.Index, err)
		}

		// Optional: verify hash matches ID.
		checkID := storage.NewChunkID(data)
		if !checkID.Equal(cm.ID) {
			return fmt.Errorf("DownloadFileFromProvider: chunk %d hash mismatch", cm.Index)
		}

		n, err := f.Write(data)
		if err != nil {
			return fmt.Errorf("DownloadFileFromProvider: write chunk %d: %w", cm.Index, err)
		}
		written += int64(n)
	}

	if written != meta.Size {
		log.Printf("[dht] DownloadFileFromProvider: warning: written %d bytes, meta.Size=%d\n",
			written, meta.Size)
	}

	log.Printf("[dht] downloaded file root=%s from %s to %s (%d bytes)\n",
		root.String(), providerAddr, destPath, written)

	return nil
}

// DownloadFile finds providers for the given file root via the DHT, picks one,
// and downloads the file to destPath.
func (s *Service) DownloadFile(root storage.ChunkID, destPath string) error {
	providers, err := s.FindFileProviders(root)
	if err != nil {
		return fmt.Errorf("DownloadFile: FindFileProviders: %w", err)
	}
	if len(providers) == 0 {
		return fmt.Errorf("DownloadFile: no providers found for root %s", root.String())
	}

	// For now, just pick the first provider.
	p := providers[0]
	log.Printf("[dht] DownloadFile: using provider %s at %s for root %s\n",
		p.ID.String(), p.Address, root.String())

	return s.DownloadFileFromProvider(root, p.Address, destPath)
}

// ReplicateFileChunks ensures that each chunk in the given FileMeta is
// replicated to up to replicaCount peers (in addition to the local copy).
// It uses the DHT to find peers close to each chunk's key and sends
// STORE_CHUNK to those peers.
func (s *Service) ReplicateFileChunks(meta *storage.FileMeta, replicaCount int) error {
	if meta == nil {
		return fmt.Errorf("ReplicateFileChunks: meta is nil")
	}
	if s.store == nil {
		return fmt.Errorf("ReplicateFileChunks: no local ChunkStore configured")
	}
	if replicaCount <= 1 {
		// 1 means "only local copy" is fine.
		return nil
	}

	for _, cm := range meta.Chunks {
		chunkID := cm.ID

		// Load chunk bytes from local store.
		data, err := s.store.GetChunk(chunkID)
		if err != nil {
			log.Printf("[dht] ReplicateFileChunks: cannot load chunk %s: %v\n",
				chunkID.String(), err)
			continue
		}

		// Find peers close to the chunk key.
		key := chunkKeyFromID(chunkID)

		closest, err := s.Lookup(key)
		if err != nil {
			log.Printf("[dht] ReplicateFileChunks: Lookup for chunk %s error: %v\n",
				chunkID.String(), err)
			continue
		}
		if len(closest) == 0 {
			log.Printf("[dht] ReplicateFileChunks: no peers found for chunk %s\n",
				chunkID.String())
			continue
		}

		// We already have 1 local copy; we need up to replicaCount-1 remote replicas.
		needed := replicaCount - 1
		replicated := 0

		for _, c := range closest {
			// Skip self (same ID or same address).
			if c.ID.Equals(s.node.id) || c.Address == s.transport.Addr() {
				continue
			}

			log.Printf("[dht] ReplicateFileChunks: sending chunk %s to %s\n",
				chunkID.String(), c.Address)

			// StoreChunkRemote sends STORE_CHUNK and waits for STORE_CHUNK_RESULT.
			remoteID, err := s.StoreChunkRemote(c.Address, data)
			if err != nil {
				log.Printf("[dht] ReplicateFileChunks: StoreChunkRemote to %s failed: %v\n",
					c.Address, err)
				continue
			}

			// Optional: sanity check remote ChunkID matches local.
			if !remoteID.Equal(chunkID) {
				log.Printf("[dht] ReplicateFileChunks: warning: remote ChunkID %s != local %s\n",
					remoteID.String(), chunkID.String())
			}

			replicated++
			if replicated >= needed {
				break
			}
		}

		log.Printf("[dht] ReplicateFileChunks: chunk %s replicated to %d remote peers\n",
			chunkID.String(), replicated)
	}

	return nil
}

// StoreFileWithEC reads the file at path, splits it into chunks of at most
// chunkSize bytes, encodes each chunk with Reed–Solomon erasure coding using
// the provided ECParams, and stores all shards in the local ChunkStore.
//
// It returns an ECFileMeta describing how to reconstruct the file from this node.
//
// NOTE: This step does NOT announce anything to the DHT yet. It is purely
// local storage. In later steps we will add DHT-level announcements and
// cross-node shard placement.
func (s *Service) StoreFileWithEC(path string, chunkSize int, params storage.ECParams) (*storage.ECFileMeta, error) {
	if s.store == nil {
		return nil, fmt.Errorf("StoreFileWithEC: no ChunkStore configured on Service")
	}
	if chunkSize <= 0 {
		return nil, fmt.Errorf("StoreFileWithEC: invalid chunkSize %d", chunkSize)
	}
	if err := params.Validate(); err != nil {
		return nil, fmt.Errorf("StoreFileWithEC: invalid EC params: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("StoreFileWithEC: open %s: %w", path, err)
	}
	defer f.Close()

	log.Printf("[dht] StoreFileWithEC: storing %s with chunkSize=%d, data=%d parity=%d\n",
		path, chunkSize, params.DataShards, params.ParityShards)

	meta, err := storage.BuildECFileMetaFromReader(s.store, filepath.Base(path), f, chunkSize, params)
	if err != nil {
		return nil, fmt.Errorf("StoreFileWithEC: build EC file meta: %w", err)
	}

	log.Printf("[dht] StoreFileWithEC: stored %s as %d chunks, total size=%d bytes\n",
		path, len(meta.Chunks), meta.Size)

	return meta, nil
}
