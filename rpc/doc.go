// Package rpc implements a lightweight JSON-RPC 2.0 style RPC protocol
// that uses github.com/RomiChan/protobuf/proto as its serialization
// format. It provides both client and server sides over any
// io.ReadWriteCloser (typically a net.Conn), with support for
// concurrent in-flight calls, notifications, and request cancellation
// via context.
//
// # Wire protocol
//
// Each message on the wire is a 4-byte big-endian length prefix followed
// by a protobuf-encoded frame:
//
//	message Frame {
//	    optional int64 id = 1;     // present for requests and responses
//	    string method = 2;         // non-empty for requests/notifications
//	    bytes  params = 3;         // protobuf-encoded params message
//	    bytes  result = 4;         // protobuf-encoded result message
//	    Error  error  = 5;         // present on failure
//	}
//
// A frame with a non-empty method and an id is a request; with a method
// but no id it is a notification (never answered); with no method it is
// a response, correlated to a request by id. Params and result carry
// opaque protobuf-encoded messages whose concrete types are agreed upon
// by client and server, which keeps the envelope fixed while payloads
// stay freely definable. Frames are capped at 16 MiB.
//
// # Messages
//
// Params and result types are ordinary Go structs with protobuf tags,
// exactly as produced by protoc-gen-golite or written by hand:
//
//	type AddArgs struct {
//	    A int64 `protobuf:"varint,1,opt"`
//	    B int64 `protobuf:"varint,2,opt"`
//	}
//
//	type AddReply struct {
//	    Sum int64 `protobuf:"varint,1,opt"`
//	}
//
// # Server
//
// A server implements the Handler interface, typically by switching on
// the request method. Params are decoded with Request.UnmarshalParams,
// and the returned value (a pointer to a protobuf-tagged struct, or nil)
// is marshaled back to the caller:
//
//	handler := rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
//	    switch req.Method {
//	    case "Add":
//	        args, err := req.UnmarshalParams[AddArgs]()
//	        if err != nil {
//	            return nil, &rpc.Error{Code: rpc.InvalidParams, Message: err.Error()}
//	        }
//	        return &AddReply{Sum: args.A + args.B}, nil
//	    }
//	    return nil, &rpc.Error{Code: rpc.MethodNotFound, Message: "unknown method: " + req.Method}
//	})
//
// Serve it on a listener:
//
//	go rpc.ListenAndServe(ctx, "tcp", ":9000", handler)
//
// or manage the listener yourself with NewServer and Serve, or serve a
// single accepted connection with Server.ServeConn.
//
// Handlers are invoked concurrently, one goroutine per request, so they
// must be safe for concurrent use. A handler panic is recovered and
// reported to the caller as an InternalError.
//
// The generic HandlerOf adapter removes the params decoding boilerplate
// for strongly typed handlers:
//
//	add := rpc.HandlerOf(func(ctx context.Context, args *AddArgs) (*AddReply, error) {
//	    return &AddReply{Sum: args.A + args.B}, nil
//	})
//
// # Client
//
// Create a lazy connection pool for concurrent calls:
//
//	client := rpc.NewClient("tcp", "127.0.0.1:9000", rpc.ClientOptions{
//	    PoolSize:     8,
//	    DialTimeout:  5 * time.Second,
//	    CallTimeout:  210 * time.Second, // application policy; disabled by default
//	    WriteTimeout: 5 * time.Second,
//	})
//	defer client.Close()
//
//	reply, err := client.Call[AddReply](ctx, "Add", &AddArgs{A: 1, B: 2})
//
// The zero ClientOptions uses eight round-robin slots, five-second dial and
// write timeouts, and no default call timeout. Connections are created on first
// use and reused; each supports multiple concurrent requests. Concurrent callers
// selecting the same empty slot share one dial, while different slots dial in
// parallel. Each caller can cancel its own wait. A shared dial is bounded by the
// initiating caller's context, DialTimeout, and Client.Close; canceling the
// initiating caller can therefore fail other callers sharing that dial.
//
// CallTimeout includes connection acquisition and respects earlier caller
// deadlines. It also applies to Client.Notify. Broken connections are discarded
// on the next use of their slot and dialed again. Failed calls are returned to
// the caller without replaying them, since the peer may have already executed
// the request. Method names, request construction, and application-specific type
// conversions belong in application clients wrapping Client.
//
// For a single connection, dial a server directly:
//
//	conn, err := rpc.Dial(ctx, "tcp", "127.0.0.1:9000")
//	if err != nil {
//	    // dial failed
//	}
//	defer conn.Close()
//
//	reply, err := conn.Call[AddReply](ctx, "Add", &AddArgs{A: 1, B: 2})
//
// The generic Call method decodes the response into a new AddReply
// directly; the params type is inferred from the argument. For methods
// without parameters, pass a typed nil:
//
//	reply, err = conn.Call[AddReply](ctx, "Get", (*AddArgs)(nil))
//
// Call blocks until the response arrives, the connection breaks, or ctx
// is canceled. Multiple goroutines may Call and Notify on the same Conn
// concurrently.
//
// Notifications carry no id and are never answered:
//
//	conn.Notify(ctx, "Log", &LogEntry{Msg: "hello"})
//
// To use a transport other than TCP, wrap any io.ReadWriteCloser with
// NewConn (client side) or Server.ServeConn (server side).
//
// Conn sets a fresh five-second deadline for each write when the transport
// supports SetWriteDeadline, including net.Conn. NewConn and Dial accept optional
// ConnOptions to configure WriteTimeout; a negative value disables it. Earlier
// call deadlines take priority, cancellation can interrupt an active write, and
// write failures close the transport and fail all pending calls. Canceling while
// waiting for the write lock does not close the connection. Other transports
// must provide their own means of bounding blocked writes.
//
// # Testing
//
// The rpc/rpctest subpackage provides in-memory servers with automatic test
// cleanup, pooled clients, and connection interruption for recovery tests. It
// works with testing/synctest to test timeouts without waiting in real time.
//
// # Errors
//
// When a call fails on the peer, Call returns a non-nil error of type
// *Error carrying the peer's code and message:
//
//	var rerr *rpc.Error
//	if errors.As(err, &rerr) && rerr.Code == rpc.MethodNotFound {
//	    // the server does not implement this method
//	}
//
// A handler may return an *Error to control the code and message; any
// other non-nil error is reported as InternalError with err.Error() as
// the message. The predefined codes ParseError, InvalidRequest,
// MethodNotFound, InvalidParams, and InternalError follow JSON-RPC 2.0;
// application-defined codes may use any other value.
//
// # Lifecycle
//
// Client.Close prevents further dialing, cancels active dials, wakes connection
// waiters with ErrClientClosed, and closes every connection in the pool. It is
// safe to call concurrently or repeatedly.
//
// Conn.Close shuts a connection down: pending calls fail immediately and
// in-flight handler goroutines are waited for. Conn.Done is closed when
// the connection terminates for any reason, including the peer going
// away. Canceling the context passed to Serve or ListenAndServe closes
// the listener; existing connections are unaffected.
package rpc
