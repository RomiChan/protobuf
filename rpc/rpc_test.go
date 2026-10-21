package rpc

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/RomiChan/protobuf/proto"
)

// drain discards everything written to c so that writes to its peer do
// not block, simulating a peer that reads but never responds.
func drain(c net.Conn) {
	go io.Copy(io.Discard, c)
}

type echoArgs struct {
	Msg string `protobuf:"bytes,1,opt"`
}

type echoReply struct {
	Msg string `protobuf:"bytes,1,opt"`
}

type testHandler struct {
	mu       sync.Mutex
	notified []string
}

func (h *testHandler) Handle(ctx context.Context, req *Request) (interface{}, error) {
	switch req.Method {
	case "echo":
		args, err := req.UnmarshalParams[echoArgs]()
		if err != nil {
			return nil, &Error{Code: InvalidParams, Message: err.Error()}
		}
		return &echoReply{Msg: args.Msg}, nil
	case "fail":
		return nil, &Error{Code: 42, Message: "boom"}
	case "plainfail":
		return nil, errors.New("plain failure")
	case "panic":
		panic("oh no")
	case "notify":
		args, err := req.UnmarshalParams[echoArgs]()
		if err != nil {
			return nil, err
		}
		h.mu.Lock()
		h.notified = append(h.notified, args.Msg)
		h.mu.Unlock()
		return nil, nil
	case "noresult":
		return nil, nil
	}
	return nil, &Error{Code: MethodNotFound, Message: "unknown method: " + req.Method}
}

func newPipe(t *testing.T) (*Conn, *testHandler) {
	t.Helper()
	c1, c2 := net.Pipe()
	h := &testHandler{}
	srv := NewServer(h)
	go srv.ServeConn(context.Background(), c2)
	cli := NewConn(c1)
	t.Cleanup(func() { cli.Close() })
	return cli, h
}

func TestCall(t *testing.T) {
	cli, _ := newPipe(t)
	reply, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "hello"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reply.Msg != "hello" {
		t.Fatalf("reply.Msg = %q, want %q", reply.Msg, "hello")
	}
}

func TestCallNilParamsAndResult(t *testing.T) {
	cli, _ := newPipe(t)
	if _, err := cli.Call[echoReply](context.Background(), "noresult", (*echoArgs)(nil)); err != nil {
		t.Fatalf("Call: %v", err)
	}
}

func TestCallError(t *testing.T) {
	cli, _ := newPipe(t)
	_, err := cli.Call[echoReply](context.Background(), "fail", (*echoArgs)(nil))
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if rerr.Code != 42 || rerr.Message != "boom" {
		t.Fatalf("rerr = %+v", rerr)
	}
}

func TestCallPlainError(t *testing.T) {
	cli, _ := newPipe(t)
	_, err := cli.Call[echoReply](context.Background(), "plainfail", (*echoArgs)(nil))
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if rerr.Code != InternalError {
		t.Fatalf("code = %d, want InternalError", rerr.Code)
	}
}

func TestCallHandlerPanic(t *testing.T) {
	cli, _ := newPipe(t)
	_, err := cli.Call[echoReply](context.Background(), "panic", (*echoArgs)(nil))
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if rerr.Code != InternalError {
		t.Fatalf("code = %d, want InternalError", rerr.Code)
	}
}

func TestCallMethodNotFound(t *testing.T) {
	cli, _ := newPipe(t)
	_, err := cli.Call[echoReply](context.Background(), "nosuchmethod", (*echoArgs)(nil))
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if rerr.Code != MethodNotFound {
		t.Fatalf("code = %d, want MethodNotFound", rerr.Code)
	}
}

func TestCallWithoutServerHandler(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()
	// peer is a bare client conn with no handler
	peer := NewConn(c2)
	defer peer.Close()
	cli := NewConn(c1)
	defer cli.Close()

	_, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "hi"})
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if rerr.Code != MethodNotFound {
		t.Fatalf("code = %d, want MethodNotFound", rerr.Code)
	}
}

