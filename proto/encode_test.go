package proto

import (
	"math"
	"testing"
)

func BenchmarkEncodeVarintShort(b *testing.B) {
	c := [10]byte{}

	for i := 0; i < b.N; i++ {
		appendVarint(c[:], 0)
	}
}

func BenchmarkEncodeVarintLong(b *testing.B) {
	c := [10]byte{}

	for i := 0; i < b.N; i++ {
		appendVarint(c[:], math.MaxUint64)
	}
}

func BenchmarkEncodeTag(b *testing.B) {
	c := [8]byte{}

	for i := 0; i < b.N; i++ {
		appendTag(c[:], 1, varint)
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

	size := Size(msg)
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		if _, err := Marshal(&msg); err != nil {
			b.Fatal(err)
		}
	}
}
