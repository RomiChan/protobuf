package proto

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/RomiChan/protobuf/proto/internal/testproto"
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
		&message{
			A: 1,
			B: 2,
			C: 3,
			S: &submessage{
				X: "hello",
				Y: "world",
			},
		},

		&struct {
			Min int64 `protobuf:"zigzag64,1,opt"`
			Max int64 `protobuf:"zigzag64,2,opt"`
		}{Min: math.MinInt64, Max: math.MaxInt64},

		// pointers
		&struct {
			M *message `protobuf:"bytes,1,opt"`
		}{M: nil},
		&struct {
			M1 *message `protobuf:"bytes,1,opt"`
			M2 *message `protobuf:"bytes,2,opt"`
		}{
			M1: &message{A: 10, B: 100, C: 1000},
			M2: &message{S: &submessage{X: "42"}},
		},

		// slices (repeated)
		&struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: nil},
		&struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: []int32{0}},
		&struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: []int32{0, 0, 0}},
		&struct {
			S []int32 `protobuf:"varint,1,rep"`
		}{S: []int32{1, 2, 3}},
		&struct {
			S []string `protobuf:"bytes,1,rep"`
		}{S: nil},
		&struct {
			S []string `protobuf:"bytes,1,rep"`
		}{S: []string{""}},
		&struct {
			S []string `protobuf:"bytes,1,rep"`
		}{S: []string{"A", "B", "C"}},
		&struct {
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

			p := reflect.New(reflect.TypeOf(v).Elem())
			if err := Unmarshal(b, p.Interface()); err != nil {
				t.Fatal(err)
			}

			x := p.Interface()
			if !reflect.DeepEqual(v, x) {
				t.Errorf("values mismatch:\nexpected: %#v\nfound:    %#v", v, x)
			}
		})
	}
}

func TestProto2(t *testing.T) {
	values := []*testproto.Proto2{
		{}, // nil
		{ // none-nil but default value
			BoolValue:  Bool(false),
			Int32Val:   Int32(0),
			Uint32Val:  Uint32(0),
			Int64Val:   Int64(0),
			Uint64Val:  Uint64(0),
			FloatVal:   Float32(0),
			DoubleVal:  Float64(0),
			StringVal:  String(""),
			BytesVal:   []byte{},
			Fixed32Val: Uint32(0),
			Fixed64Val: Uint64(0),
			Sint32Val:  Int32(0),
			Sint64Val:  Int64(0),
		},
		{
			BoolValue:  Bool(true),
			Int32Val:   Int32(1),
			Uint32Val:  Uint32(2),
			Int64Val:   Int64(3),
			Uint64Val:  Uint64(4),
			FloatVal:   Float32(114.514),
			DoubleVal:  Float64(1919.810),
			StringVal:  String("Hello World"),
			BytesVal:   make([]byte, 16),
			Fixed32Val: Uint32(5),
			Fixed64Val: Uint64(6),
			Sint32Val:  Int32(7),
			Sint64Val:  Int64(8),
		},
		{ // FIXME(wdvxdr)
			// Nested: &testproto.Proto2_NestedMessage{},
		},
		{
			Nested: &testproto.Proto2_NestedMessage{
				Int32Val:  Int32(0),
				Int64Val:  Int64(0),
				StringVal: String(""),
			},
		},
		{
			Nested: &testproto.Proto2_NestedMessage{
				Int32Val:  Int32(114514),
				Int64Val:  Int64(1919810),
				StringVal: String("Hello World!"),
			},
		},
	}

	for i, v := range values {
		t.Run(strconv.Itoa(i+1), func(t *testing.T) {
			n := Size(v)
			b, err := Marshal(v)
			assert.NoError(t, err)
			assert.Len(t, b, n)

			p := new(testproto.Proto2)
			assert.NoError(t, Unmarshal(b, &p))
			assert.Equal(t, v, p)
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
	assert.NoError(t, err)
	assert.NoError(t, Unmarshal(b, &m2))
	assert.Equal(t, m2.I, int32(-1))
}

func TestBoolPointer(t *testing.T) {
	type message struct {
		A *bool `protobuf:"varint,1,opt"`
	}
	var m message
	data, err := Marshal(&m)
	assert.NoError(t, err)

	var b message
	assert.NoError(t, Unmarshal(data, &b))
	assert.Equal(t, m, b)
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

func TestMap(t *testing.T) {
	type message struct {
		StrMap map[string]string `protobuf:"bytes,1,opt" protobuf_key:"bytes,1,opt" protobuf_val:"bytes,2,opt"`
		IntMap map[int64]int64   `protobuf:"bytes,2,opt" protobuf_key:"varint,1,opt" protobuf_val:"varint,2,opt"`
	}

	var mi = &message{
		StrMap: map[string]string{
			"":      "",
			"a":     "b",
			"hello": "world",
		},
		IntMap: map[int64]int64{
			0:    0,
			1:    1,
			114:  514,
			1919: 810,
		},
	}

	size := Size(mi)
	out, err := Marshal(mi)
	assert.NoError(t, err)
	assert.Equal(t, size, len(out))

	var mo message
	assert.NoError(t, Unmarshal(out, &mo))
	assert.Equal(t, mi, &mo)
}
