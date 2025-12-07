package p2p

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// TCPPeer is a concrete implementation of Peer over a net.Conn (TCP or TLS).
type TCPPeer struct {
	conn     net.Conn
	outbound bool
	wg       *sync.WaitGroup
}

// NewTCPPeer constructs a TCPPeer from a net.Conn and outbound flag.
func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		conn:     conn,
		outbound: outbound,
		wg:       &sync.WaitGroup{},
	}
}

// Addr returns the remote address as a string, e.g. "127.0.0.1:9000".
func (p *TCPPeer) Addr() string {
	return p.conn.RemoteAddr().String()
}

// Send writes raw bytes to the underlying connection.
// This satisfies the Peer interface.
func (p *TCPPeer) Send(b []byte) error {
	_, err := p.conn.Write(b)
	return err
}

// Close closes the connection.
func (p *TCPPeer) Close() error {
	return p.conn.Close()
}

// Outbound indicates whether we dialed this peer (true) or accepted it (false).
func (p *TCPPeer) Outbound() bool {
	return p.outbound
}

// CloseStream signals that a streaming operation finished.
// This matches the pattern from your original snippet.
func (p *TCPPeer) CloseStream() {
	p.wg.Done()
}

// TCPTransportOpts holds configuration for TCPTransport.
type TCPTransportOpts struct {
	ListenAddr    string        // e.g. "127.0.0.1:9000" or ":0" for random free port
	HandshakeFunc HandshakeFunc // optional handshake callback
	Decoder       Decoder       // how to decode incoming RPCs
	Encoder       Encoder       // how to encode outgoing RPCs
	OnPeer        func(Peer) error

	// Optional TLS configuration.
	// If non-nil, Dial() uses tls.Dial, and inbound conns are wrapped with tls.Server.
	TLSConfig *tls.Config
}

// TCPTransport is a concrete Transport implementation using TCP (and optional TLS).
type TCPTransport struct {
	TCPTransportOpts              // embed options for direct field access
	listener         net.Listener // TCP listener
	rpcCh            chan RPC     // channel for incoming RPCs

	peers   map[string]*TCPPeer // active peers by address (remote addr string)
	peersMu sync.RWMutex        // protects access to peers map
}

// NewTCPTransport creates a new TCPTransport with the given options.
func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {
	return &TCPTransport{
		TCPTransportOpts: opts,
		rpcCh:            make(chan RPC, 1024),
		peers:            make(map[string]*TCPPeer),
	}
}

// Addr returns the transport's listening address.
//
// If ListenAddr was ":0", after ListenAndAccept() this will be updated to
// the actual address chosen by the OS (e.g. "127.0.0.1:54321").
func (t *TCPTransport) Addr() string {
	return t.ListenAddr
}

// Consume returns a receive-only channel of incoming RPCs.
func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcCh
}

// Close stops the listener (if any). Existing connections are not force-closed here.
func (t *TCPTransport) Close() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

// ListenAndAccept starts listening on the configured address and launches
// the accept loop in a background goroutine.
func (t *TCPTransport) ListenAndAccept() error {
	var err error

	t.listener, err = net.Listen("tcp", t.ListenAddr)
	if err != nil {
		return err
	}

	// If ListenAddr was ":0", update it to the actual address selected.
	t.ListenAddr = t.listener.Addr().String()

	go t.startAcceptLoop()

	log.Printf("TCP transport listening on %s\n", t.ListenAddr)
	return nil
}

// Dial connects to a remote address and starts a connection handler in a goroutine.
func (t *TCPTransport) Dial(addr string) error {
	var conn net.Conn
	var err error

	if t.TLSConfig != nil {
		// Use TLS for outgoing connections.
		conn, err = tls.Dial("tcp", addr, t.TLSConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}

	go t.handleConn(conn, true)
	return nil
}

// Send encodes and sends an RPC to the peer at the given address.
// If there is no existing connection, it will attempt to Dial() first.
func (t *TCPTransport) Send(addr string, rpc RPC) error {
	t.peersMu.RLock()
	peer, ok := t.peers[addr]
	t.peersMu.RUnlock()

	if !ok {
		// No existing connection; try to dial.
		if err := t.Dial(addr); err != nil {
			return err
		}

		// Give handleConn some time to register the peer.
		time.Sleep(50 * time.Millisecond)

		t.peersMu.RLock()
		peer = t.peers[addr]
		t.peersMu.RUnlock()
		if peer == nil {
			return fmt.Errorf("could not connect to %s", addr)
		}
	}

	if t.Encoder == nil {
		return fmt.Errorf("no encoder configured")
	}

	return t.Encoder.Encode(peer.conn, &rpc)
}

// startAcceptLoop continuously accepts new connections until the listener is closed.
func (t *TCPTransport) startAcceptLoop() {
	for {
		conn, err := t.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			// Listener closed; exit loop.
			return
		}
		if err != nil {
			log.Printf("TCP accept error: %s\n", err)
			continue
		}

		// Wrap inbound connection with TLS if configured.
		if t.TLSConfig != nil {
			tlsConn := tls.Server(conn, t.TLSConfig)
			if err := tlsConn.Handshake(); err != nil {
				log.Printf("TLS handshake error: %v\n", err)
				_ = conn.Close()
				continue
			}
			conn = tlsConn
		}

		go t.handleConn(conn, false)
	}
}

// handleConn wraps the connection into a TCPPeer, performs handshake,
// invokes OnPeer, then enters the read loop using the Decoder.
func (t *TCPTransport) handleConn(conn net.Conn, outbound bool) {
	var err error

	peer := NewTCPPeer(conn, outbound)

	// Register peer.
	t.peersMu.Lock()
	t.peers[peer.Addr()] = peer
	t.peersMu.Unlock()

	defer func() {
		// Remove peer from map on exit.
		t.peersMu.Lock()
		delete(t.peers, peer.Addr())
		t.peersMu.Unlock()

		if err != nil {
			log.Printf("dropping peer %s due to error: %v\n", peer.Addr(), err)
		} else {
			log.Printf("closing peer %s\n", peer.Addr())
		}
		_ = peer.Close()
	}()

	// Perform handshake if provided.
	if t.HandshakeFunc != nil {
		if err = t.HandshakeFunc(peer); err != nil {
			return
		}
	}

	// Notify upper layers about the new peer.
	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			return
		}
	}

	// Read loop: decode RPCs using Decoder.
	for {
		if t.Decoder == nil {
			err = fmt.Errorf("no decoder configured")
			return
		}

		rpc := RPC{}
		err = t.Decoder.Decode(conn, &rpc)
		if err != nil {
			// Decode error usually means connection closed or broken.
			return
		}

		// Attach the peer's address to the RPC.
		rpc.From = peer.Addr()

		// Handle stream control RPCs specially.
		if rpc.Stream {
			peer.wg.Add(1)
			log.Printf("[%s] incoming stream, waiting...\n", peer.Addr())
			peer.wg.Wait()
			log.Printf("[%s] stream closed, resuming read loop\n", peer.Addr())
			continue
		}

		// Deliver RPC to consumers.
		t.rpcCh <- rpc
	}
}
