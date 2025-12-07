# DecentFS

A decentralized file storage system built on Kademlia DHT with support for Merkle trees and Reed-Solomon erasure coding.

## Overview

DecentFS is a peer-to-peer distributed file storage network where nodes communicate via a Kademlia-based DHT for peer discovery and file chunk distribution. Files are split into chunks, encoded with optional erasure coding, and stored across multiple peers for redundancy and fault tolerance.

**Key Features:**
- **Kademlia DHT** for scalable peer discovery and routing
- **Merkle Tree Verification** for data integrity
- **Reed-Solomon Erasure Coding** for fault-tolerant storage (configurable k+m splits)
- **P2P Transport** using length-prefixed JSON over TCP
- **Local Chunk Storage** with filesystem-based persistence
- **Bootstrap Protocol** to join existing networks

## Installation

### Prerequisites
- Go 1.21 or later

### Build
```bash
git clone https://github.com/kunal-geeks/decentfs.git
cd decentfs
go build -o decentfs ./cmd/main.go
```

## Quick Start

### Start a Bootstrap Node
```bash
./decentfs -listen 127.0.0.1:4000
```

### Start Additional Nodes (Join Network)
```bash
./decentfs -listen 127.0.0.1:4001 -bootstrap 127.0.0.1:4000
./decentfs -listen 127.0.0.1:4002 -bootstrap 127.0.0.1:4000
```

## Usage Examples

### Peer Lookup via DHT
```bash
./decentfs -listen 127.0.0.1:4001 \
    -bootstrap 127.0.0.1:4000 \
    -lookup <peer_id_hex>
```

### Store File with Merkle Tree
```bash
./decentfs -listen 127.0.0.1:4000 \
    -bootstrap 127.0.0.1:4001 \
    -store-file /path/to/file.txt \
    -chunk-size 262144
```

### Store File with Erasure Coding (4+2)
```bash
./decentfs -listen 127.0.0.1:4000 \
    -store-file /path/to/file.txt \
    -chunk-size 262144 \
    -ec 4+2
```

### Download File via DHT
```bash
./decentfs -listen 127.0.0.1:4001 \
    -bootstrap 127.0.0.1:4000 \
    -download-root <root_chunk_id_hex> \
    -out /path/to/output.txt
```

### Direct Peer Chunk Operations
```bash
# Store a chunk on a specific peer
./decentfs -listen 127.0.0.1:4000 \
    -store-to 127.0.0.1:4001 \
    -store-data "hello world"

# Fetch a chunk from a specific peer
./decentfs -listen 127.0.0.1:4000 \
    -fetch-from 127.0.0.1:4001 \
    -fetch-chunk <chunk_id_hex>
```

## Architecture

### Core Components

**DHT (Kademlia)**
- Distributed hash table for peer discovery and routing
- Bucket-based contact management with LRU eviction
- Configurable K-parameter for replication factor
- FindNode and FindValue RPC operations

**P2P Transport**
- Length-prefixed JSON protocol over TCP
- RPC-based message handling
- Ping, FindNode, Store, and Retrieve operations

**Storage Layer**
- Filesystem-based chunk persistence
- Configurable chunk size (default: 256KB)
- Deterministic chunk naming via SHA-256 hashing

**File Encoding**
- **Merkle Trees**: Cryptographic verification of file integrity
- **Reed-Solomon Erasure Coding**: k+m data redundancy (e.g., 4+2 means 4 data + 2 parity chunks)

**Bootstrap Protocol**
- Join existing network via known bootstrap peers
- Automatic routing table population
- K-bucket management and contact refresh

### Message Flow
```
Client Request
    ↓
Local Node
    ↓
DHT Lookup (FindNode/FindValue)
    ↓
Peer Discovery & Contact
    ↓
RPC to Target Peer
    ↓
Response & Chunk Storage/Retrieval
```

## Configuration

### Command-line Flags
- `-listen <addr>` - Local listening address (default: `127.0.0.1:8000`)
- `-bootstrap <addr>` - Bootstrap peer address (optional)
- `-store-file <path>` - File to store in the network
- `-chunk-size <bytes>` - Chunk size for file splitting (default: `262144`)
- `-ec <k+m>` - Erasure coding ratio (e.g., `4+2`)
- `-download-root <id>` - Root chunk ID to download
- `-out <path>` - Output file path for downloads
- `-store-to <addr>` - Target peer for direct chunk storage
- `-store-data <data>` - Chunk data to store
- `-fetch-from <addr>` - Target peer for chunk retrieval
- `-fetch-chunk <id>` - Chunk ID to fetch
- `-lookup <id>` - Peer ID to lookup in DHT

## Project Structure

```
decentfs/
├── cmd/
│   └── main.go              # Entry point
├── internal/
│   ├── dht/                 # Kademlia DHT implementation
│   │   ├── id.go            # ID representation and operations
│   │   ├── routing_table.go # Bucket and contact management
│   │   ├── message.go       # DHT message types
│   │   ├── node.go          # DHT node logic
│   │   └── lookup.go        # Peer/key lookup operations
│   ├── p2p/                 # P2P transport layer
│   │   ├── transport.go     # TCP communication
│   │   └── rpc.go           # RPC handling
│   ├── storage/             # Chunk storage
│   │   └── store.go         # Filesystem persistence
│   └── codec/               # File encoding
│       ├── merkle.go        # Merkle tree implementation
│       └── erasure.go       # Reed-Solomon coding
├── go.mod
├── go.sum
└── README.md
```

## Testing

Run the test suite:
```bash
go test ./...
```

Run with coverage:
```bash
go test -cover ./...
```

## Protocol Details

### Kademlia RPC Types
- **PING** - Check if peer is alive
- **FIND_NODE** - Discover K closest nodes to target ID
- **FIND_VALUE** - Lookup key/chunk in DHT
- **STORE** - Store a chunk on this node
- **NODES** - Response with list of closest nodes

### Chunk Metadata
Each stored chunk includes:
- Chunk ID (SHA-256 hash)
- Data payload
- Optional Merkle proof
- Replication hints for erasure coding

## Performance Considerations

- **Lookup Latency**: O(log N) where N is network size
- **Storage Overhead**: With 4+2 erasure coding, 50% overhead vs. 3x replication
- **Chunk Size**: Larger chunks reduce DHT lookups but increase transfer time
- **K-value**: Higher K improves fault tolerance at the cost of more routing state

## Future Enhancements

- [ ] Byzantine fault tolerance
- [ ] Caching layer for frequently accessed chunks
- [ ] Rate limiting and spam protection
- [ ] Web dashboard for network visualization
- [ ] IPFS-compatible API layer
- [ ] Persistent peer discovery (DHT snapshots)

## License

MIT

## Contributing

Contributions are welcome! Please open issues and submit pull requests.
