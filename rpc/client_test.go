package rpc

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for goroutine")
		var zero T
		return zero
	}
}

func echoTransport() net.Conn {
	a, b := net.Pipe()
	go NewServer(&testHandler{}).ServeConn(context.Background(), b)
	return a
}

func TestClientLazyRoundRobin(t *testing.T) {
	var dials int
	c := NewClient("test", "address", ClientOptions{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "test" || address != "address" {
				t.Errorf("dial target = %s %s", network, address)
			}
			dials++
			return echoTransport(), nil
		},
	})
	defer c.Close()
	if dials != 0 || len(c.slots) != 8 || c.options.DialTimeout != 5*time.Second {
		t.Fatal("unexpected client defaults or eager dial")
	}
	conns := make([]*Conn, 8)
	for i := range 24 {
		conn, err := c.connection(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if i < 8 {
			conns[i] = conn
		} else if conn != conns[i%8] {
			t.Fatal("slot connection was not reused in round-robin order")
		}
	}
	if dials != 8 {
		t.Fatalf("dials = %d, want 8", dials)
	}
}

func TestClientConcurrentDialAndCalls(t *testing.T) {
	for _, poolSize := range []int{1, 3} {
		t.Run(string(rune('0'+poolSize)), func(t *testing.T) {
			var dials atomic.Int32
			started := make(chan struct{}, 24)
			release := make(chan struct{})
			defer close(release)
			c := NewClient("test", "address", ClientOptions{
				PoolSize: poolSize,
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					dials.Add(1)
					started <- struct{}{}
					select {
					case <-release:
						return echoTransport(), nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				},
			})
			defer c.Close()
			results := make(chan error, 24)
			for i := range 24 {
				go func() {
					msg := string(rune('a' + i))
					reply, err := c.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: msg})
					if err == nil && reply.Msg != msg {
						err = errors.New("mismatched reply")
					}
					results <- err
				}()
			}
			// Every slot must start dialing before any dial is allowed to finish.
			for range poolSize {
				receive(t, started)
			}
			for range poolSize {
				release <- struct{}{}
			}
			for range 24 {
				if err := receive(t, results); err != nil {
					t.Fatal(err)
				}
			}
			if got := dials.Load(); got != int32(poolSize) {
				t.Fatalf("dials = %d, want %d", got, poolSize)
			}
		})
	}
}

func TestClientWaiterCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c := NewClient("test", "address", ClientOptions{
		PoolSize: 1,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			close(started)
			select {
			case <-release:
				return echoTransport(), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})
	defer c.Close()
	first := make(chan error, 1)
	go func() {
		_, err := c.connection(context.Background())
		first <- err
	}()
	receive(t, started)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.connection(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting call error = %v", err)
	}
	release <- struct{}{}
	if err := receive(t, first); err != nil {
		t.Fatalf("waiter cancellation affected initiating call: %v", err)
	}
}

func TestClientTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dial     bool
		call     time.Duration
		deadline bool
	}{
		{name: "dial", dial: true},
		{name: "call includes dial", dial: true, call: 20 * time.Millisecond},
		{name: "call waits for reply", call: 30 * time.Millisecond},
		{name: "caller deadline wins", call: time.Hour, deadline: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient("test", "address", ClientOptions{
				PoolSize:    1,
				DialTimeout: 60 * time.Millisecond,
				CallTimeout: tc.call,
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					if tc.dial {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					a, b := net.Pipe()
					t.Cleanup(func() { b.Close() })
					drain(b)
					return a, nil
				},
			})
			defer c.Close()
			ctx := context.Background()
			if tc.deadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
			}
			result := make(chan error, 1)
			go func() {
				_, err := c.Call[echoReply](ctx, "echo", (*echoArgs)(nil))
				result <- err
			}()
			if err := receive(t, result); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Call error = %v", err)
			}
		})
	}
}

func TestClientCanceledBeforeAndAfterDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a, b := net.Pipe()
	defer b.Close()
	var dials atomic.Int32
	c := NewClient("test", "address", ClientOptions{
		PoolSize: 1,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			cancel() // simulate cancellation concurrent with a successful dial
			return a, nil
		},
	})
	defer c.Close()
	canceled, cancelFirst := context.WithCancel(context.Background())
	cancelFirst()
	if _, err := c.Call[echoReply](canceled, "echo", (*echoArgs)(nil)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := c.Notify(canceled, "notify", (*echoArgs)(nil)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if dials.Load() != 0 {
		t.Fatal("canceled call started a dial")
	}
	if _, err := c.Call[echoReply](ctx, "echo", (*echoArgs)(nil)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.Close()
	if _, err := b.Read(make([]byte, 1)); err == nil {
		t.Fatal("late connection was not closed")
	}
}

func TestClientReconnectAndDialRetry(t *testing.T) {
	var dials atomic.Int32
	dialErr := errors.New("dial failed")
	c := NewClient("test", "address", ClientOptions{
		PoolSize: 1,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			if dials.Add(1) == 1 {
				return nil, dialErr
			}
			return echoTransport(), nil
		},
	})
	defer c.Close()
	if _, err := c.connection(context.Background()); !errors.Is(err, dialErr) {
		t.Fatal(err)
	}
	conn, err := c.connection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Break the transport rather than explicitly closing the pooled Conn.
	conn.rwc.Close()
	receive(t, conn.Done())
	reply, err := c.Call[echoReply](context.Background(), "echo", &echoArgs{Msg: "reconnected"})
	if err != nil || reply.Msg != "reconnected" || dials.Load() != 3 {
		t.Fatalf("reply = %v, err = %v, dials = %d", reply, err, dials.Load())
	}
	if err := c.Notify(context.Background(), "notify", &echoArgs{Msg: "hello"}); err != nil {
		t.Fatal(err)
	}
}

func TestClientCloseDuringDial(t *testing.T) {
	started := make(chan struct{}, 2)
	var dials atomic.Int32
	c := NewClient("test", "address", ClientOptions{
		PoolSize: 2,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dials.Add(1)
			started <- struct{}{}
			<-ctx.Done()
			// Even a transport returned after cancellation must be closed.
			a, b := net.Pipe()
			t.Cleanup(func() { b.Close() })
			return a, nil
		},
	})
	defer c.Close()
	results := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := c.connection(context.Background())
			results <- err
		}()
	}
	for range 2 {
		receive(t, started)
	}
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	receive(t, closed)
	for range 8 {
		if err := receive(t, results); !errors.Is(err, ErrClientClosed) {
			t.Fatal(err)
		}
	}
	if err := c.Notify(context.Background(), "notify", (*echoArgs)(nil)); !errors.Is(err, ErrClientClosed) {
		t.Fatal(err)
	}
	if dials.Load() != 2 {
		t.Fatalf("dials = %d, want 2", dials.Load())
	}
}

func TestClientCloseAllConnections(t *testing.T) {
	c := NewClient("test", "address", ClientOptions{
		PoolSize: 3,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return echoTransport(), nil
		},
	})
	defer c.Close()
	conns := make([]*Conn, 3)
	for i := range conns {
		var err error
		conns[i], err = c.connection(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { c.Close() })
	}
	wg.Wait()
	for _, conn := range conns {
		select {
		case <-conn.Done():
		default:
			t.Fatal("pooled connection is still open")
		}
	}
	if _, err := c.Call[echoReply](context.Background(), "echo", (*echoArgs)(nil)); !errors.Is(err, ErrClientClosed) {
		t.Fatal(err)
	}
}

func TestClientCloseFailsPendingCalls(t *testing.T) {
	started := make(chan struct{}, 3)
	c := NewClient("test", "address", ClientOptions{
		PoolSize: 3,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			a, b := net.Pipe()
			go func() {
				defer b.Close()
				for {
					if _, err := readFrame(b); err != nil {
						return
					}
					started <- struct{}{}
				}
			}()
			return a, nil
		},
	})
	defer c.Close()
	results := make(chan error, 3)
	for range 3 {
		go func() {
			_, err := c.Call[echoReply](context.Background(), "echo", (*echoArgs)(nil))
			results <- err
		}()
	}
	for range 3 {
		receive(t, started)
	}
	c.Close()
	for range 3 {
		if err := receive(t, results); err == nil {
			t.Fatal("pending call succeeded after closing client")
		}
	}
}

func TestClientTCP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go NewServer(&testHandler{}).Serve(ctx, l)
	c := NewClient("tcp", l.Addr().String(), ClientOptions{PoolSize: 2})
	defer c.Close()
	for range 4 {
		reply, err := c.Call[echoReply](ctx, "echo", &echoArgs{Msg: "tcp"})
		if err != nil || reply.Msg != "tcp" {
			t.Fatalf("reply = %v, err = %v", reply, err)
		}
	}
}
