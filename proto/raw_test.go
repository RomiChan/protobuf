package proto

import (
	"testing"
	"unsafe"
)

type rawInner struct {
	Msg string `protobuf:"bytes,1,opt"`
}

type rawEnvelope struct {
	Kind int32      `protobuf:"varint,1,opt"`
	Body RawMessage `protobuf:"bytes,2,opt"`
}

func TestRawMessageField(t *testing.T) {
	body, err := Marshal(&rawInner{Msg: "hello"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	data, err := Marshal(&rawEnvelope{Kind: 7, Body: body})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var env rawEnvelope
	if err := Unmarshal(data, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Kind != 7 {
		t.Fatalf("env.Kind = %d, want 7", env.Kind)
	}
	var inner rawInner
	if err := Unmarshal(env.Body, &inner); err != nil {
		t.Fatalf("Unmarshal body: %v", err)
	}
	if inner.Msg != "hello" {
		t.Fatalf("inner.Msg = %q, want %q", inner.Msg, "hello")
	}
}

func TestRawMessageZeroCopy(t *testing.T) {
	data, err := Marshal(&rawEnvelope{Kind: 1, Body: RawMessage("hello")})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var env rawEnvelope
	if err := Unmarshal(data, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(env.Body) != "hello" {
		t.Fatalf("env.Body = %q, want %q", env.Body, "hello")
	}
	lo := uintptr(unsafe.Pointer(&data[0]))
	hi := lo + uintptr(len(data))
	p := uintptr(unsafe.Pointer(&env.Body[0]))
	if p < lo || p >= hi {
		t.Fatal("decoded RawMessage does not alias the input buffer")
	}
}

func TestRawMessageNil(t *testing.T) {
	data, err := Marshal(&rawEnvelope{Kind: 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var env rawEnvelope
	if err := Unmarshal(data, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Body != nil {
		t.Fatalf("env.Body = %v, want nil", env.Body)
	}
}

func TestRawMessageInteropWithBytes(t *testing.T) {
	// RawMessage and []byte fields with the same tag are wire-compatible.
	type bytesEnvelope struct {
		Kind int32  `protobuf:"varint,1,opt"`
		Body []byte `protobuf:"bytes,2,opt"`
	}
	src, err := Marshal(&rawEnvelope{Kind: 3, Body: RawMessage("abc")})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var dst bytesEnvelope
	if err := Unmarshal(src, &dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(dst.Body) != "abc" || dst.Kind != 3 {
		t.Fatalf("dst = %+v", dst)
	}
}