func TestNotify(t *testing.T) {
	cli, h := newPipe(t)
	if err := cli.Notify(context.Background(), "notify", &echoArgs{Msg: "ping"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		n := len(h.notified)
		h.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.notified) != 1 || h.notified[0] != "ping" {
		t.Fatalf("notified = %v, want [ping]", h.notified)
	}
}

func TestConcurrentCalls(t *testing.T) {
	cli, _ := newPipe(t)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := string(rune('a' + i))
			reply, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: msg})
			if err != nil {
				errs <- err
				return
			}
			if reply.Msg != msg {
				errs <- errors.New("mismatched reply: " + reply.Msg)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestCallContextCancel(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()
	drain(c2) // peer reads but never responds
	cli := NewConn(c1)
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := cli.Call[echoReply](ctx, "echo", &echoArgs{Msg: "hi"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestCloseFailsPendingCalls(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()
	drain(c2) // peer reads but never responds
	cli := NewConn(c1)

	done := make(chan error, 1)
	go func() {
		_, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "hi"})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cli.Close()
	if err := <-done; err == nil {
		t.Fatal("pending call succeeded after Close")
	}
	if _, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "hi"}); err == nil {
		t.Fatal("call on closed conn succeeded")
	}
}

func TestPeerClose(t *testing.T) {
	c1, c2 := net.Pipe()
	cli := NewConn(c1)
	defer cli.Close()
	c2.Close()

	done := make(chan error, 1)
	go func() {
		_, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "hi"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("call succeeded after peer closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not fail after peer closed")
	}
}

func TestHandlerOf(t *testing.T) {
	h := HandlerOf(func(ctx context.Context, args *echoArgs) (*echoReply, error) {
		return &echoReply{Msg: args.Msg + "!"}, nil
	})
	c1, c2 := net.Pipe()
	go NewServer(h).ServeConn(context.Background(), c2)
	cli := NewConn(c1)
	defer cli.Close()

	reply, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "typed"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reply.Msg != "typed!" {
		t.Fatalf("reply.Msg = %q, want %q", reply.Msg, "typed!")
	}
}

func TestHandlerOfNilReply(t *testing.T) {
	h := HandlerOf(func(ctx context.Context, args *echoArgs) (*echoReply, error) {
		return nil, nil
	})
	c1, c2 := net.Pipe()
	go NewServer(h).ServeConn(context.Background(), c2)
	cli := NewConn(c1)
	defer cli.Close()

	if _, err := cli.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "x"}); err != nil {
		t.Fatalf("Call: %v", err)
	}
}

func TestHandlerOfInvalidParams(t *testing.T) {
	h := HandlerOf(func(ctx context.Context, args *echoArgs) (*echoReply, error) {
		return &echoReply{Msg: args.Msg}, nil
	})
	c1, c2 := net.Pipe()
	go NewServer(h).ServeConn(context.Background(), c2)
	cli := NewConn(c1)
	defer cli.Close()

	// field 1 with a varint wire type where the target expects bytes
	type badArgs struct {
		Msg int32 `protobuf:"varint,1,opt"`
	}
	_, err := cli.Call[echoReply](context.Background(), "echo", &badArgs{Msg: 1})
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if rerr.Code != InvalidParams {
		t.Fatalf("code = %d, want InvalidParams", rerr.Code)
	}
}

func TestTCPRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := NewServer(&testHandler{})
	go srv.Serve(ctx, l)

	cli, err := Dial(ctx, "tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer cli.Close()

	reply, err := cli.Call[echoReply](ctx, "echo", &echoArgs{Msg: "over tcp"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reply.Msg != "over tcp" {
		t.Fatalf("reply.Msg = %q", reply.Msg)
	}
}

func TestConnWriteTimeout(t *testing.T) {
	for _, notify := range []bool{false, true} {
		a, b := net.Pipe()
		c := NewConn(a, ConnOptions{WriteTimeout: 20 * time.Millisecond})
		t.Cleanup(func() { c.Close(); b.Close() })
		result := make(chan error, 1)
		go func() {
			if notify {
				result <- c.Notify(context.Background(), "notify", (*echoArgs)(nil))
			} else {
				_, err := c.Call[echoReply](context.Background(), "echo", (*echoArgs)(nil))
				result <- err
			}
		}()
		var timeout net.Error
		if err := receive(t, result); !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("write error = %v, want timeout", err)
		}
		receive(t, c.Done())
	}
}

func TestConnWriteUsesCallDeadline(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	c := NewConn(a, ConnOptions{WriteTimeout: time.Hour})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := c.Call[echoReply](ctx, "echo", (*echoArgs)(nil))
		result <- err
	}()
	if err := receive(t, result); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("write error = %v, want context.DeadlineExceeded", err)
	}
	receive(t, c.Done())
}

