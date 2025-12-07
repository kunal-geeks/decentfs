package dht

import (
	"testing"
	"time"

	"github.com/kunal-geeks/decentfs/internal/p2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTransport is a simple in-memory implementation of p2p.Transport
// for testing the DHT Service integration.
type mockTransport struct {
	addr string
	ch   chan p2p.RPC
}

func newMockTransport(addr string) *mockTransport {
	return &mockTransport{
		addr: addr,
		ch:   make(chan p2p.RPC, 16),
	}
}

func (m *mockTransport) Addr() string {
	return m.addr
}

func (m *mockTransport) ListenAndAccept() error {
	// No-op for mock.
	return nil
}

func (m *mockTransport) Dial(addr string) error {
	// No-op for mock.
	return nil
}

func (m *mockTransport) Consume() <-chan p2p.RPC {
	return m.ch
}

func (m *mockTransport) Send(addr string, rpc p2p.RPC) error {
	// Outbound sending is not needed for this integration test,
	// so we can make this a no-op that satisfies the interface.
	return nil
}

func (m *mockTransport) Close() error {
	close(m.ch)
	return nil
}

func TestService_Integration_HandlePingFromTransport(t *testing.T) {
	self := MustRandomID()
	mt := newMockTransport("mock-addr")

	svc := NewService(ServiceOpts{
		ID:        self,
		Transport: mt,
		Store:     nil,
	})

	other := MustRandomID()

	// Build a PING DHT message.
	msg := &Message{
		Type:      MsgPing,
		From:      other,
		Timestamp: time.Now().Unix(),
	}
	payload, err := msg.Encode()
	require.NoError(t, err)

	// Simulate an incoming RPC from "other" over the mock transport.
	rpc := p2p.RPC{
		From:    "127.0.0.1:9999",
		Stream:  false,
		Payload: payload,
	}
	mt.ch <- rpc

	// Give the service some time to process the message.
	time.Sleep(50 * time.Millisecond)

	// The sender should now be in the routing table.
	c := findContactInRT(svc.Node().RoutingTable(), other)
	require.NotNil(t, c, "sender should be added to routing table via Service")
	assert.Equal(t, "127.0.0.1:9999", c.Address, "contact address mismatch")
}
