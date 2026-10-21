package proto

import (
	"math"
	"testing"
)

type message struct {
	A int32       `protobuf:"varint,1,opt"`
	B int32       `protobuf:"varint,2,opt"`
	C int32       `protobuf:"varint,3,opt"`
	S *submessage `protobuf:"bytes,4,opt"`
}

type submessage struct {
	X string `protobuf:"bytes,1,opt"`
	Y string `protobuf:"bytes,2,opt"`
}

func BenchmarkEncodeVarintShort(b *testing.B) {
	c := [10]byte{}

	for i := 0; i < b.N; i++ {
		writeVarint(c[:], 0, 0)
	}
}

func BenchmarkEncodeVarintLong(b *testing.B) {
	c := [10]byte{}

	for i := 0; i < b.N; i++ {
		writeVarint(c[:], 0, math.MaxUint64)
	}
}

func BenchmarkEncodeTag(b *testing.B) {
	c := [8]byte{}

	for i := 0; i < b.N; i++ {
		writeTag(c[:], 0, 1, varint)
	}
}

func BenchmarkEncodeMessage(b *testing.B) {
	msg := &message{
		A: 1,
		B: 100,
		C: 10000,
		S: &submessage{
			X: "",
			Y: "Hello World!",
		},
	}

	size := Size(msg)
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		if _, err := Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMap(b *testing.B) {
	msg := struct {
		M map[string]string `protobuf:"bytes,1,opt" protobuf_key:"bytes,1,opt" protobuf_val:"bytes,2,opt"`
	}{
		M: map[string]string{
			"hello": "world",
		},
	}

	size := Size(&msg)
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		if _, err := Marshal(&msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeSlice(b *testing.B) {
	msg := struct {
		S []int32 `protobuf:"varint,1,rep"`
	}{
		S: []int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	}

	size := Size(&msg)
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		if _, err := Marshal(&msg); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEncodeDecodeVarint(t *testing.T) {
	b := make([]byte, 10)
	n := writeVarint(b, 0, 42)

	v, _, err := decodeVarint(b[:n])
	if err != nil {
		t.Fatal(err)
	}
	if v != 42 {
		t.Errorf("decoded value mismatch: want %d, got %d", 42, v)
	}
}

func TestEncodeDecodeVarintZigZag(t *testing.T) {
	b := make([]byte, 10)
	n := writeVarintZigZag(b, 0, -42)

	v, _, err := decodeVarintZigZag(b[:n])
	if err != nil {
		t.Fatal(err)
	}
	if v != -42 {
		t.Errorf("decoded value mismatch: want %d, got %d", -42, v)
	}
}

func TestEncodeDecodeTag(t *testing.T) {
	b := make([]byte, 10)
	n := writeTag(b, 0, 1, varint)

	num, typ, _, err := decodeTag(b[:n])
	if err != nil {
		t.Fatal(err)
	}
	if num != 1 {
		t.Errorf("decoded field number mismatch: want %d, got %d", 1, num)
	}
	if typ != varint {
		t.Errorf("decoded wire type mismatch: want %d, got %d", varint, typ)
	}
}
