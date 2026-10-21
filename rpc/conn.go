package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"
	"time"

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

// ConnOptions configures a Conn.
type ConnOptions struct {
	// WriteTimeout bounds each transport write when the transport implements
	// SetWriteDeadline(time.Time) error (as net.Conn does). Zero uses five
	// seconds; a negative value disables this timeout. An earlier call deadline
	// takes priority, and cancellation interrupts writes on these transports.
	// Other io.ReadWriteClosers cannot enforce write deadlines.
	WriteTimeout time.Duration
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

	wmu          chan struct{} // serializes writes, with cancellable acquisition
	writeTimeout time.Duration

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
// Optional options override the defaults; if several are supplied, the last wins.
func NewConn(rwc io.ReadWriteCloser, options ...ConnOptions) *Conn {
	return newConn(context.Background(), rwc, nil, options...)
}

// Dial connects to address on the named network (see net.Dial) and
// returns a client-side connection ready for calls.
func Dial(ctx context.Context, network, address string, options ...ConnOptions) (*Conn, error) {
	var d net.Dialer
	nc, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return newConn(context.Background(), nc, nil, options...), nil
}

func newConn(ctx context.Context, rwc io.ReadWriteCloser, h Handler, options ...ConnOptions) *Conn {
	writeTimeout := 5 * time.Second
	if len(options) > 0 && options[len(options)-1].WriteTimeout != 0 {
		writeTimeout = options[len(options)-1].WriteTimeout
	}
	c := &Conn{
		rwc:          rwc,
		handler:      h,
		ctx:          ctx,
		pending:      make(map[int64]chan callResult),
		done:         make(chan struct{}),
		wmu:          make(chan struct{}, 1),
		writeTimeout: writeTimeout,
	}
	c.wmu <- struct{}{}
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
	if err := ctx.Err(); err != nil {
		return result, err
	}
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

	if err := c.write(ctx, &frame{ID: proto.Some(id), Method: method, Params: p}); err != nil {
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
	return c.write(ctx, &frame{Method: method, Params: p})
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

func (c *Conn) write(ctx context.Context, f *frame) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.terminalError()
	case <-c.wmu:
	}
	defer func() { c.wmu <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if writer, ok := c.rwc.(interface{ SetWriteDeadline(time.Time) error }); ok {
		var deadline time.Time
		if c.writeTimeout > 0 {
			deadline = time.Now().Add(c.writeTimeout)
		}
		if d, ok := ctx.Deadline(); ok && (deadline.IsZero() || d.Before(deadline)) {
			deadline = d
		}
		if err := writer.SetWriteDeadline(deadline); err != nil {
			c.shutdown(err)
			return err
		}
		if ctx.Done() != nil {
			canceled := make(chan struct{})
			stop := context.AfterFunc(ctx, func() {
				writer.SetWriteDeadline(time.Now())
				close(canceled)
			})
			defer func() {
				if !stop() {
					<-canceled
				}
			}()
		}
	}
	if err := writeFrame(c.rwc, f); err != nil {
		c.shutdown(err)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// The transport deadline can fire just before the context's timer runs.
		if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
			return context.DeadlineExceeded
		}
		return err
	}
	return ctx.Err()
}

func (c *Conn) terminalError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Conn) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// run is the read loop: it dispatches incoming requests to the handler
// and routes responses to waiting callers.
func (c *Conn) run() {
	defer func() {
		c.wg.Wait()
		close(c.done)
	}()
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
			c.write(context.Background(), &frame{ID: f.ID, Error: &Error{
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
	c.write(context.Background(), resp)
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
// calls with err. The read loop closes done after handler goroutines drain.
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
	})
}
