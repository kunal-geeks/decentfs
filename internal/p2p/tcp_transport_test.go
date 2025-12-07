package p2p

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test that handleConn receives an encoded RPC and forwards it to rpcCh.
func TestTCPTransport_HandleConnReceivesRPC(t *testing.T) {
	codec := NewLengthPrefixedJSONCodec()

	tr := NewTCPTransport(TCPTransportOpts{
		ListenAddr:    "127.0.0.1:0",
		HandshakeFunc: nil,
		Decoder:       codec,
		Encoder:       codec,
	})

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Start handler on the "server" side.
	go tr.handleConn(serverConn, false)

	original := &RPC{
		Stream:  false,
		Payload: []byte(`"hello"`),
		Meta: map[string]any{
			"test": true,
		},
	}

	// Encode and send from the client side using the codec.
	err := codec.Encode(clientConn, original)
	require.NoError(t, err)

	// Wait for the RPC to appear on the transport's channel.
	select {
	case msg := <-tr.Consume():
		// We don't assert on From (with net.Pipe it's implementation-dependent),
		// but we do check payload and meta.
		assert.Equal(t, original.Stream, msg.Stream, "Stream flag mismatch")
		assert.Equal(t, string(original.Payload), string(msg.Payload), "Payload mismatch")
		assert.Equal(t, original.Meta["test"], msg.Meta["test"], "Meta[test] mismatch")
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for RPC on transport channel")
	}
}

// Test that ListenAndAccept and Dial work together, and that HandshakeFunc and OnPeer are called.
func TestTCPTransport_ListenAndDialWithHandshake(t *testing.T) {
	codec := NewLengthPrefixedJSONCodec()

	handshakeCalled := false
	onPeerCalled := false

	server := NewTCPTransport(TCPTransportOpts{
		// :0 lets the OS pick a free port.
		ListenAddr: "127.0.0.1:0",
		Decoder:    codec,
		Encoder:    codec,
		HandshakeFunc: func(p Peer) error {
			handshakeCalled = true
			return nil
		},
		OnPeer: func(p Peer) error {
			onPeerCalled = true
			return nil
		},
	})

	err := server.ListenAndAccept()
	require.NoError(t, err)
	defer server.Close()

	client := NewTCPTransport(TCPTransportOpts{
		ListenAddr: "127.0.0.1:0",
		Decoder:    codec,
		Encoder:    codec,
	})

	// Dial the server's actual address (updated in ListenAndAccept).
	err = client.Dial(server.Addr())
	require.NoError(t, err)

	// Give some time for handshake and OnPeer to be called.
	time.Sleep(100 * time.Millisecond)

	assert.True(t, handshakeCalled, "HandshakeFunc was not called")
	assert.True(t, onPeerCalled, "OnPeer callback was not called")
}
