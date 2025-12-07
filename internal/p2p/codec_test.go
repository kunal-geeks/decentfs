package p2p

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to create an in-memory connection pair.
func newPipeConn() (net.Conn, net.Conn) {
	return net.Pipe()
}

func TestLengthPrefixedJSONCodec_EncodeDecode(t *testing.T) {
	c := NewLengthPrefixedJSONCodec()

	serverConn, clientConn := newPipeConn()
	defer serverConn.Close()
	defer clientConn.Close()

	original := &RPC{
		From:    "127.0.0.1:9000",
		Stream:  false,
		Payload: []byte(`{"op":"ping"}`),
		Meta: map[string]any{
			"type": "test",
			"id":   123,
		},
	}

	// Encode on "client" side.
	go func() {
		err := c.Encode(clientConn, original)
		require.NoError(t, err, "encode should not fail")
	}()

	// Decode on "server" side.
	var decoded RPC
	err := c.Decode(serverConn, &decoded)
	require.NoError(t, err, "decode should not fail")

	// Basic field equality with assert (non-fatal, we want to see all mismatches).
	assert.Equal(t, original.From, decoded.From, "From mismatch")
	assert.Equal(t, original.Stream, decoded.Stream, "Stream mismatch")
	assert.Equal(t, string(original.Payload), string(decoded.Payload), "Payload mismatch")

	// Meta[type] should match as string.
	assert.Equal(t, original.Meta["type"], decoded.Meta["type"], "Meta[type] mismatch")

	// Meta[id] comes back as float64 due to JSON number decoding.
	decodedID, ok := decoded.Meta["id"].(float64)
	require.True(t, ok, "Meta[id] should be float64 after JSON decode")

	origID, ok := original.Meta["id"].(int)
	require.True(t, ok, "original Meta[id] should be int")

	assert.Equal(t, origID, int(decodedID), "Meta[id] numeric value mismatch")
}

// Test that messages exceeding MaxMessageSize cause an error.
func TestLengthPrefixedJSONCodec_TooLargeMessage(t *testing.T) {
	c := NewLengthPrefixedJSONCodec()

	serverConn, clientConn := newPipeConn()
	defer serverConn.Close()
	defer clientConn.Close()

	// Create a payload larger than MaxMessageSize.
	tooBigPayload := make([]byte, MaxMessageSize+1)

	rpc := &RPC{
		Payload: tooBigPayload,
	}

	err := c.Encode(clientConn, rpc)
	assert.Error(t, err, "encode should fail for too large message")
}
