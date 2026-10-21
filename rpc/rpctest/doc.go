// Package rpctest provides in-memory servers for testing RPC handlers and
// application clients. Requests use the real RPC framing, protobuf encoding,
// connection pool, and dispatch code over net.Pipe.
//
// NewServer registers cleanup with testing.TB. Its Client method creates pooled
// clients, while DialContext can be injected into application-specific clients.
// CloseClientConnections allows tests to simulate a server disconnect without
// shutting down the server.
//
// # Concurrent tests and timeouts
//
// Servers work with testing/synctest. Create the server, clients, and all related
// goroutines inside the same synctest.Test callback. The fake clock can then
// advance through production timeouts without waiting in real time:
//
//	func TestTimeout(t *testing.T) {
//	    synctest.Test(t, func(t *testing.T) {
//	        srv := rpctest.NewServer(t, rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
//	            <-ctx.Done()
//	            return nil, ctx.Err()
//	        }))
//	        client := srv.Client(rpc.ClientOptions{CallTimeout: 210 * time.Second})
//	        _, err := client.Call[Reply](t.Context(), "Slow", (*Args)(nil))
//	        if !errors.Is(err, context.DeadlineExceeded) {
//	            t.Fatalf("Call: %v", err)
//	        }
//	    })
//	}
//
// RPC call cancellation is local to the caller; it is not sent to the handler.
// The handler above is released when server cleanup cancels its connection
// context. Handlers must honor that context or otherwise arrange to finish.
//
// Tests inside the rpc package that import rpctest must use package rpc_test to
// avoid an import cycle. White-box tests can remain in package rpc.
package rpctest
