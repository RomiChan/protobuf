package proto

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestEncodeDecodeVarint(t *testing.T) {
	var b []byte

	b = appendVarint(b, 42)

	v, _, err := decodeVarint(b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 42 {
		t.Errorf("decoded value mismatch: want %d, got %d", 42, v)
	}
}

func TestEncodeDecodeVarintZigZag(t *testing.T) {
	var b []byte

	b = appendVarintZigZag(b, -42)
	v, _, err := decodeVarintZigZag(b)
	if err != nil {
		t.Fatal(err)
	}
	if v != -42 {
		t.Errorf("decoded value mismatch: want %d, got %d", -42, v)
	}
}

func TestEncodeDecodeTag(t *testing.T) {
	var b []byte

	b = appendTag(b, 1, varint)
	num, typ, _, err := decodeTag(b)
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

type key struct {
	Hi uint64 `protobuf:"varint,1,opt"`
	Lo uint64 `protobuf:"varint,2,opt"`
}

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

func TestMarshalUnmarshal(t *testing.T) {
	values := []interface{}{
		// bool
		true,
		false,

		// sfixed32
		int32(0),
		int32(math.MinInt32),
		int32(math.MaxInt32),

		// sfixed64
		int64(0),
		int64(math.MinInt64),
		int64(math.MaxInt64),

		// varint
		uint32(0),
		uint32(1),
		uint32(1234567890),

		// fixed32
		uint32(0),
		uint32(1234567890),

		// fixed64
		uint64(0),
		uint64(1234567890),

		// float
		float32(0),
		float32(math.Copysign(0, -1)),
		float32(0.1234),

		// double
		float64(0),
		float64(math.Copysign(0, -1)),
		float64(0.1234),

		// string
		"",
		"A",
		"Hello World!",

		// bytes
		([]byte)(nil),
		[]byte(""),
		[]byte("A"),
		[]byte("Hello World!"),

		// messages
		/*
			struct{ B bool }{B: false},
			struct{ B bool }{B: true},

			struct{ I int32 }{I: 0},
			struct{ I int32 }{I: 1},

			struct{ I32 int32 }{I32: 0},
			struct{ I32 int32 }{I32: -1234567890},

			struct{ I64 int64 }{I64: 0},
			struct{ I64 int64 }{I64: -1234567890},

			struct{ U int32 }{U: 0},
			struct{ U int32 }{U: 1},

			struct{ U32 uint32 }{U32: 0},
			struct{ U32 uint32 }{U32: 1234567890},

			struct{ U64 uint64 }{U64: 0},
			struct{ U64 uint64 }{U64: 1234567890},

			struct{ F32 float32 }{F32: 0},
			struct{ F32 float32 }{F32: 0.1234},

			struct{ F64 float64 }{F64: 0},
			struct{ F64 float64 }{F64: 0.1234},

			struct{ S string }{S: ""},
			struct{ S string }{S: "E"},

			struct{ B []byte }{B: nil},
			struct{ B []byte }{B: []byte{}},
			struct{ B []byte }{B: []byte{1, 2, 3}},
		*/
		&message{
			A: 1,
			B: 2,
			C: 3,
			S: &submessage{
				X: "hello",
				Y: "world",
			},
		},

		struct {
			Min int64 `protobuf:"zigzag64,1,opt"`
			Max int64 `protobuf:"zigzag64,2,opt"`
		}{Min: math.MinInt64, Max: math.MaxInt64},

		// pointers
		struct {
			M *message `protobuf:"bytes,1,opt"`
		}{M: nil},
		struct {
			M1 *message `protobuf:"bytes,1,opt"`
			M2 *message `protobuf:"bytes,2,opt"`
		}{
			M1: &message{A: 10, B: 100, C: 1000},
			M2: &message{S: &submessage{X: "42"}},
		},

		// byte arrays
		[0]byte{},
		[8]byte{},
		[16]byte{0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xA, 0xB, 0xC, 0xD, 0xE, 0xF},
		&[...]byte{},
		&[...]byte{3, 2, 1},

		// slices (repeated)
		struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: nil},
		struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: []int32{0}},
		struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: []int32{0, 0, 0}},
		struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: []int32{1, 2, 3}},
		struct {
			S []string `protobuf:"bytes,1,rep"`
		}{S: nil},
		struct {
			S []string `protobuf:"bytes,1,rep"`
		}{S: []string{""}},
		struct {
			S []string `protobuf:"bytes,1,rep"`
		}{S: []string{"A", "B", "C"}},
		struct {
			K []key `protobuf:"bytes,1,opt"`
		}{
			K: []key{
				{Hi: 0, Lo: 0},
				{Hi: 0, Lo: 1},
				{Hi: 0, Lo: 2},
				{Hi: 0, Lo: 3},
				{Hi: 0, Lo: 4},
			},
		},
	}

	for _, v := range values {
		t.Run(fmt.Sprintf("%T/%+v", v, v), func(t *testing.T) {
			n := Size(v)

			b, err := Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if n != len(b) {
				t.Fatalf("value size and buffer length mismatch (%d != %d) %v to %s", n, len(b), v, hex.EncodeToString(b))
			}

			p := reflect.New(reflect.TypeOf(v))
			if err := Unmarshal(b, p.Interface()); err != nil {
				t.Fatal(err)
			}

			x := p.Elem().Interface()
			if !reflect.DeepEqual(v, x) {
				t.Errorf("values mismatch:\nexpected: %#v\nfound:    %#v", v, x)
			}
		})
	}
}

