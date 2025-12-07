package dht

import (
	"fmt"
	"testing"
	"time"

	"github.com/kunal-geeks/decentfs/internal/p2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNode_HandleRPC_PingAddsContactAndRespondsPong(t *testing.T) {
	self := MustRandomID()
	node := NewNode(self)

	other := MustRandomID()

	// Build a PING message from "other".
	msg := &Message{
		Type:      MsgPing,
		From:      other,
		Timestamp: time.Now().Unix(),
	}
	payload, err := msg.Encode()
	require.NoError(t, err)

	rpc := p2p.RPC{
		From:    "127.0.0.1:9001",
		Stream:  false,
		Payload: payload,
		Meta:    nil,
	}

	resp, err := node.HandleRPC(rpc)
	require.NoError(t, err)
	require.NotNil(t, resp, "PING should yield a response")

	assert.Equal(t, MsgPong, resp.Type, "response should be PONG")
	assert.True(t, resp.From.Equals(self), "PONG should be from self")

	// Sender should be in routing table now.
	c := findContactInRT(node.RoutingTable(), other)
	require.NotNil(t, c, "sender should be added to routing table")
	assert.Equal(t, "127.0.0.1:9001", c.Address, "contact address mismatch")
}

func TestNode_HandleRPC_FindNodeReturnsClosest(t *testing.T) {
	self := MustRandomID()
	node := NewNode(self)

	// Populate routing table with some contacts.
	for i := 0; i < 10; i++ {
		id := MustRandomID()
		c := Contact{
			ID:       id,
			Address:  fmt.Sprintf("127.0.0.1:910%d", i),
			LastSeen: time.Now(),
		}
		node.RoutingTable().AddContact(c)
	}

	target := MustRandomID()

	// Build a FIND_NODE message from some other node.
	other := MustRandomID()
	msg := &Message{
		Type:      MsgFindNode,
		From:      other,
		Target:    &target,
		Timestamp: time.Now().Unix(),
	}
	payload, err := msg.Encode()
	require.NoError(t, err)

	rpc := p2p.RPC{
		From:    "127.0.0.1:9200",
		Stream:  false,
		Payload: payload,
	}

	resp, err := node.HandleRPC(rpc)
	require.NoError(t, err)
	require.NotNil(t, resp, "FIND_NODE should yield a response")
	assert.Equal(t, MsgNodes, resp.Type, "response type should be NODES")

	// Compute expected closest using routing table directly.
	expected := node.RoutingTable().FindClosest(target, K)

	// We don't require exact order equality in tests (though it should match),
	// but we at least ensure the same IDs are present.
	require.LessOrEqual(t, len(resp.Nodes), len(expected))
	if len(resp.Nodes) == 0 {
		t.Fatalf("expected some nodes in response")
	}

	// Build a set of expected IDs.
	expectedMap := make(map[string]struct{})
	for _, c := range expected {
		expectedMap[c.ID.String()] = struct{}{}
	}

	for _, c := range resp.Nodes {
		_, ok := expectedMap[c.ID.String()]
		assert.True(t, ok, "response contains a node not in expected closest set")
	}
}
