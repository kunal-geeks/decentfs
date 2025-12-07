package dht

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/kunal-geeks/decentfs/internal/p2p"
	"github.com/kunal-geeks/decentfs/internal/storage"
)

// MessageType represents the type of a DHT message.
type MessageType string

const (
	MsgPing     MessageType = "PING"
	MsgPong     MessageType = "PONG"
	MsgFindNode MessageType = "FIND_NODE"
	MsgNodes    MessageType = "NODES"

	// New chunk-related message types.
	MsgStoreChunk       MessageType = "STORE_CHUNK"
	MsgStoreChunkResult MessageType = "STORE_CHUNK_RESULT"
	MsgGetChunk         MessageType = "GET_CHUNK"
	MsgChunkData        MessageType = "CHUNK_DATA"

	// NEW: file provider mapping
	MsgStoreFileProvider MessageType = "STORE_FILE_PROVIDER"
	MsgGetFileProviders  MessageType = "GET_FILE_PROVIDERS"
	MsgFileProviders     MessageType = "FILE_PROVIDERS"

	// NEW: file metadata RPC
	MsgGetFileMeta MessageType = "GET_FILE_META"
	MsgFileMeta    MessageType = "FILE_META"
)

// Message is the wire format for DHT messages.
// It will be JSON-encoded into p2p.RPC.Payload.
type Message struct {
	Type      MessageType `json:"type"`
	From      ID          `json:"from"`
	Timestamp int64       `json:"ts"`

	// DHT routing
	Target *ID       `json:"target,omitempty"`
	Nodes  []Contact `json:"nodes,omitempty"`

	// Chunk storage
	ChunkID   string `json:"chunk_id,omitempty"`
	ChunkData []byte `json:"chunk_data,omitempty"`

	// File provider mapping
	FileRoot  string    `json:"file_root,omitempty"`
	Providers []Contact `json:"providers,omitempty"`

	// File metadata
	FileMeta *storage.FileMeta `json:"file_meta,omitempty"`

	// NEW: sender's listening address (e.g. "127.0.0.1:4102")
	// This lets other nodes store a stable address for this ID instead of
	// the ephemeral connection port like 127.0.0.1:52040.
	NodeAddr string `json:"node_addr,omitempty"`

	// Error for responses
	Error string `json:"error,omitempty"`

	// Request/response correlation
	RequestID string `json:"req_id,omitempty"`
}

