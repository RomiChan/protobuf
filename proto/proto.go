package proto

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/RomiChan/syncx"
)

//go:generate go run ./gen/option
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
	b = info.encode(b, p)
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

var structInfoCache syncx.Map[unsafe.Pointer, *structInfo] // map[unsafe.Pointer]*structInfo
var codecCache sync.Map                                    // map[reflect.Type]codec

func cachedStructInfoOf(t reflect.Type) *structInfo {
	c, ok := structInfoCache.Load(pointer(t))
	if ok {
		return c
	}

	w := &walker{
		codecs: make(map[reflect.Type]*codec),
		infos:  make(map[reflect.Type]*structInfo),
	}

	info := w.structInfo(t)
	actual, _ := structInfoCache.LoadOrStore(pointer(t), info)
	return actual
}
