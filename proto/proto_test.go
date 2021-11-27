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
	Hi uint64
	Lo uint64
}

type message struct {
	A int32
	B int32
	C int32
	S submessage
}

type submessage struct {
	X string
	Y string
}

type structWithMap struct {
	M map[int32]string
}

type custom [16]byte

func (c *custom) Size() int { return len(c) }

func (c *custom) MarshalTo(b []byte) (int, error) {
	return copy(b, c[:]), nil
}

func (c *custom) Unmarshal(b []byte) error {
	copy(c[:], b)
	return nil
}

type messageWithCustomField struct {
	Custom custom
}

func TestMarshalUnmarshal(t *testing.T) {
	intVal := int32(42)
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

		&message{
			A: 1,
			B: 2,
			C: 3,
			S: submessage{
				X: "hello",
				Y: "world",
			},
		},

		struct {
			Min int64 `protobuf:"zigzag64,1,opt,name=min,proto3"`
			Max int64 `protobuf:"zigzag64,2,opt,name=min,proto3"`
		}{Min: math.MinInt64, Max: math.MaxInt64},

		// pointers
		struct{ M *message }{M: nil},
		struct {
			M1 *message
			M2 *message
			M3 *message
		}{
			M1: &message{A: 10, B: 100, C: 1000},
			M2: &message{S: submessage{X: "42"}},
		},

		// byte arrays
		[0]byte{},
		[8]byte{},
		[16]byte{0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xA, 0xB, 0xC, 0xD, 0xE, 0xF},
		&[...]byte{},
		&[...]byte{3, 2, 1},

		// slices (repeated)
		struct{ S []int32 }{S: nil},
		struct{ S []int32 }{S: []int32{0}},
		struct{ S []int32 }{S: []int32{0, 0, 0}},
		struct{ S []int32 }{S: []int32{1, 2, 3}},
		struct{ S []string }{S: nil},
		struct{ S []string }{S: []string{""}},
		struct{ S []string }{S: []string{"A", "B", "C"}},
		struct{ K []key }{
			K: []key{
				{Hi: 0, Lo: 0},
				{Hi: 0, Lo: 1},
				{Hi: 0, Lo: 2},
				{Hi: 0, Lo: 3},
				{Hi: 0, Lo: 4},
			},
		},

		// maps (repeated)
		struct{ M map[int32]string }{},
		struct{ M map[int32]string }{
			M: map[int32]string{0: ""},
		},
		struct{ M map[int32]string }{
			M: map[int32]string{0: "A", 1: "B", 2: "C"},
		},
		&struct{ M map[int32]string }{
			M: map[int32]string{0: "A", 1: "B", 2: "C"},
		},
		struct {
			M1 map[int32]int32
			M2 map[string]string
			M3 map[string]message
			M4 map[string]*message
			M5 map[key]uint32
		}{
			M1: map[int32]int32{0: 1},
			M2: map[string]string{"": "A"},
			M3: map[string]message{
				"m0": {},
				"m1": {A: 42},
				"m3": {S: submessage{X: "X", Y: "Y"}},
			},
			M4: map[string]*message{
				"m0": {},
				"m1": {A: 42},
				"m3": {S: submessage{X: "X", Y: "Y"}},
			},
			M5: map[key]uint32{
				key{Hi: 0, Lo: 0}: 0,
				key{Hi: 1, Lo: 0}: 1,
				key{Hi: 0, Lo: 1}: 2,
				key{Hi: math.MaxUint64, Lo: math.MaxUint64}: 3,
			},
		},

		// more complex inlined types use cases
		struct{ I *int32 }{},
		struct{ I *int32 }{I: new(int32)},
		struct{ I *int32 }{I: &intVal},
		struct{ M *message }{},
		struct{ M *message }{M: new(message)},
		struct{ M map[int32]int32 }{},
		struct{ M map[int32]int32 }{M: map[int32]int32{}},
		struct{ S structWithMap }{
			S: structWithMap{
				M: map[int32]string{0: "A", 1: "B", 2: "C"},
			},
		},
		&struct{ S structWithMap }{
			S: structWithMap{
				M: map[int32]string{0: "A", 1: "B", 2: "C"},
			},
		},

		// custom messages
		custom{},
		custom{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		messageWithCustomField{
			Custom: custom{1: 42},
		},
		struct {
			A int32
			B string
			C custom
		}{A: 42, B: "Hello World!", C: custom{1: 42}},
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
		I uint32
	}{I: ^uint32(0)}

	m2 := struct {
		I int32
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
