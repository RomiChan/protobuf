package proto

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"unsafe"
)

//go:generate go run ./gen/pointer
//go:generate go run ./gen/required

func Size(v interface{}) int {
	t, p := inspect(v)
	if t.Kind() != reflect.Ptr {
		panic(fmt.Errorf("proto.Marshal(%T): not a pointer", v))
	}
	t = t.Elem()
	info := cachedStructInfoOf(t)
	return info.size(p)
}

func Marshal(v interface{}) ([]byte, error) {
	t, p := inspect(v)
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("proto.Marshal(%T): not a pointer", v)
	}
	t = t.Elem()
	info := cachedStructInfoOf(t)
	b := make([]byte, 0, info.size(p))
	var err error
	b, err = info.encode(b, p)
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
	c := cachedStructInfoOf(t)

	n, err := c.decode(b, p)
	if err != nil {
		return err
	}
	if n < len(b) {
		return fmt.Errorf("proto.Unmarshal(%T): read=%d < buffer=%d", v, n, len(b))
	}
	return nil
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
		return "bytes"
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

var structInfoCache atomic.Value // map[unsafe.Pointer]*structInfo

func loadCachedStruct(t reflect.Type) (*structInfo, map[unsafe.Pointer]*structInfo) {
	cache, _ := structInfoCache.Load().(map[unsafe.Pointer]*structInfo)
	return cache[pointer(t)], cache
}

func storeCachedStruct(newCache map[unsafe.Pointer]*structInfo) {
	structInfoCache.Store(newCache)
}

func cachedStructInfoOf(t reflect.Type) *structInfo {
	c, oldCache := loadCachedStruct(t)
	if c != nil {
		return c
	}

	w := &walker{
		codecs: make(map[reflect.Type]*codec),
		infos:  make(map[reflect.Type]*structInfo),
	}

	newCache := make(map[unsafe.Pointer]*structInfo, len(oldCache)+1)
	for p, c := range oldCache {
		newCache[p] = c
	}

	info := w.structInfo(t)
	for t, info := range w.infos {
		newCache[pointer(t)] = info
	}

	storeCachedStruct(newCache)
	return info
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
