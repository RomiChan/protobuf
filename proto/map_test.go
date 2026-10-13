package proto_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"

	. "github.com/RomiChan/protobuf/proto"
)

type mapKey string
type mapEnum int32
type namedMap map[mapKey]mapEnum

type mapNode struct {
	Name     string              `protobuf:"bytes,1,opt"`
	Children map[string]*mapNode `protobuf:"bytes,2,rep" protobuf_key:"bytes,1,opt" protobuf_val:"bytes,2,opt"`
}

func checkMapRoundTrip(t *testing.T, input any) {
	t.Helper()
	data, err := Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if size := Size(input); size != len(data) {
		t.Fatalf("Size = %d, encoded length = %d", size, len(data))
	}
	output := reflect.New(reflect.TypeOf(input).Elem()).Interface()
	if err := Unmarshal(data, output); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, output) {
		t.Fatalf("round trip mismatch: got %#v, want %#v", output, input)
	}
}

func TestMapReflectRoundTrip(t *testing.T) {
	t.Run("named types and growth", func(t *testing.T) {
		input := &struct {
			Values namedMap `protobuf:"bytes,1,rep" protobuf_key:"bytes,1,opt" protobuf_val:"varint,2,opt"`
		}{Values: namedMap{"": 0}}
		for i := 0; i < 256; i++ {
			input.Values[mapKey(fmt.Sprint(i))] = mapEnum(i - 128)
		}
		checkMapRoundTrip(t, input)
	})
	t.Run("bytes and bool keys", func(t *testing.T) {
		checkMapRoundTrip(t, &struct {
			Values map[bool][]byte `protobuf:"bytes,1,rep" protobuf_key:"varint,1,opt" protobuf_val:"bytes,2,opt"`
		}{Values: map[bool][]byte{false: nil, true: {1, 2, 3}}})
	})
	t.Run("recursive message values", func(t *testing.T) {
		input := &mapNode{Name: "root", Children: map[string]*mapNode{
			"nil": nil,
			"a": {Name: "a", Children: map[string]*mapNode{
				"leaf": {Name: "leaf"},
			}},
			"b": {Name: "b"},
		}}
		checkMapRoundTrip(t, input)
		// Reuse the same cached codec concurrently, including recursive calls.
		for i := 0; i < 8; i++ {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				for j := 0; j < 20; j++ {
					checkMapRoundTrip(t, input)
				}
			})
		}
	})
}

func TestMapNilAndEmpty(t *testing.T) {
	type message struct {
		Values map[string]string `protobuf:"bytes,1,rep" protobuf_key:"bytes,1,opt" protobuf_val:"bytes,2,opt"`
	}
	for _, values := range []map[string]string{nil, {}} {
		input := &message{Values: values}
		data, err := Marshal(input)
		if err != nil || len(data) != 0 || Size(input) != 0 {
			t.Fatalf("empty map: data=%x, err=%v", data, err)
		}
		var output message
		if err := Unmarshal(data, &output); err != nil || output.Values != nil {
			t.Fatalf("empty input: output=%v, err=%v", output, err)
		}
	}
}

func TestMapZigzagWire(t *testing.T) {
	// The same Go map type can use different protobuf encodings.
	input := &struct {
		Plain map[int32]int32 `protobuf:"bytes,1,rep" protobuf_key:"varint,1,opt" protobuf_val:"varint,2,opt"`
		Zig32 map[int32]int32 `protobuf:"bytes,2,rep" protobuf_key:"zigzag32,1,opt" protobuf_val:"zigzag32,2,opt"`
		Zig64 map[int64]int64 `protobuf:"bytes,3,rep" protobuf_key:"zigzag64,1,opt" protobuf_val:"zigzag64,2,opt"`
	}{
		Plain: map[int32]int32{1: 2},
		Zig32: map[int32]int32{-1: -2},
		Zig64: map[int64]int64{-2: -3},
	}
	want := []byte{0x0a, 4, 8, 1, 16, 2, 0x12, 4, 8, 1, 16, 3, 0x1a, 4, 8, 3, 16, 5}
	data, err := Marshal(input)
	if err != nil || !bytes.Equal(data, want) {
		t.Fatalf("encoded %x, want %x; err=%v", data, want, err)
	}
	checkMapRoundTrip(t, input)
}

