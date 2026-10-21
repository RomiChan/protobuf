package proto

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"
)

type mapField struct {
	wiretag  uint64
	keyField *structField
	valField *structField
}

// Map elements are not addressable. Reuse typed storage so the pointer-based
// codecs can read entries without allocating a key and value on every iteration.
type mapEntry struct {
	key    reflect.Value
	elem   reflect.Value
	keyPtr unsafe.Pointer
	valPtr unsafe.Pointer
}

func (e *mapEntry) clear() {
	e.key.SetZero()
	e.elem.SetZero()
}

func mapElementTag(f reflect.StructField, name string) structTag {
	tag, ok := f.Tag.Lookup(name)
	if !ok {
		panic(fmt.Sprintf("proto: map field %s is missing the %s tag", f.Name, name))
	}
	t, err := parseStructTag(tag)
	if err != nil {
		panic(err)
	}
	return t
}

func (w *walker) mapElementCodec(t reflect.Type, tag structTag) *codec {
	// Integer map elements can use fixed-width wire encoding as well as varints.
	// Both encoding and decoding must use the codec selected by the field tag.
	switch tag.wireType {
	case fixed32:
		if t.Kind() == reflect.Int32 || t.Kind() == reflect.Uint32 {
			return &fixed32RequiredCodec
		}
	case fixed64:
		if t.Kind() == reflect.Int64 || t.Kind() == reflect.Uint64 {
			return &fixed64RequiredCodec
		}
	}
	return w.codec(t, &walkerConfig{required: true, zigzag: tag.zigzag})
}

func (w *walker) mapCodec(t reflect.Type, f *mapField) *codec {
	m := new(codec)
	w.codecs[t] = m

	pool := &sync.Pool{New: func() any {
		key := reflect.New(t.Key()).Elem()
		elem := reflect.New(t.Elem()).Elem()
		return &mapEntry{
			key: key, elem: elem,
			keyPtr: key.Addr().UnsafePointer(),
			valPtr: elem.Addr().UnsafePointer(),
		}
	}}

	m.size = mapSizeFuncOf(t, f, pool)
	m.encode = mapEncodeFuncOf(t, f, pool)
	m.decode = mapDecodeFuncOf(t, f, pool)
	return m
}

func mapSizeFuncOf(t reflect.Type, f *mapField, pool *sync.Pool) sizeFunc {
	mapTagSize := sizeOfVarint(f.wiretag)
	keyCodec := f.keyField.codec
	valCodec := f.valField.codec

	return func(p unsafe.Pointer, sf *structField) int {
		if p == nil {
			return 0
		}
		mv := reflect.NewAt(t, p).Elem()
		if mv.Len() == 0 {
			return 0
		}

		entry := pool.Get().(*mapEntry)
		defer func() {
			entry.clear()
			pool.Put(entry)
		}()
		n := 0
		var iter reflect.MapIter
		iter.Reset(mv)

		for iter.Next() {
			entry.key.SetIterKey(&iter)
			entry.elem.SetIterValue(&iter)
			keySize := keyCodec.size(entry.keyPtr, f.keyField)
			valSize := valCodec.size(entry.valPtr, f.valField)
			n += mapTagSize + sizeOfVarint(uint64(keySize+valSize)) + keySize + valSize
		}
		return n
	}
}

func mapEncodeFuncOf(t reflect.Type, f *mapField, pool *sync.Pool) encodeFunc {
	mapTag := make([]byte, sizeOfVarint(f.wiretag))
	writeVarint(mapTag, 0, f.wiretag)
	keyCodec := f.keyField.codec
	valCodec := f.valField.codec

	return func(b []byte, i int, p unsafe.Pointer, sf *structField) int {
		if p == nil {
			return i
		}
		mv := reflect.NewAt(t, p).Elem()
		if mv.Len() == 0 {
			return i
		}

		entry := pool.Get().(*mapEntry)
		defer func() {
			entry.clear()
			pool.Put(entry)
		}()
		var iter reflect.MapIter
		iter.Reset(mv)

		for iter.Next() {
			entry.key.SetIterKey(&iter)
			entry.elem.SetIterValue(&iter)
			key, val := entry.keyPtr, entry.valPtr

			keySize := keyCodec.size(key, f.keyField)
			valSize := valCodec.size(val, f.valField)
			elemSize := keySize + valSize

			i += copy(b[i:], mapTag)
			i = writeVarint(b, i, uint64(elemSize))
			i = keyCodec.encode(b, i, key, f.keyField)
			i = valCodec.encode(b, i, val, f.valField)
		}
		return i
	}
}

func mapDecodeFuncOf(t reflect.Type, m *mapField, pool *sync.Pool) decodeFunc {
	return func(b []byte, p unsafe.Pointer) (int, error) {
		mv := reflect.NewAt(t, p).Elem()
		if mv.IsNil() {
			mv.Set(reflect.MakeMapWithSize(t, 10))
		}

		data, n, err := decodeVarlen(b)
		if err != nil {
			return n, err
		}
		entry := pool.Get().(*mapEntry)
		defer func() {
			entry.clear()
			pool.Put(entry)
		}()
		read, err := m.decodeEntry(data, entry)
		if err != nil {
			return n - len(data) + read, err
		}
		mv.SetMapIndex(entry.key, entry.elem)
		return n, nil
	}
}

func (m *mapField) decodeEntry(b []byte, entry *mapEntry) (int, error) {
	offset := 0
	for offset < len(b) {
		number, wire, n, err := decodeTag(b[offset:])
		offset += n
		if err != nil {
			return offset, err
		}

		var f *structField
		var p unsafe.Pointer
		switch number {
		case m.keyField.fieldNumber():
			f, p = m.keyField, entry.keyPtr
		case m.valField.fieldNumber():
			f, p = m.valField, entry.valPtr
		}
		if f == nil {
			// Skip unknown fields.
			switch wire {
			case varint:
				_, n, err = decodeVarint(b[offset:])
			case varlen:
				_, n, err = decodeVarlen(b[offset:])
			case fixed32:
				_, n, err = decodeLE32(b[offset:])
			case fixed64:
				_, n, err = decodeLE64(b[offset:])
			default:
				err = ErrWireTypeUnknown
			}
			if err != nil {
				return offset, fieldError(number, wire, err)
			}
			offset += n
			continue
		}

		if wire != f.wireType() {
			return offset, fieldError(number, wire, fmt.Errorf("expected wire type %d", f.wireType()))
		}

		// Use the original codec so zigzag and named types are preserved. The
		// codec consumes exactly its own bytes (message values consume their
		// own length prefix), so no pre-bounding pass is needed.
		read, err := f.codec.decode(b[offset:], p)
		offset += read
		if err != nil {
			return offset, fieldError(number, wire, err)
		}
	}
	return offset, nil
}
