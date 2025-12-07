package dht

import (
	"time"

	"github.com/kunal-geeks/decentfs/internal/p2p"
)

// SendPing sends a DHT PING message to the given peer address.
func (s *Service) SendPing(addr string) error {
	msg := &Message{
		Type:      MsgPing,
		From:      s.node.id,
		Timestamp: time.Now().Unix(),
	}

	payload, err := msg.Encode()
	if err != nil {
		return err
	}

	rpc := p2p.RPC{
		Stream:  false,
		Payload: payload,
	}

	return s.transport.Send(addr, rpc)
}

// SendFindNode sends a FIND_NODE(target) message to the given peer address.
func (s *Service) SendFindNode(addr string, target ID) error {
	msg := &Message{
		Type:      MsgFindNode,
		From:      s.node.id,
		Target:    &target,
		Timestamp: time.Now().Unix(),
	}

	payload, err := msg.Encode()
	if err != nil {
		return err
	}

	rpc := p2p.RPC{
		Stream:  false,
		Payload: payload,
	}

	return s.transport.Send(addr, rpc)
}
