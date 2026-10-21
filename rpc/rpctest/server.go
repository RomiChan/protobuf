package rpctest

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/RomiChan/protobuf/rpc"
)

// Server serves RPC requests over in-memory connections for tests. Create it
// with NewServer. Its methods are safe for concurrent use.
//
// Closing a connection cancels the context passed to its handlers. Handlers
// must return when that context is canceled so cleanup can finish.
type Server struct {
	server *rpc.Server
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	closed  bool
	conns   map[*connection]struct{}
	clients []*rpc.Client
	wg      sync.WaitGroup
	once    sync.Once
}

// NewServer creates an RPC test server and registers Close with t.Cleanup.
// Connections are created on demand with net.Pipe; no network ports are used.
// A nil handler reports rpc.MethodNotFound for incoming calls.
//
// When using testing/synctest, create and use the server inside synctest.Test.
func NewServer(t testing.TB, handler rpc.Handler) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	s := &Server{
		server: rpc.NewServer(handler),
		ctx:    ctx,
		cancel: cancel,
		conns:  make(map[*connection]struct{}),
	}
	t.Cleanup(s.Close)
	return s
}

// Client returns a new pooled RPC client configured to dial this server.
// Close also closes every client created by this method. Optional options
// configure the client; if several are supplied, the last wins. DialContext is
// always replaced with the server's dialer. Calls after Close return a closed
// client.
func (s *Server) Client(options ...rpc.ClientOptions) *rpc.Client {
	var opts rpc.ClientOptions
	if len(options) > 0 {
		opts = options[len(options)-1]
	}
	opts.DialContext = s.DialContext
	client := rpc.NewClient("pipe", "rpctest", opts)
	s.mu.Lock()
	closed := s.closed
	if !closed {
		s.clients = append(s.clients, client)
	}
	s.mu.Unlock()
	if closed {
		client.Close()
	}
	return client
}

// DialContext creates an in-memory connection to this server. The network and
// address arguments are ignored, so the method can be used directly as
// rpc.ClientOptions.DialContext by application clients. The caller owns the
// returned connection. Canceling ctx after dialing does not close it.
// After Close, dialing returns net.ErrClosed.
func (s *Server) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return nil, net.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, peer := net.Pipe()
	base, cancel := context.WithCancel(s.ctx)
	c := &connection{
		client: client,
		server: &serverConn{Conn: peer, cancel: cancel},
		cancel: cancel,
		done:   make(chan struct{}),
	}
	s.conns[c] = struct{}{}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			c.close()
			s.mu.Lock()
			delete(s.conns, c)
			close(c.done)
			s.mu.Unlock()
		}()
		s.server.ServeConn(base, c.server)
	}()
	return client, nil
}

// CloseClientConnections closes the current connections, cancels their handler
// contexts, and waits for their server goroutines to exit. New connections may
// still be created. Clients observe the disconnection asynchronously; use
// synctest.Wait inside a synctest bubble before asserting reconnection.
// This method must not be called from a handler on this server.
func (s *Server) CloseClientConnections() {
	s.mu.Lock()
	conns := s.connectionsLocked()
	s.mu.Unlock()
	closeConnections(conns)
}

// Close prevents dialing, cancels handler contexts, closes all connections and
// clients created by Client, and waits for server goroutines to exit. It is safe
// to call repeatedly or concurrently, but must not be called from a handler.
func (s *Server) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		conns := s.connectionsLocked()
		clients := s.clients
		s.clients = nil
		s.mu.Unlock()
		closeConnections(conns)
		for _, client := range clients {
			client.Close()
		}
		s.wg.Wait()
	})
}

func (s *Server) connectionsLocked() []*connection {
	conns := make([]*connection, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	return conns
}

func closeConnections(conns []*connection) {
	for _, c := range conns {
		c.close()
	}
	for _, c := range conns {
		<-c.done
	}
}

type connection struct {
	client net.Conn
	server net.Conn
	cancel context.CancelFunc
	done   chan struct{}
}

func (c *connection) close() {
	c.cancel()
	c.client.Close()
	c.server.Close()
}

// serverConn cancels handlers as soon as rpc.Conn shuts down the transport,
// before ServeConn waits for those handlers to return.
type serverConn struct {
	net.Conn
	cancel context.CancelFunc
}

func (c *serverConn) Close() error {
	c.cancel()
	return c.Conn.Close()
}
