package rpctest_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/RomiChan/protobuf/rpc"
	"github.com/RomiChan/protobuf/rpc/rpctest"
)

type message struct {
	Text string `protobuf:"bytes,1,opt"`
}

func echoHandler() rpc.Handler {
	return rpc.HandlerOf(func(ctx context.Context, args *message) (*message, error) {
		return &message{Text: args.Text}, nil
	})
}

func TestCallAndNotify(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		notified := make(chan string, 1)
		srv := rpctest.NewServer(t, rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
			if req.Method == "notify" {
				args, err := req.UnmarshalParams[message]()
				if err != nil {
					return nil, err
				}
				notified <- args.Text
				return nil, nil
			}
			return echoHandler().Handle(ctx, req)
		}))
		client := srv.Client()
		for i := range 16 {
			text := fmt.Sprint(i)
			reply, err := client.Call[message](t.Context(), "echo", &message{Text: text})
			if err != nil || reply.Text != text {
				t.Fatalf("Call = %v, %v", reply, err)
			}
		}
		if err := client.Notify(t.Context(), "notify", &message{Text: "notification"}); err != nil {
			t.Fatal(err)
		}
		if got := <-notified; got != "notification" {
			t.Fatal(got)
		}
	})
}

func TestConcurrentCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{}, 16)
		release := make(chan struct{})
		srv := rpctest.NewServer(t, rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
			entered <- struct{}{}
			select {
			case <-release:
				return echoHandler().Handle(ctx, req)
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}))
		// All handlers must enter before any returns, even on a single slot.
		client := srv.Client(rpc.ClientOptions{PoolSize: 1})
		results := make(chan error, 16)
		for i := range 16 {
			go func() {
				text := fmt.Sprint(i)
				reply, err := client.Call[message](t.Context(), "echo", &message{Text: text})
				if err == nil && reply.Text != text {
					err = errors.New("mismatched reply")
				}
				results <- err
			}()
		}
		for range 16 {
			<-entered
		}
		close(release)
		for range 16 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestDialContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := rpctest.NewServer(t, echoHandler())
		ctx, cancel := context.WithCancel(t.Context())
		transport, err := srv.DialContext(ctx, "application-network", "application-address")
		if err != nil {
			t.Fatal(err)
		}
		defer transport.Close()
		cancel() // dial context does not control an established connection
		conn := rpc.NewConn(transport)
		defer conn.Close()
		reply, err := conn.Call[message](t.Context(), "echo", &message{Text: "custom dialer"})
		if err != nil || reply.Text != "custom dialer" {
			t.Fatalf("Call = %v, %v", reply, err)
		}
		if _, err := srv.DialContext(ctx, "", ""); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled dial = %v", err)
		}
	})
}

func TestClientOptionsAndTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := rpctest.NewServer(t, rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}))
		client := srv.Client(rpc.ClientOptions{
			CallTimeout: 210 * time.Second,
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				t.Error("Client did not replace the dialer")
				return nil, errors.New("unexpected dial")
			},
		})
		start := time.Now()
		if _, err := client.Call[message](t.Context(), "slow", (*message)(nil)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Call = %v", err)
		}
		if elapsed := time.Since(start); elapsed != 210*time.Second {
			t.Fatalf("elapsed = %v, want 210s", elapsed)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		start = time.Now()
		if _, err := client.Call[message](ctx, "slow", (*message)(nil)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Call = %v", err)
		}
		if elapsed := time.Since(start); elapsed != 5*time.Second {
			t.Fatalf("elapsed = %v, want 5s", elapsed)
		}
	})
}

func TestDisconnectAndReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{}, 1)
		finished := make(chan struct{}, 1)
		srv := rpctest.NewServer(t, rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
			if req.Method == "slow" {
				started <- struct{}{}
				<-ctx.Done()
				finished <- struct{}{}
				return nil, ctx.Err()
			}
			return echoHandler().Handle(ctx, req)
		}))
		client := srv.Client(rpc.ClientOptions{PoolSize: 1})
		result := make(chan error, 1)
		go func() {
			_, err := client.Call[message](t.Context(), "slow", (*message)(nil))
			result <- err
		}()
		<-started
		srv.CloseClientConnections()
		<-finished
		if err := <-result; err == nil {
			t.Fatal("disconnected call succeeded")
		}
		synctest.Wait()
		reply, err := client.Call[message](t.Context(), "echo", &message{Text: "reconnected"})
		if err != nil || reply.Text != "reconnected" {
			t.Fatalf("Call after disconnect = %v, %v", reply, err)
		}
	})
}

func TestPeerCloseCancelsHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		finished := make(chan struct{})
		srv := rpctest.NewServer(t, rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
			close(started)
			<-ctx.Done()
			close(finished)
			return nil, ctx.Err()
		}))
		client := srv.Client()
		go client.Call[message](t.Context(), "slow", (*message)(nil))
		<-started
		client.Close()
		synctest.Wait()
		select {
		case <-finished:
		default:
			t.Fatal("closing peer did not cancel handler")
		}
	})
}

func TestClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{}, 2)
		var exited atomic.Int32
		srv := rpctest.NewServer(t, rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
			started <- struct{}{}
			<-ctx.Done()
			exited.Add(1)
			return nil, ctx.Err()
		}))
		clients := []*rpc.Client{srv.Client(), srv.Client()}
		results := make(chan error, 2)
		for _, client := range clients {
			go func() {
				_, err := client.Call[message](t.Context(), "slow", (*message)(nil))
				results <- err
			}()
		}
		for range 2 {
			<-started
		}
		transport, err := srv.DialContext(t.Context(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		srv.Close()
		srv.Close()
		if exited.Load() != 2 {
			t.Fatal("Close returned before handlers exited")
		}
		for range 2 {
			if err := <-results; err == nil {
				t.Fatal("pending call succeeded after Close")
			}
		}
		if _, err := transport.Read(make([]byte, 1)); err == nil {
			t.Fatal("unmanaged connection is still open")
		}
		if _, err := srv.DialContext(t.Context(), "", ""); !errors.Is(err, net.ErrClosed) {
			t.Fatalf("dial after Close = %v", err)
		}
		for _, client := range append(clients, srv.Client()) {
			if _, err := client.Call[message](t.Context(), "echo", (*message)(nil)); !errors.Is(err, rpc.ErrClientClosed) {
				t.Fatalf("client after Close = %v", err)
			}
		}
	})
}

func TestAutomaticCleanup(t *testing.T) {
	var srv *rpctest.Server
	var client *rpc.Client
	var transport net.Conn
	t.Run("server", func(t *testing.T) {
		srv = rpctest.NewServer(t, echoHandler())
		client = srv.Client()
		var err error
		transport, err = srv.DialContext(t.Context(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Call[message](t.Context(), "echo", &message{Text: "cleanup"}); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := client.Call[message](t.Context(), "echo", (*message)(nil)); !errors.Is(err, rpc.ErrClientClosed) {
		t.Fatalf("client after cleanup = %v", err)
	}
	if _, err := transport.Read(make([]byte, 1)); err == nil {
		t.Fatal("transport is still open after cleanup")
	}
	if _, err := srv.DialContext(t.Context(), "", ""); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("dial after cleanup = %v", err)
	}
}

func TestConcurrentLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := rpctest.NewServer(t, echoHandler())
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				conn, err := srv.DialContext(t.Context(), "", "")
				if err == nil {
					conn.Close()
				} else if !errors.Is(err, net.ErrClosed) {
					t.Error(err)
				}
			})
			wg.Go(func() { srv.Client().Close() })
			wg.Go(srv.CloseClientConnections)
		}
		for range 4 {
			wg.Go(srv.Close)
		}
		wg.Wait()
	})
}

func TestNilHandler(t *testing.T) {
	srv := rpctest.NewServer(t, nil)
	_, err := srv.Client().Call[message](t.Context(), "missing", (*message)(nil))
	var remote *rpc.Error
	if !errors.As(err, &remote) || remote.Code != rpc.MethodNotFound {
		t.Fatalf("Call = %v, want MethodNotFound", err)
	}
}
