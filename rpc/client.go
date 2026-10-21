package rpc

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// ErrClientClosed is returned when a Client has been closed.
var ErrClientClosed = errors.New("rpc: client closed")

// ClientOptions configures a Client. The zero value uses eight connections,
// five-second dial and write timeouts, and no default call timeout.
type ClientOptions struct {
	// PoolSize is the number of round-robin slots. Values <= 0 use eight.
	// Each connection supports multiple concurrent requests.
	PoolSize int
	// DialTimeout bounds each dial, in addition to the initiating call's
	// context. Values <= 0 use five seconds.
	DialTimeout time.Duration
	// CallTimeout covers connection acquisition, writing, and waiting for a
	// reply. Values <= 0 disable it; an earlier caller deadline takes priority.
	// It also applies to Notify.
	CallTimeout time.Duration
	// WriteTimeout is passed to each Conn; see ConnOptions.WriteTimeout.
	WriteTimeout time.Duration
	// DialContext optionally replaces net.Dialer.DialContext. It must honor
	// cancellation and return a connection owned by the client on success.
	DialContext func(context.Context, string, string) (net.Conn, error)
}

type clientDial struct {
	done chan struct{}
	err  error // published by closing done
	conn *Conn
}

type clientSlot struct {
	conn *Conn
	dial *clientDial
}

// Client maintains a lazy pool of multiplexed RPC connections. Calls and
// notifications select slots in round-robin order. Failed connections are
// replaced on subsequent use; requests are never automatically replayed.
// A Client must be created with NewClient and must not be copied.
type Client struct {
	network string
	address string
	options ClientOptions
	ctx     context.Context
	cancel  context.CancelFunc

	mu     sync.Mutex
	slots  []clientSlot
	next   int
	closed bool
	dials  sync.WaitGroup

	closeOnce sync.Once
}

// NewClient creates a client without dialing. Close releases all connections.
func NewClient(network, address string, options ClientOptions) *Client {
	if options.PoolSize <= 0 {
		options.PoolSize = 8
	}
	if options.DialTimeout <= 0 {
		options.DialTimeout = 5 * time.Second
	}
	if options.DialContext == nil {
		options.DialContext = (&net.Dialer{}).DialContext
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		network: network,
		address: address,
		options: options,
		ctx:     ctx,
		cancel:  cancel,
		slots:   make([]clientSlot, options.PoolSize),
	}
}

// Call invokes method using a pooled connection. Its type and error semantics
// match Conn.Call. The timeout, if configured, starts before connection lookup.
func (c *Client) Call[R, P any](ctx context.Context, method string, params *P) (R, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()
	conn, err := c.connection(ctx)
	if err != nil {
		var result R
		return result, err
	}
	return conn.Call[R](ctx, method, params)
}

// Notify sends a notification using a pooled connection; see Conn.Notify.
func (c *Client) Notify[P any](ctx context.Context, method string, params *P) error {
	ctx, cancel := c.callContext(ctx)
	defer cancel()
	conn, err := c.connection(ctx)
	if err != nil {
		return err
	}
	return conn.Notify(ctx, method, params)
}

func (c *Client) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.options.CallTimeout > 0 {
		return context.WithTimeout(ctx, c.options.CallTimeout)
	}
	return ctx, func() {}
}

func (c *Client) connection(ctx context.Context) (*Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClientClosed
	}
	slot := &c.slots[c.next]
	c.next = (c.next + 1) % len(c.slots)
	if slot.conn != nil {
		select {
		case <-slot.conn.Done():
			slot.conn = nil
		default:
			conn := slot.conn
			c.mu.Unlock()
			return conn, ctx.Err()
		}
	}
	dial := slot.dial
	if dial == nil {
		dial = &clientDial{done: make(chan struct{})}
		slot.dial = dial
		c.dials.Add(1)
		go c.dial(ctx, slot, dial)
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, ErrClientClosed
	case <-dial.done:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Check cancellation and closure even if dialing completed concurrently.
	if c.closed {
		return nil, ErrClientClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return dial.conn, dial.err
}

func (c *Client) dial(ctx context.Context, slot *clientSlot, dial *clientDial) {
	defer c.dials.Done()
	ctx, cancel := context.WithTimeout(ctx, c.options.DialTimeout)
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	defer cancel()
	if c.ctx.Err() != nil {
		cancel()
	}
	var nc net.Conn
	err := ctx.Err()
	if err == nil {
		nc, err = c.options.DialContext(ctx, c.network, c.address)
	}
	c.mu.Lock()
	if c.closed {
		err = ErrClientClosed
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		slot.conn = NewConn(nc, ConnOptions{WriteTimeout: c.options.WriteTimeout})
		dial.conn = slot.conn
	}
	slot.dial = nil
	dial.err = err
	close(dial.done)
	c.mu.Unlock()
	if err != nil && nc != nil {
		nc.Close()
	}
}

// Close prevents new calls and dials, cancels active dials, wakes connection
// waiters, and closes every pooled connection. It waits for dials and connection
// shutdown to finish and is safe to call concurrently or more than once.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.cancel()
		conns := make([]*Conn, 0, len(c.slots))
		for i := range c.slots {
			if conn := c.slots[i].conn; conn != nil {
				conns = append(conns, conn)
				c.slots[i].conn = nil
			}
		}
		c.mu.Unlock()
		// Signal every connection before waiting for any handlers to drain.
		for _, conn := range conns {
			conn.shutdown(errClosed)
		}
		for _, conn := range conns {
			conn.Close()
		}
		c.dials.Wait()
	})
	return nil
}