// Encode encodes the DHT message as JSON bytes.
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage decodes JSON bytes into a DHT Message.
func DecodeMessage(b []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Node represents a DHT node:
// - has a local ID
// - maintains a routing table
// - (later) will talk over p2p.Transport
type Node struct {
	id    ID
	rt    *RoutingTable
	store storage.ChunkStore

	addr string // NEW: node’s advertised listening address

	// NEW: file providers: fileRootHex -> []Contact
	fileProviders map[string][]Contact
	fpMu          sync.RWMutex

	// NEW: local file metadata: fileRootHex -> *storage.FileMeta
	fileMeta map[string]*storage.FileMeta
	fmMu     sync.RWMutex
}

func (n *Node) SetNodeAddr(addr string) {
	n.addr = addr
}

// NewNode creates a DHT Node with the given ID.
func NewNode(id ID) *Node {
	return &Node{
		id:            id,
		rt:            NewRoutingTable(id),
		fileProviders: make(map[string][]Contact),
		fileMeta:      make(map[string]*storage.FileMeta),
	}
}

// ID returns the node's ID.
func (n *Node) ID() ID {
	return n.id
}

// RoutingTable returns the node's routing table (for tests/inspection).
func (n *Node) RoutingTable() *RoutingTable {
	return n.rt
}

// StoreLocalChunk stores the data in the node's local chunk store
// and returns the ChunkID. It returns an error if no store is configured.
func (n *Node) StoreLocalChunk(data []byte) (storage.ChunkID, error) {
	if n.store == nil {
		return storage.ChunkID{}, fmt.Errorf("StoreLocalChunk: no store configured")
	}
	return n.store.PutChunk(data)
}

// GetLocalChunk retrieves a chunk by ID from the node's local store.
func (n *Node) GetLocalChunk(id storage.ChunkID) ([]byte, error) {
	if n.store == nil {
		return nil, fmt.Errorf("GetLocalChunk: no store configured")
	}
	return n.store.GetChunk(id)
}

// HandleRPC takes a p2p.RPC (received over the network), interprets it as a DHT
// Message, updates the routing table, and (if needed) returns a response Message.
//
// This function does NOT send anything over the network directly. It just
// returns a DHT message that upper layers can encode and send back.
func (n *Node) HandleRPC(rpc p2p.RPC) (*Message, error) {
	msg, err := DecodeMessage(rpc.Payload)
	if err != nil {
		return nil, fmt.Errorf("HandleRPC: decode error: %w", err)
	}

	switch msg.Type {
	case MsgPing:
		log.Printf("[dht] (%s) received PING from %s (%s)\n",
			n.id.String(), rpc.From, msg.From.String())

		addr := rpc.From
		if msg.NodeAddr != "" {
			addr = msg.NodeAddr
		}

		if !msg.From.Equals(n.id) {
			n.rt.AddContact(Contact{
				ID:      msg.From,
				Address: addr,
			})
		}

		return &Message{
			Type:      MsgPong,
			From:      n.id,
			NodeAddr:  n.addr, // NEW: include own listening address
			Timestamp: time.Now().Unix(),
		}, nil

	case MsgPong:
		log.Printf("[dht] (%s) received PONG from %s (%s)\n",
			n.id.String(), rpc.From, msg.From.String())

		addr := rpc.From
		if msg.NodeAddr != "" {
			addr = msg.NodeAddr
		}

		if !msg.From.Equals(n.id) {
			n.rt.AddContact(Contact{
				ID:      msg.From,
				Address: addr,
			})
		}
		return nil, nil

	case MsgFindNode:
		if msg.Target == nil {
			return nil, fmt.Errorf("HandleRPC: FIND_NODE missing target")
		}

		log.Printf("[dht] (%s) received FIND_NODE(target=%s) from %s (%s)\n",
			n.id.String(), msg.Target.String(), rpc.From, msg.From.String())

		addr := rpc.From
		if msg.NodeAddr != "" {
			addr = msg.NodeAddr
		}

		if !msg.From.Equals(n.id) {
			n.rt.AddContact(Contact{
				ID:      msg.From,
				Address: addr,
			})
		}

		closest := n.rt.FindClosest(*msg.Target, K)
		log.Printf("[dht] (%s) responding to FIND_NODE with %d nodes\n",
			n.id.String(), len(closest))

		return &Message{
			Type:      MsgNodes,
			From:      n.id,
			NodeAddr:  n.addr, // NEW: include own listening address
			Nodes:     closest,
			Timestamp: time.Now().Unix(),
		}, nil

	case MsgNodes:
		log.Printf("[dht] (%s) received NODES(%d entries) from %s (%s)\n",
			n.id.String(), len(msg.Nodes), rpc.From, msg.From.String())

		for _, c := range msg.Nodes {
			log.Printf("[dht]   node %s at %s\n", c.ID.String(), c.Address)
			if !c.ID.Equals(n.id) {
				n.rt.AddContact(Contact{
					ID:      c.ID,
					Address: c.Address,
				})
			}
		}

		if !msg.From.Equals(n.id) {
			addr := rpc.From
			if msg.NodeAddr != "" {
				addr = msg.NodeAddr
			}
			n.rt.AddContact(Contact{
				ID:      msg.From,
				Address: addr,
			})
		}

		return nil, nil

	// NEW: handle chunk storage messages
	case MsgStoreChunk:
		log.Printf("[dht] (%s) received STORE_CHUNK from %s (%s)\n",
			n.id.String(), rpc.From, msg.From.String())

		if n.store == nil {
			return &Message{
				Type:      MsgStoreChunkResult,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID, // <- echo back
				Error:     "no local chunk store configured",
			}, nil
		}

		if len(msg.ChunkData) == 0 {
			return &Message{
				Type:      MsgStoreChunkResult,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID, // <- echo back
				Error:     "empty chunk data",
			}, nil
		}

		cid, err := n.StoreLocalChunk(msg.ChunkData)
		if err != nil {
			return &Message{
				Type:      MsgStoreChunkResult,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID, // <- echo back
				Error:     fmt.Sprintf("store error: %v", err),
			}, nil
		}

		return &Message{
			Type:      MsgStoreChunkResult,
			From:      n.id,
			NodeAddr:  n.addr,
			Timestamp: time.Now().Unix(),
			RequestID: msg.RequestID, // <- echo back
			ChunkID:   cid.String(),
		}, nil

	case MsgGetChunk:
		log.Printf("[dht] (%s) received GET_CHUNK(%s) from %s (%s)\n",
			n.id.String(), msg.ChunkID, rpc.From, msg.From.String())

		if n.store == nil {
			return &Message{
				Type:      MsgChunkData,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID, // <- echo back
				ChunkID:   msg.ChunkID,
				Error:     "no local chunk store configured",
			}, nil
		}

		if msg.ChunkID == "" {
			return &Message{
				Type:      MsgChunkData,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID, // <- echo back
				Error:     "missing chunk_id",
			}, nil
		}

		cid, err := storage.ChunkIDFromHex(msg.ChunkID)
		if err != nil {
			return &Message{
				Type:      MsgChunkData,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID, // <- echo back
				ChunkID:   msg.ChunkID,
				Error:     fmt.Sprintf("invalid chunk_id: %v", err),
			}, nil
		}

		data, err := n.GetLocalChunk(cid)
		if err != nil {
			return &Message{
				Type:      MsgChunkData,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID, // <- echo back
				ChunkID:   msg.ChunkID,
				Error:     fmt.Sprintf("chunk not found: %v", err),
			}, nil
		}

		return &Message{
			Type:      MsgChunkData,
			From:      n.id,
			NodeAddr:  n.addr,
			Timestamp: time.Now().Unix(),
			RequestID: msg.RequestID, // <- echo back
			ChunkID:   msg.ChunkID,
			ChunkData: data,
		}, nil

	// NEW: handle file provider mapping messages
	case MsgStoreFileProvider:
		log.Printf("[dht] (%s) received STORE_FILE_PROVIDER(root=%s) from %s (%s)\n",
			n.id.String(), msg.FileRoot, rpc.From, msg.From.String())

		if msg.FileRoot == "" {
			// Ignore invalid.
			return nil, nil
		}

		addr := rpc.From
		if msg.NodeAddr != "" {
			addr = msg.NodeAddr
		}

		// Use sender as provider.
		if !msg.From.Equals(n.id) {
			n.addFileProvider(msg.FileRoot, Contact{
				ID:       msg.From,
				Address:  addr,
				LastSeen: time.Now(),
			})
		}
		// Fire-and-forget, no response.
		return nil, nil

	case MsgGetFileProviders:
		log.Printf("[dht] (%s) received GET_FILE_PROVIDERS(root=%s) from %s (%s)\n",
			n.id.String(), msg.FileRoot, rpc.From, msg.From.String())

		if msg.FileRoot == "" {
			return &Message{
				Type:      MsgFileProviders,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID,
				Error:     "missing file_root",
			}, nil
		}

		providers := n.getFileProviders(msg.FileRoot)

		// Optionally, if this node *itself* is a provider, we could also
		// include it here; for now we rely on AnnounceFile to insert self.

		return &Message{
			Type:      MsgFileProviders,
			From:      n.id,
			NodeAddr:  n.addr,
			Timestamp: time.Now().Unix(),
			RequestID: msg.RequestID, // echo back!
			FileRoot:  msg.FileRoot,
			Providers: providers,
		}, nil

	// NEW: handle file metadata messages
	case MsgGetFileMeta:
		log.Printf("[dht] (%s) received GET_FILE_META(root=%s) from %s (%s)\n",
			n.id.String(), msg.FileRoot, rpc.From, msg.From.String())

		if msg.FileRoot == "" {
			return &Message{
				Type:      MsgFileMeta,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID,
				Error:     "missing file_root",
			}, nil
		}

		meta := n.getFileMeta(msg.FileRoot)
		if meta == nil {
			return &Message{
				Type:      MsgFileMeta,
				From:      n.id,
				NodeAddr:  n.addr,
				Timestamp: time.Now().Unix(),
				RequestID: msg.RequestID,
				FileRoot:  msg.FileRoot,
				Error:     "file meta not found",
			}, nil
		}

		return &Message{
			Type:      MsgFileMeta,
			From:      n.id,
			NodeAddr:  n.addr,
			Timestamp: time.Now().Unix(),
			RequestID: msg.RequestID,
			FileRoot:  msg.FileRoot,
			FileMeta:  meta,
		}, nil

	case MsgFileMeta, MsgFileProviders, MsgStoreChunkResult, MsgChunkData:
		// These are responses that should normally be intercepted by Service.readLoop.
		log.Printf("[dht] (%s) received stray response message %s from %s\n",
			n.id.String(), msg.Type, rpc.From)
		return nil, nil

	default:
		log.Printf("[dht] (%s) received unknown message type %q from %s\n",
			n.id.String(), msg.Type, rpc.From)
		return nil, nil
	}
}

func (n *Node) addFileProvider(rootHex string, c Contact) {
	n.fpMu.Lock()
	defer n.fpMu.Unlock()

	list := n.fileProviders[rootHex]
	// avoid duplicates
	for _, existing := range list {
		if existing.ID.Equals(c.ID) && existing.Address == c.Address {
			return
		}
	}
	n.fileProviders[rootHex] = append(list, c)
}

func (n *Node) getFileProviders(rootHex string) []Contact {
	n.fpMu.RLock()
	defer n.fpMu.RUnlock()

	list := n.fileProviders[rootHex]
	out := make([]Contact, len(list))
	copy(out, list)
	return out
}

func (n *Node) addFileMeta(meta *storage.FileMeta) {
	if meta == nil {
		return
	}
	rootHex := meta.Root.String()

	n.fmMu.Lock()
	defer n.fmMu.Unlock()

	n.fileMeta[rootHex] = meta
}

func (n *Node) getFileMeta(rootHex string) *storage.FileMeta {
	n.fmMu.RLock()
	defer n.fmMu.RUnlock()

	return n.fileMeta[rootHex]
}
