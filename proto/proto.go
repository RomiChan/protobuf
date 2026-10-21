package proto

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/RomiChan/protobuf/internal/vm"
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
	return info.sizeNoTape(p)
}

func Marshal(v interface{}) ([]byte, error) {
	t, p := inspect(v)
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("proto.Marshal(%T): not a pointer", v)
	}
	t = t.Elem()
	info := cachedStructInfoOf(t)

	// The size pass records nested message sizes on a tape so the encode
	// pass can replay them instead of recomputing each subtree.
	var tape *vm.Tape
	if info.hasMessage {
		tape = tapePool.Get().(*vm.Tape)
		tape.Reset()
	}
	var size int
	if tape != nil {
		size = info.sizeTape(p, tape)
	} else {
		size = info.sizeNoTape(p)
	}
	b := make([]byte, size)
	info.encode(b, 0, p, tape)
	if tape != nil {
		tapePool.Put(tape)
	}
	return b, nil
}

var tapePool = sync.Pool{New: func() any { return new(vm.Tape) }}

func Unmarshal(b []byte, v interface{}) error {
	if len(b) == 0 {
		// nothing to do
		return nil
	}

	t, p := inspect(v)
	if t.Kind() != reflect.Pointer || p == nil {
		return &InvalidUnmarshalError{Type: t}
	}
	elem := t.Elem()
	if elem.Kind() != reflect.Struct {
		return &InvalidUnmarshalError{Type: t}
	}
	c := cachedStructInfoOf(elem)

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
	// sizeT and encodeT are the tape-aware variants, installed for values
	// that carry nested messages: struct codecs, and the slice codecs built
	// on them. They are nil for scalar codecs. Both are always installed
	// together, so the size pass and the encode pass agree on which values
	// are recorded on the tape.
	sizeT   tapeSizeFunc
	encodeT tapeEncodeFunc
	// isMessage marks a struct codec, i.e. a value encoded as a nested
	// message. Slice codecs use it to tell message elements from scalar ones.
	isMessage bool
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
