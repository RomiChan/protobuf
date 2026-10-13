package rpc

import (
	"context"
	"io"
	"net"
)

// Server accepts RPC connections and dispatches their requests to a
// Handler.
type Server struct {
	handler Handler
}

// NewServer returns a Server dispatching requests to h.
func NewServer(h Handler) *Server {
	return &Server{handler: h}
}

// ServeConn serves a single connection, blocking until the connection
// terminates. rwc is closed when ServeConn returns.
func (s *Server) ServeConn(ctx context.Context, rwc io.ReadWriteCloser) error {
	c := newConn(rwc, s.handler)
	c.ctx = ctx
	<-c.Done()
	return nil
}

// Serve accepts connections from l and serves each in its own
// goroutine until l fails or ctx is canceled, which closes l.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		l.Close()
	}()
	for {
		rwc, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.ServeConn(ctx, rwc)
	}
}

// ListenAndServe announces on the local network address (see
// net.Listen) and serves incoming connections with h until ctx is
// canceled or the listener fails.
func ListenAndServe(ctx context.Context, network, address string, h Handler) error {
	l, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	return NewServer(h).Serve(ctx, l)
}