// observedWriteConn signals when the first transport write is blocked.
type observedWriteConn struct {
	net.Conn
	once    sync.Once
	started chan struct{}
}

func (c *observedWriteConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(p)
}

func TestConnCancelWhileWritingAndWaitingForWriter(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	transport := &observedWriteConn{Conn: a, started: make(chan struct{})}
	c := NewConn(transport, ConnOptions{WriteTimeout: -1})
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() {
		_, err := c.Call[echoReply](ctx, "echo", (*echoArgs)(nil))
		first <- err
	}()
	receive(t, transport.started)
	waiter, cancelWaiter := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelWaiter()
	second := make(chan error, 1)
	go func() { second <- c.Notify(waiter, "notify", (*echoArgs)(nil)) }()
	if err := receive(t, second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting writer error = %v", err)
	}
	select {
	case <-c.Done():
		t.Fatal("canceling a waiting writer closed the transport")
	default:
	}
	cancel()
	if err := receive(t, first); !errors.Is(err, context.Canceled) {
		t.Fatalf("active writer error = %v", err)
	}
	receive(t, c.Done())
}

func TestConnCanceledCallDoesNotWrite(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	transport := &observedWriteConn{Conn: a, started: make(chan struct{})}
	c := NewConn(transport)
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Call[echoReply](ctx, "echo", (*echoArgs)(nil)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-transport.started:
		t.Fatal("canceled call wrote a request")
	default:
	}
}

type failingWriteTransport struct {
	io.ReadWriteCloser
	err   error
	short bool
}

func (c *failingWriteTransport) Write(p []byte) (int, error) {
	if c.short {
		return len(p) - 1, nil
	}
	return 0, c.err
}

func TestConnWriteFailureClosesTransport(t *testing.T) {
	writeErr := errors.New("write failed")
	for _, short := range []bool{false, true} {
		a, b := net.Pipe()
		c := NewConn(&failingWriteTransport{ReadWriteCloser: a, err: writeErr, short: short})
		t.Cleanup(func() { c.Close(); b.Close() })
		want := writeErr
		if short {
			want = io.ErrShortWrite
		}
		if err := c.Notify(context.Background(), "notify", (*echoArgs)(nil)); !errors.Is(err, want) {
			t.Fatal(err)
		}
		receive(t, c.Done())
	}
}

func TestServerResponseWriteFailureDoesNotDeadlock(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	c := newConn(context.Background(), a, &testHandler{}, ConnOptions{WriteTimeout: 20 * time.Millisecond})
	defer c.Close()
	if err := writeFrame(b, &frame{ID: proto.Some[int64](1), Method: "noresult"}); err != nil {
		t.Fatal(err)
	}
	// The peer never reads the response. A handler writing it must be able to
	// shut down the connection without waiting for its own goroutine to exit.
	receive(t, c.Done())
}

func TestConnWriteDeadlineRefresh(t *testing.T) {
	a, b := net.Pipe()
	c := NewConn(a, ConnOptions{WriteTimeout: 40 * time.Millisecond})
	defer c.Close()
	go NewServer(&testHandler{}).ServeConn(context.Background(), b)
	for range 2 {
		if _, err := c.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "hello"}); err != nil {
			t.Fatal(err)
		}
		// An expired deadline from the previous write must not affect the next.
		time.Sleep(60 * time.Millisecond)
	}
}
