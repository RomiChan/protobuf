package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"

	"github.com/RomiChan/protobuf/proto"
)

// errClosed is reported to pending and future calls on a closed Conn
// when no more specific cause is known.
var errClosed = errors.New("rpc: connection closed")

// callResult is delivered to a waiting Call.
type callResult struct {
	resp *frame
	err  error
}

// Conn is a bidirectional RPC connection. It serves both the client
// side (Call, Notify) and, when constructed with a Handler, the server
// side. Multiple calls may be in flight concurrently.
//
// A Conn must be created with NewConn or Dial, or obtained from a
// Server. Close it to release resources.
type Conn struct {
	rwc     io.ReadWriteCloser
	handler Handler
	ctx     context.Context // base context for handler invocations

	wmu sync.Mutex // serializes writes to rwc

	mu      sync.Mutex
	seq     int64
	pending map[int64]chan callResult
	err     error // terminal error, set once closed

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup // in-flight handler goroutines
}

// NewConn creates a client-side connection over rwc and starts its read
// loop. rwc is closed when the connection is closed or breaks.
func NewConn(rwc io.ReadWriteCloser) *Conn {
	return newConn(context.Background(), rwc, nil)
}

// Dial connects to address on the named network (see net.Dial) and
// returns a client-side connection ready for calls.
func Dial(ctx context.Context, network, address string) (*Conn, error) {
	var d net.Dialer
	nc, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return newConn(context.Background(), nc, nil), nil
}

func newConn(ctx context.Context, rwc io.ReadWriteCloser, h Handler) *Conn {
	c := &Conn{
		rwc:     rwc,
		handler: h,
		ctx:     ctx,
		pending: make(map[int64]chan callResult),
		done:    make(chan struct{}),
	}
	go c.run()
	return c
}

// Call invokes method on the peer with protobuf-marshaled params and
// waits for the response, decoding it into a new R. R and P must be
// struct types with protobuf tags (not pointers). For methods without
// parameters, pass a typed nil, e.g. conn.Call[Reply](ctx, "m", (*Args)(nil)).
// If the peer reports an error, Call returns it as an *Error. Canceling
// ctx abandons the call.
func (c *Conn) Call[R, P any](ctx context.Context, method string, params *P) (R, error) {
	var result R
	var p proto.RawMessage
	if params != nil {
		b, err := proto.Marshal(params)
		if err != nil {
			return result, fmt.Errorf("rpc.Call(%s): %w", method, err)
		}
		p = b
	}

	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return result, err
	}
	c.seq++
	id := c.seq
	ch := make(chan callResult, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.write(&frame{ID: proto.Some(id), Method: method, Params: p}); err != nil {
		c.removePending(id)
		return result, err
	}

	select {
	case r := <-ch:
		if r.err != nil {
			return result, r.err
		}
		if r.resp.Error != nil {
			return result, r.resp.Error
		}
		if len(r.resp.Result) > 0 {
			if err := proto.Unmarshal(r.resp.Result, &result); err != nil {
				return result, fmt.Errorf("rpc.Call(%s): %w", method, err)
			}
		}
		return result, nil
	case <-ctx.Done():
		c.removePending(id)
		return result, ctx.Err()
	}
}

// Notify sends a notification: a request the peer must not respond to.
// P must be a struct type with protobuf tags (not a pointer); pass a
// typed nil for notifications without parameters. There is no
// acknowledgment; delivery errors surface only if the underlying write
// fails.
func (c *Conn) Notify[P any](ctx context.Context, method string, params *P) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var p proto.RawMessage
	if params != nil {
		b, err := proto.Marshal(params)
		if err != nil {
			return fmt.Errorf("rpc.Notify(%s): %w", method, err)
		}
		p = b
	}
	return c.write(&frame{Method: method, Params: p})
}

// Close shuts down the connection, failing all pending calls, and
// blocks until in-flight handler goroutines return.
func (c *Conn) Close() error {
	c.shutdown(errClosed)
	<-c.done
	return nil
}

// Done returns a channel closed when the connection terminates, whether
// by Close, a broken transport, or the peer going away.
func (c *Conn) Done() <-chan struct{} { return c.done }

func (c *Conn) write(f *frame) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return writeFrame(c.rwc, f)
}

func (c *Conn) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// run is the read loop: it dispatches incoming requests to the handler
// and routes responses to waiting callers.
func (c *Conn) run() {
	err := errClosed
	for {
		var f *frame
		f, err = readFrame(c.rwc)
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			break
		}
		if f.Method != "" {
			c.wg.Add(1)
			go func() {
				defer c.wg.Done()
				c.dispatch(f)
			}()
			continue
		}
		if f.ID.IsNone() {
			continue
		}
		c.mu.Lock()
		ch := c.pending[f.ID.Unwrap()]
		delete(c.pending, f.ID.Unwrap())
		c.mu.Unlock()
		if ch != nil {
			ch <- callResult{resp: f}
		}
	}
	c.shutdown(err)
}

// dispatch invokes the handler for one incoming request and, unless it
// is a notification, writes the response.
func (c *Conn) dispatch(f *frame) {
	if c.handler == nil {
		if f.ID.IsSome() {
			c.write(&frame{ID: f.ID, Error: &Error{
				Code:    MethodNotFound,
				Message: "rpc: no handler for incoming requests",
			}})
		}
		return
	}

	result, err := c.safelyHandle(&Request{Method: f.Method, Params: f.Params})

	if f.ID.IsNone() {
		return // notification
	}
	resp := &frame{ID: f.ID}
	if err != nil {
		if re, ok := err.(*Error); ok {
			resp.Error = re
		} else {
			resp.Error = &Error{Code: InternalError, Message: err.Error()}
		}
	} else {
		resp.Result = result
	}
	c.write(resp)
}

// safelyHandle invokes the handler and marshals its result, converting
// panics and marshal failures into errors. A nil (or typed-nil) result
// yields nil bytes.
func (c *Conn) safelyHandle(req *Request) (result proto.RawMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = &Error{Code: InternalError, Message: fmt.Sprintf("rpc: handler panic: %v", r)}
		}
	}()
	res, err := c.handler.Handle(c.ctx, req)
	if err != nil || res == nil {
		return nil, err
	}
	if v := reflect.ValueOf(res); v.Kind() == reflect.Ptr && v.IsNil() {
		return nil, nil
	}
	b, err := proto.Marshal(res)
	if err != nil {
		return nil, &Error{Code: InternalError, Message: fmt.Sprintf("rpc: marshal result: %v", err)}
	}
	return proto.RawMessage(b), nil
}

// shutdown terminates the connection exactly once, failing all pending
// calls with err and waiting for handler goroutines to drain.
func (c *Conn) shutdown(err error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.err = err
		for id, ch := range c.pending {
			ch <- callResult{err: err}
			delete(c.pending, id)
		}
		c.mu.Unlock()
		c.rwc.Close()
		c.wg.Wait()
		close(c.done)
	})
}
