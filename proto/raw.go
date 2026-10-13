package proto

import (
	"reflect"
	"unsafe"
)

// RawMessage is a raw encoded protobuf message, analogous to
// encoding/json.RawMessage. Used as a field type with a bytes wire tag,
// it stores an encoded message verbatim, delaying (or avoiding)
// decoding of that message:
//
//	type Envelope struct {
//	    Kind int32      `protobuf:"varint,1,opt"`
//	    Body RawMessage `protobuf:"bytes,2,opt"`
//	}
//
// A nil RawMessage encodes as an absent field.
//
// WARNING: decoding a RawMessage is zero-copy. Unlike []byte fields,
// whose contents are copied during Unmarshal, a RawMessage aliases the
// buffer passed to Unmarshal. The value must not be retained beyond
// that buffer's lifetime, nor used after the buffer is modified or
// reused; copy it first (e.g. with slices.Clone) when in doubt.
type RawMessage []byte

var rawMessageType = reflect.TypeOf(RawMessage(nil))

var rawCodec = codec{
	size:   sizeOfRaw,
	encode: encodeRaw,
	decode: decodeRaw,
}

func sizeOfRaw(p unsafe.Pointer, f *structField) int {
	v := *(*RawMessage)(p)
	if v != nil {
		return sizeOfVarlen(len(v)) + f.tagsize
	}
	return 0
}

func encodeRaw(b []byte, p unsafe.Pointer, f *structField) []byte {
	v := *(*RawMessage)(p)
	if v != nil {
		b = appendVarint(b, f.wiretag)
		b = appendVarint(b, uint64(len(v)))
		b = append(b, v...)
	}
	return b
}

func decodeRaw(b []byte, p unsafe.Pointer) (int, error) {
	v, n, err := decodeVarlen(b)
	if err != nil {
		return n, err
	}
	*(*RawMessage)(p) = RawMessage(v)
	return n, nil
}
