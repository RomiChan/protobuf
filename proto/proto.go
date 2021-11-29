package proto

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"unsafe"
)

func Size(v interface{}) int {
	t, p := inspect(v)
	if t.Kind() != reflect.Ptr {
		panic(fmt.Errorf("proto.Marshal(%T): not a pointer", v))
	}
	t = t.Elem()
	c := cachedCodecOf(t)
	return c.size(p, noflags)
}

func Marshal(v interface{}) ([]byte, error) {
	t, p := inspect(v)
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("proto.Marshal(%T): not a pointer", v)
	}
	t = t.Elem()
	c := cachedCodecOf(t)
	b := make([]byte, 0, c.size(p, noflags))
	var err error
	b, err = c.encode(b, p, noflags)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal(%T): %w", v, err)
	}
	return b, nil
}

func Unmarshal(b []byte, v interface{}) error {
	if len(b) == 0 {
		// nothing to do
		return nil
	}

	t, p := inspect(v)
	t = t.Elem() // Unmarshal must be passed a pointer
	c := cachedCodecOf(t)

	n, err := c.decode(b, p, noflags)
	if err != nil {
		return err
	}
	if n < len(b) {
		return fmt.Errorf("proto.Unmarshal(%T): read=%d < buffer=%d", v, n, len(b))
	}
	return nil
}

type flags uintptr

const (
	noflags  flags = 0
	wantzero flags = 1 << 1
	// Shared with structField.flags in struct.go:
	// zigzag flags = 1 << 2
)

func (f flags) has(x flags) bool {
	return (f & x) != 0
}

func (f flags) with(x flags) flags {
	return f | x
}

func (f flags) without(x flags) flags {
	return f & ^x
}

func (f flags) uint64(i int64) uint64 {
	if f.has(zigzag) {
		return encodeZigZag64(i)
	} else {
		return uint64(i)
	}
}

func (f flags) int64(u uint64) int64 {
	if f.has(zigzag) {
		return decodeZigZag64(u)
	} else {
		return int64(u)
	}
}

type iface struct {
	typ unsafe.Pointer
	ptr unsafe.Pointer
}

func inspect(v interface{}) (reflect.Type, unsafe.Pointer) {
	return reflect.TypeOf(v), pointer(v)
}

func pointer(v interface{}) unsafe.Pointer {
	return (*iface)(unsafe.Pointer(&v)).ptr
}

type fieldNumber uint

type wireType uint

const (
	varint  wireType = 0
	fixed64 wireType = 1
	varlen  wireType = 2
	fixed32 wireType = 5
)

func (wt wireType) String() string {
	switch wt {
	case varint:
		return "varint"
	case varlen:
		return "varlen"
	case fixed32:
		return "fixed32"
	case fixed64:
		return "fixed64"
	default:
		return "unknown"
	}
}

type codec struct {
	size   sizeFunc
	encode encodeFunc
	decode decodeFunc
}

var codecCache atomic.Value // map[unsafe.Pointer]*codec

func loadCachedCodec(t reflect.Type) (*codec, map[unsafe.Pointer]*codec) {
	cache, _ := codecCache.Load().(map[unsafe.Pointer]*codec)
	return cache[pointer(t)], cache
}

func storeCachedCodec(newCache map[unsafe.Pointer]*codec) {
	codecCache.Store(newCache)
}

func cachedCodecOf(t reflect.Type) *codec {
	c, oldCache := loadCachedCodec(t)
	if c != nil {
		return c
	}

	var p reflect.Type
	isPtr := t.Kind() == reflect.Ptr
	if isPtr {
		p = t
		t = t.Elem()
	} else {
		p = reflect.PtrTo(t)
	}

	seen := make(map[reflect.Type]*codec)
	c1 := codecOf(t, seen, false)
	c2 := codecOf(p, seen, false)

	newCache := make(map[unsafe.Pointer]*codec, len(oldCache)+2)
	for p, c := range oldCache {
		newCache[p] = c
	}

	newCache[pointer(t)] = c1
	newCache[pointer(p)] = c2
	storeCachedCodec(newCache)

	if isPtr {
		return c2
	} else {
		return c1
	}
}

func codecOf(t reflect.Type, seen map[reflect.Type]*codec, zigzag bool) *codec {
	if c := seen[t]; c != nil {
		return c
	}

	switch t.Kind() {
	case reflect.Bool:
		return &boolCodec
	case reflect.Int32:
		if zigzag {
			return &zigzag32Codec
		}
		return &int32Codec
	case reflect.Int64:
		if zigzag {
			return &zigzag64Codec
		}
		return &int64Codec
	case reflect.Uint32:
		return &uint32Codec
	case reflect.Uint64:
		return &uint64Codec
	case reflect.Float32:
		return &float32Codec
	case reflect.Float64:
		return &float64Codec
	case reflect.String:
		return &stringCodec
	case reflect.Slice:
		elem := t.Elem()
		switch elem.Kind() {
		case reflect.Uint8:
			return &bytesCodec
		}
	case reflect.Struct:
		return structCodecOf(t, seen)
	case reflect.Ptr:
		return pointerCodecOf(t, seen, zigzag)
	}

	panic("unsupported type: " + t.String())
}

// Bool stores v in a new bool value and returns a pointer to it.
func Bool(v bool) *bool { return &v }

// Int32 stores v in a new int32 value and returns a pointer to it.
func Int32(v int32) *int32 { return &v }

// Int64 stores v in a new int64 value and returns a pointer to it.
func Int64(v int64) *int64 { return &v }

// Float32 stores v in a new float32 value and returns a pointer to it.
func Float32(v float32) *float32 { return &v }

// Float64 stores v in a new float64 value and returns a pointer to it.
func Float64(v float64) *float64 { return &v }

// Uint32 stores v in a new uint32 value and returns a pointer to it.
func Uint32(v uint32) *uint32 { return &v }

// Uint64 stores v in a new uint64 value and returns a pointer to it.
func Uint64(v uint64) *uint64 { return &v }

// String stores v in a new string value and returns a pointer to it.
func String(v string) *string { return &v }
