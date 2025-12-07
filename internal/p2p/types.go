package p2p

import (
	"net"
)

// Peer represents a remote node in our P2P network.
// We keep it as an interface so we can:
//   - have different implementations (TCP, QUIC, mock for tests)
//   - write higher-level code (DHT, storage, etc.) that doesn't care
//     about the underlying transport details.
type Peer interface {
	// Addr returns the remote peer's network address as a string,
	// e.g. "127.0.0.1:9000".
	Addr() string

	// Send writes raw bytes to this peer.
	// We don't yet care about *what* the bytes mean (could be JSON, protobuf, etc.).
	Send([]byte) error

	// Close closes the connection to this peer.
	Close() error

	// Outbound tells us whether this peer was:
	// - true: we dialed out to them
	// - false: we accepted their incoming connection
	Outbound() bool
}

// Transport defines the behavior any network transport must implement
// (TCP, TLS over TCP, QUIC, etc.).
// Higher-level components (like the DHT) will interact with this interface,
// not with concrete TCP types.
type Transport interface {

	// Addr returns the local address this transport is bound to, e.g. "127.0.0.1:9000".
	Addr() string

	// ListenAndAccept starts listening on Addr() and accepting connections.
	// It should return quickly, with the accept loop running in a goroutine.
	ListenAndAccept() error

	// Dial connects to a remote address and sets up a Peer
	// (including any handshakes). It should not block forever.
	Dial(addr string) error

	// Consume returns a receive-only channel of RPC messages.
	// This is how the upper layers "listen" for messages from peers.
	Consume() <-chan RPC

	// Send sends an RPC message to the given address.
	Send(addr string, rpc RPC) error

	// Close shuts down the transport (e.g. closes the listener).
	Close() error
}

// RPC represents a single logical message received from or sent to a peer.
// Think of it like "one envelope" over the wire.
type RPC struct {
	// From is filled in by the Transport to indicate the sender's address.
	From string

	// Stream marks "control" messages indicating the start of a streaming
	// operation (like large file transfer). When Stream == true, the transport
	// may temporarily pause its normal message loop.
	Stream bool

	// Payload is the raw message body. The encoding/decoding layer
	// (JSON, protobuf, etc.) decides how to structure this.
	Payload []byte

	// Meta is optional key-value metadata for this RPC.
	// Using map[string]any allows flexibility (like headers).
	Meta map[string]any
}

// Decoder reads from a net.Conn and decodes a single RPC message from it.
// It is responsible for:
// - framing (knowing where a message starts/ends)
// - deserialization (e.g. JSON decode into RPC struct)
type Decoder interface {
	Decode(conn net.Conn, rpc *RPC) error
}

// Encoder is the dual of Decoder: it takes an RPC and writes it to a net.Conn
// using the same framing and serialization rules.
type Encoder interface {
	Encode(conn net.Conn, rpc *RPC) error
}

// HandshakeFunc is a user-provided function that runs right after a connection
// is established, but *before* we start reading normal RPC messages.
//
// Typical uses:
// - Exchange node IDs
// - TLS mutual verification (checking certs)
// - Sending/receiving JWTs or auth tokens
// - Version negotiation (protocol version, capabilities, etc.)
type HandshakeFunc func(Peer) error
