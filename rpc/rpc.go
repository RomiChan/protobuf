package rpc

import (
	"context"

	"github.com/RomiChan/protobuf/proto"
)

// JSON-RPC 2.0 defined error codes.
const (
	// ParseError indicates invalid payload was received by the peer.
	ParseError = -32700
	// InvalidRequest indicates the payload was not a valid request.
	InvalidRequest = -32600
	// MethodNotFound indicates the method does not exist.
	MethodNotFound = -32601
	// InvalidParams indicates the request params failed to decode.
	InvalidParams = -32602
	// InternalError indicates an internal error on the peer, including
	// handler panics and result marshal failures.
	InternalError = -32603
)

// Error is the wire representation of an RPC error. It is returned by
// Call when the peer reports a failure, and may be returned by a Handler
// to control the error code and message sent back to the caller.
//
// Data, if non-empty, carries an opaque protobuf-encoded message whose
// concrete type is agreed upon by both peers.
type Error struct {
	Code    int64            `protobuf:"zigzag64,1,opt"`
	Message string           `protobuf:"bytes,2,opt"`
	Data    proto.RawMessage `protobuf:"bytes,3,opt"`
}

func (e *Error) Error() string { return e.Message }

// Request is an incoming RPC request delivered to a Handler.
type Request struct {
	// Method is the requested method name.
	Method string
	// Params is the raw protobuf-encoded params message. It aliases a
	// per-frame buffer that is never reused by the connection, so it
	// remains valid after Handle returns (see proto.RawMessage).
	Params proto.RawMessage
}

// UnmarshalParams decodes the request params into a new P, which must
// be a struct type with protobuf tags (not a pointer).
func (r *Request) UnmarshalParams[P any]() (P, error) {
	var params P
	err := proto.Unmarshal(r.Params, &params)
	return params, err
}

// Handler responds to RPC requests. The returned result, if non-nil,
// must be a pointer to a struct with protobuf tags and is marshaled
// back to the caller. Returning a nil result produces an empty result.
//
// Returning an *Error reports it to the caller with its code preserved;
// any other non-nil error is reported as InternalError.
type Handler interface {
	Handle(ctx context.Context, req *Request) (interface{}, error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, req *Request) (interface{}, error)

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, req *Request) (interface{}, error) {
	return f(ctx, req)
}

// HandlerOf adapts a strongly typed function to a Handler, eliminating
// the per-method params decoding boilerplate. P and R must be struct
// types with protobuf tags (not pointers). A params decoding failure is
// reported to the caller as InvalidParams.
//
//	add := rpc.HandlerOf(func(ctx context.Context, args *AddArgs) (*AddReply, error) {
//	    return &AddReply{Sum: args.A + args.B}, nil
//	})
//
// Combine it with a method switch or map to serve multiple methods:
//
//	srv := rpc.NewServer(rpc.HandlerFunc(func(ctx context.Context, req *rpc.Request) (interface{}, error) {
//	    if req.Method == "Add" {
//	        return add.Handle(ctx, req)
//	    }
//	    return nil, &rpc.Error{Code: rpc.MethodNotFound, Message: "unknown method: " + req.Method}
//	}))
func HandlerOf[P, R any](fn func(ctx context.Context, params *P) (*R, error)) Handler {
	return HandlerFunc(func(ctx context.Context, req *Request) (interface{}, error) {
		params, err := req.UnmarshalParams[P]()
		if err != nil {
			return nil, &Error{Code: InvalidParams, Message: err.Error()}
		}
		reply, err := fn(ctx, &params)
		if reply == nil {
			return nil, err
		}
		return reply, err
	})
}
