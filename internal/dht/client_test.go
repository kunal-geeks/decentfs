package dht

import (
	"testing"

	"github.com/kunal-geeks/decentfs/internal/p2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSendTransport is a mock p2p.Transport that records outgoing RPCs.
type mockSendTransport struct {
	addr string
	ch   chan p2p.RPC
	sent []p2p.RPC
}

func newMockSendTransport(addr string) *mockSendTransport {
	return &mockSendTransport{
		addr: addr,
		ch:   make(chan p2p.RPC, 16),
		sent: make([]p2p.RPC, 0),
	}
}

func (m *mockSendTransport) Addr() string            { return m.addr }
func (m *mockSendTransport) ListenAndAccept() error  { return nil }
func (m *mockSendTransport) Dial(addr string) error  { return nil }
func (m *mockSendTransport) Consume() <-chan p2p.RPC { return m.ch }
func (m *mockSendTransport) Close() error            { close(m.ch); return nil }
func (m *mockSendTransport) Send(addr string, rpc p2p.RPC) error {
	m.sent = append(m.sent, rpc)
	return nil
}

func TestService_SendPing(t *testing.T) {
	self := MustRandomID()
	mt := newMockSendTransport("mock")
	svc := NewService(ServiceOpts{
		ID:        self,
		Transport: mt,
	})

	err := svc.SendPing("127.0.0.1:9100")
	require.NoError(t, err)
	require.Len(t, mt.sent, 1)

	msg, err := DecodeMessage(mt.sent[0].Payload)
	require.NoError(t, err)

	assert.Equal(t, MsgPing, msg.Type)
	assert.True(t, msg.From.Equals(self))
}

func TestService_SendFindNode(t *testing.T) {
	self := MustRandomID()
	target := MustRandomID()
	mt := newMockSendTransport("mock")
	svc := NewService(ServiceOpts{
		ID:        self,
		Transport: mt,
		Store:     nil,
	})

	err := svc.SendFindNode("127.0.0.1:9200", target)
	require.NoError(t, err)
	require.Len(t, mt.sent, 1)

	msg, err := DecodeMessage(mt.sent[0].Payload)
	require.NoError(t, err)

	assert.Equal(t, MsgFindNode, msg.Type)
	assert.True(t, msg.From.Equals(self))
	require.NotNil(t, msg.Target)
	assert.True(t, msg.Target.Equals(target))
}
