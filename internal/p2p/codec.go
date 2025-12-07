package p2p

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

// MaxMessageSize defines an upper bound on a single RPC message size.
// This protects us from a peer sending a gigantic length and causing
// our node to allocate huge memory.
const MaxMessageSize = 4 << 20 // 4 MiB

// LengthPrefixedJSONCodec implements Encoder and Decoder.
// It encodes RPC as JSON, and frames each message as:
//
//	[4-byte big-endian length][JSON bytes...]
//
// The Decoder reverses this process.
type LengthPrefixedJSONCodec struct{}

// NewLengthPrefixedJSONCodec is a ctor helper.
// Right now the codec has no state, but this keeps the API flexible.
func NewLengthPrefixedJSONCodec() *LengthPrefixedJSONCodec {
	return &LengthPrefixedJSONCodec{}
}

// Encode implements the Encoder interface.
//
// Steps:
// 1. Marshal the RPC struct into JSON bytes.
// 2. Compute the length of JSON bytes.
// 3. Write the length as a 4-byte big-endian uint32.
// 4. Write the JSON bytes.
func (c *LengthPrefixedJSONCodec) Encode(conn net.Conn, rpc *RPC) error {
	// Convert RPC to JSON bytes.
	data, err := json.Marshal(rpc)
	if err != nil {
		return fmt.Errorf("encode: json marshal error: %w", err)
	}

	if len(data) > MaxMessageSize {
		return fmt.Errorf("encode: message too large (%d > %d)", len(data), MaxMessageSize)
	}

	// 4 bytes for length prefix
	var lengthPrefix [4]byte
	binary.BigEndian.PutUint32(lengthPrefix[:], uint32(len(data)))

	// Write the length, then the JSON payload.
	if _, err := conn.Write(lengthPrefix[:]); err != nil {
		return fmt.Errorf("encode: write length error: %w", err)
	}

	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("encode: write payload error: %w", err)
	}

	return nil
}

// Decode implements the Decoder interface.
//
// Steps:
// 1. Read exactly 4 bytes for length prefix.
// 2. Parse length as big-endian uint32.
// 3. Validate length against MaxMessageSize.
// 4. Allocate a buffer of that size and read exactly that many bytes.
// 5. Unmarshal JSON into the provided *RPC.
func (c *LengthPrefixedJSONCodec) Decode(conn net.Conn, rpc *RPC) error {
	// Read exactly 4 bytes of length prefix.
	var lengthPrefix [4]byte
	if _, err := io.ReadFull(conn, lengthPrefix[:]); err != nil {
		return fmt.Errorf("decode: read length error: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthPrefix[:])
	if length == 0 {
		return fmt.Errorf("decode: zero-length message")
	}
	if length > uint32(MaxMessageSize) {
		return fmt.Errorf("decode: message too large (%d > %d)", length, MaxMessageSize)
	}

	buf := make([]byte, length)

	// Read exactly "length" bytes.
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("decode: read payload error: %w", err)
	}

	// Reset rpc to a zero value, then unmarshal into it.
	*rpc = RPC{}

	if err := json.Unmarshal(buf, rpc); err != nil {
		return fmt.Errorf("decode: json unmarshal error: %w", err)
	}

	return nil
}
