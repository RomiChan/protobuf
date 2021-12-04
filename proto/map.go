package proto

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	. "github.com/RomiChan/protobuf/internal/runtime_reflect"
)

const zeroSize = 1 // sizeOfVarint(0)

type mapField struct {
	wiretag  uint64
	keyField *structField
	valField *structField
}

func (w *walker) mapCodec(t reflect.Type, f *mapField) *codec {
	m := new(codec)
	w.codecs[t] = m

	m.size = mapSizeFuncOf(t, f)
	m.encode = mapEncodeFuncOf(t, f)
	m.decode = mapDecodeFuncOf(t, f, w)
	return m
}

func mapSizeFuncOf(t reflect.Type, f *mapField) sizeFunc {
	mapTagSize := sizeOfVarint(f.wiretag)
	keyCodec := f.keyField.codec
	valCodec := f.valField.codec

	return func(p unsafe.Pointer, sf *structField) int {
		if p == nil {
			return 0
		}

		p = *(*unsafe.Pointer)(p)

		n := 0
		m := MapIter{}
		defer m.Done()

		for m.Init(pointer(t), p); m.HasNext(); m.Next() {
			keySize := keyCodec.size(m.Key(), f.keyField)
			valSize := valCodec.size(m.Value(), f.valField)
			n += mapTagSize + sizeOfVarint(uint64(keySize+valSize)) + keySize + valSize
		}
		if n == 0 {
			n = mapTagSize + zeroSize
		}
		return n
	}
}

func mapEncodeFuncOf(t reflect.Type, f *mapField) encodeFunc {
	mapTag := appendVarint(nil, f.wiretag)
	zero := append(mapTag, 0)
	keyCodec := f.keyField.codec
	valCodec := f.valField.codec

	return func(b []byte, p unsafe.Pointer, sf *structField) []byte {
		if p == nil {
			return b
		}
		p = *(*unsafe.Pointer)(p)

		origLen := len(b)

		m := MapIter{}
		defer m.Done()

		for m.Init(pointer(t), p); m.HasNext(); m.Next() {
			key := m.Key()
			val := m.Value()

			keySize := keyCodec.size(key, f.keyField)
			valSize := keyCodec.size(val, f.valField)
			elemSize := keySize + valSize

			b = append(b, mapTag...)
			b = appendVarint(b, uint64(elemSize))
			b = keyCodec.encode(b, key, f.keyField)
			b = valCodec.encode(b, val, f.valField)
		}

		if len(b) == origLen {
			b = append(b, zero...)
		}
		return b
	}
}

func formatWireTag(wire uint64) reflect.StructTag {
	return reflect.StructTag(fmt.Sprintf(`protobuf:"%s,%d,opt"`, wireType(wire&7), wire>>3))
}

func mapDecodeFuncOf(t reflect.Type, m *mapField, w *walker) decodeFunc {
	structType := reflect.StructOf([]reflect.StructField{
		{Name: "Key", Type: t.Key(), Tag: formatWireTag(m.keyField.wiretag)},
		{Name: "Elem", Type: t.Elem(), Tag: formatWireTag(m.valField.wiretag)},
	})

	info := w.structInfo(structType)
	structPool := new(sync.Pool)
	structZero := pointer(reflect.Zero(structType).Interface())

	valueType := t.Elem()
	valueOffset := structType.Field(1).Offset

	mtype := pointer(t)
	stype := pointer(structType)
	vtype := pointer(valueType)

	return func(b []byte, p unsafe.Pointer) (int, error) {
		m := (*unsafe.Pointer)(p)
		if *m == nil {
			*m = MakeMap(mtype, 10)
		}
		if len(b) == 0 {
			return 0, nil
		}

		s := pointer(structPool.Get())
		if s == nil {
			s = unsafe.Pointer(reflect.New(structType).Pointer())
		}

		_, nl, err := decodeVarint(b)
		if err != nil {
			return 0, err
		}
		n, err := info.decode(b[nl:], s)
		if err == nil {
			v := MapAssign(mtype, *m, s)
			Assign(vtype, v, unsafe.Pointer(uintptr(s)+valueOffset))
		}
		Assign(stype, s, structZero)
		structPool.Put(s)
		return n + nl, err
	}
}