func TestMapEntryWire(t *testing.T) {
	type message struct {
		Values map[int32]int32 `protobuf:"bytes,1,rep" protobuf_key:"varint,1,opt" protobuf_val:"varint,2,opt"`
	}
	for _, tc := range []struct {
		name string
		data []byte
		want map[int32]int32
	}{
		{"empty entry", []byte{0x0a, 0}, map[int32]int32{0: 0}},
		{"missing key", []byte{0x0a, 2, 16, 7}, map[int32]int32{0: 7}},
		{"missing value", []byte{0x0a, 2, 8, 7}, map[int32]int32{7: 0}},
		{"reverse order", []byte{0x0a, 4, 16, 7, 8, 2}, map[int32]int32{2: 7}},
		{"duplicate key", []byte{0x0a, 4, 8, 2, 16, 7, 0x0a, 4, 8, 2, 16, 9}, map[int32]int32{2: 9}},
		{"duplicate fields", []byte{0x0a, 8, 8, 1, 8, 2, 16, 7, 16, 9}, map[int32]int32{2: 9}},
		{"reset entry", []byte{0x0a, 4, 8, 2, 16, 7, 0x0a, 0}, map[int32]int32{2: 7, 0: 0}},
		{"unknown fields", []byte{0x0a, 21, 24, 1, 34, 1, 42, 45, 0, 0, 0, 0, 49, 0, 0, 0, 0, 0, 0, 0, 0, 8, 2}, map[int32]int32{2: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := message{Values: map[int32]int32{99: 100}}
			if err := Unmarshal(tc.data, &output); err != nil {
				t.Fatal(err)
			}
			tc.want[99] = 100 // Unmarshal preserves unrelated existing entries.
			if !reflect.DeepEqual(output.Values, tc.want) {
				t.Fatalf("got %v, want %v", output.Values, tc.want)
			}
		})
	}
}

func TestMapFixedWire(t *testing.T) {
	input := &struct {
		Fixed32 map[int32]uint32 `protobuf:"bytes,1,rep" protobuf_key:"fixed32,1,opt" protobuf_val:"fixed32,2,opt"`
		Fixed64 map[uint64]int64 `protobuf:"bytes,2,rep" protobuf_key:"fixed64,1,opt" protobuf_val:"fixed64,2,opt"`
	}{
		Fixed32: map[int32]uint32{-1: 0},
		Fixed64: map[uint64]int64{0: -1},
	}
	want := []byte{
		0x0a, 10, 13, 255, 255, 255, 255, 21, 0, 0, 0, 0,
		0x12, 18, 9, 0, 0, 0, 0, 0, 0, 0, 0, 17, 255, 255, 255, 255, 255, 255, 255, 255,
	}
	data, err := Marshal(input)
	if err != nil || !bytes.Equal(data, want) {
		t.Fatalf("encoded %x, want %x; err=%v", data, want, err)
	}
	checkMapRoundTrip(t, input)
}

func TestMapEntryErrorRecovery(t *testing.T) {
	type message struct {
		Values map[string]string `protobuf:"bytes,1,rep" protobuf_key:"bytes,1,opt" protobuf_val:"bytes,2,opt"`
	}
	for _, tc := range []struct {
		name string
		data []byte
		eof  bool
	}{
		{"truncated entry", []byte{0x0a, 4, 0x0a, 1, 'k'}, true},
		{"truncated value", []byte{0x0a, 5, 0x0a, 1, 'k', 0x12, 1}, true},
		{"wrong wire", []byte{0x0a, 2, 8, 1}, false},
		{"truncated tag", []byte{0x0a, 1, 0x80}, true},
		{"truncated unknown bytes", []byte{0x0a, 5, 0x0a, 1, 'k', 0x1a, 1}, true},
		{"truncated unknown varint", []byte{0x0a, 2, 24, 0x80}, true},
		{"truncated unknown fixed32", []byte{0x0a, 2, 29, 0}, true},
		{"truncated unknown fixed64", []byte{0x0a, 2, 25, 0}, true},
		{"invalid wire", []byte{0x0a, 1, 31}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := message{Values: map[string]string{"keep": "value"}}
			err := Unmarshal(tc.data, &output)
			if err == nil || tc.eof && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(output.Values) != 1 || output.Values["keep"] != "value" {
				t.Fatalf("failed entry modified map: %v", output.Values)
			}
			if err := Unmarshal([]byte{0x0a, 0}, &output); err != nil {
				t.Fatal(err)
			}
			if value, ok := output.Values[""]; !ok || value != "" || len(output.Values) != 2 {
				t.Fatalf("stale pooled entry after error: %v", output.Values)
			}
		})
	}
}

func TestMapElementTagPanics(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input any
	}{
		{"missing tags", &struct {
			Values map[string]string `protobuf:"bytes,1,rep"`
		}{}},
		{"invalid key tag", &struct {
			Values map[string]string `protobuf:"bytes,1,rep" protobuf_key:"bogus,1,opt" protobuf_val:"bytes,2,opt"`
		}{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			Marshal(tc.input)
		})
	}
}

func TestMapMessageFieldBoundary(t *testing.T) {
	type message struct {
		Values map[string]*mapNode `protobuf:"bytes,1,rep" protobuf_key:"bytes,1,opt" protobuf_val:"bytes,2,opt"`
	}
	// Value precedes key; the nested message must not consume the key field.
	data := []byte{0x0a, 8, 0x12, 3, 0x0a, 1, 'v', 0x0a, 1, 'k'}
	var output message
	if err := Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	want := map[string]*mapNode{"k": {Name: "v"}}
	if !reflect.DeepEqual(output.Values, want) {
		t.Fatalf("got %v, want %v", output.Values, want)
	}
}