func TestIssue106(t *testing.T) {
	m1 := struct {
		I uint32 `protobuf:"varint,1,opt"`
	}{I: ^uint32(0)}

	m2 := struct {
		I int32 `protobuf:"varint,1,opt"`
	}{}

	b, err := Marshal(&m1)
	if err != nil {
		t.Fatal(err)
	}

	if err := Unmarshal(b, &m2); err != nil {
		t.Fatal(err)
	}

	if m2.I != -1 {
		t.Error("unexpected value:", m2.I)
	}
}

func TestBoolPointer(t *testing.T) {
	type message struct {
		A *bool `protobuf:"varint,1,opt"`
	}
	var m message
	data, err := Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	var b message
	err = Unmarshal(data, &b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, b) {
		t.Fatalf("mismatch m!=b")
	}
}

func TestPrivateField(t *testing.T) {
	type private struct {
		a uint64 `protobuf:"varint,1,opt"`
		b uint64 `protobuf:"varint,2,opt"`
	}
	type public struct {
		A uint64 `protobuf:"varint,1,opt"`
		B uint64 `protobuf:"varint,2,opt"`
	}
	s := &private{1, 2}
	if Size(s) != 0 {
		t.Errorf("Size of private field should be zero.")
	}
	data, err := Marshal(s)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("Marshal of private field should be empty.")
	}

	data, err = Marshal(&public{2, 1})
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("Marshal of public field should not be empty.")
	}
	err = Unmarshal(data, s)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if s.a == 2 || s.b == 1 {
		t.Errorf("Unmarshal of private field: %v", s)
	}
}

func TestFixed(t *testing.T) {
	type message struct {
		Fixed32 uint32 `protobuf:"fixed32,1,opt"`
		Fixed64 uint64 `protobuf:"fixed64,2,opt"`
	}
	m := &message{
		Fixed32: 0x01020304,
		Fixed64: 0x0102030405060708,
	}
	if Size(m) != 14 { // 1+4+1+8
		t.Fatalf("Size of struct with fixed32 and fixed64 fields is not 14.")
	}
	b, err := Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	var m2 message
	if err := Unmarshal(b, &m2); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}
	if m2.Fixed32 != 0x01020304 {
		t.Errorf("m2.Fixed32 = %x, want 0x01020304", m2.Fixed32)
	}
	if m2.Fixed64 != 0x0102030405060708 {
		t.Errorf("m2.Fixed64 = %x, want 0x0102030405060708", m2.Fixed64)
	}
}

func TestFixedPointer(t *testing.T) {
	type message struct {
		Fixed32 *uint32 `protobuf:"fixed32,1,opt"`
		Fixed64 *uint64 `protobuf:"fixed64,2,opt"`
	}
	var fixed32 uint32 = 0x01020304
	var fixed64 uint64 = 0x0102030405060708
	m := &message{
		Fixed32: &fixed32,
		Fixed64: &fixed64,
	}
	if Size(m) != 14 { // 1+4+1+8
		t.Fatalf("Size of struct with fixed32 and fixed64 fields is not 14.")
	}
	b, err := Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	var m2 message
	if err := Unmarshal(b, &m2); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}
	if *m2.Fixed32 != 0x01020304 {
		t.Errorf("m2.Fixed32 = %x, want 0x01020304", m2.Fixed32)
	}
	if *m2.Fixed64 != 0x0102030405060708 {
		t.Errorf("m2.Fixed64 = %x, want 0x0102030405060708", m2.Fixed64)
	}
}
