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
	wiretag    uint64
	keyFlags   uint8
	valFlags   uint8
	keyWireTag uint64
	keyCodec   *codec
	valWireTag uint64
	valCodec   *codec
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
	const (
		keyTagSize = 1 // sizeOfTag(1, f.keyCodec.wire)
		valTagSize = 1 // sizeOfTag(2, f.valCodec.wire)
	)
	return func(p unsafe.Pointer) int {
		if p == nil {
			return 0
		}

		p = *(*unsafe.Pointer)(p)

		n := 0
		m := MapIter{}
		defer m.Done()

		for m.Init(pointer(t), p); m.HasNext(); m.Next() {
			keySize := f.keyCodec.size(m.Key())
			valSize := f.valCodec.size(m.Value())

			if keySize > 0 {
				n += keyTagSize + keySize
				if (f.keyFlags & embedded) != 0 {
					n += sizeOfVarint(uint64(keySize))
				}
			}

			if valSize > 0 {
				n += valTagSize + valSize
				if (f.valFlags & embedded) != 0 {
					n += sizeOfVarint(uint64(valSize))
				}
			}

			n += mapTagSize + sizeOfVarint(uint64(keySize+valSize))
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

	return func(b []byte, p unsafe.Pointer) ([]byte, error) {
		if p == nil {
			return b, nil
		}
		p = *(*unsafe.Pointer)(p)

		origLen := len(b)
		var err error

		m := MapIter{}
		defer m.Done()

		for m.Init(pointer(t), p); m.HasNext(); m.Next() {
			key := m.Key()
			val := m.Value()

			keySize := f.keyCodec.size(key)
			valSize := f.valCodec.size(val)
			elemSize := keySize + valSize

			if keySize > 0 {
				elemSize += 1 // keyTagSize
				if (f.keyFlags & embedded) != 0 {
					elemSize += sizeOfVarint(uint64(keySize))
				}
			}

			if valSize > 0 {
				elemSize += 1 // valTagSize
				if (f.valFlags & embedded) != 0 {
					elemSize += sizeOfVarint(uint64(valSize))
				}
			}

			b = append(b, mapTag...)
			b = appendVarint(b, uint64(elemSize))

			if keySize > 0 {
				b = appendVarint(b, f.keyWireTag)

				if (f.keyFlags & embedded) != 0 {
					b = appendVarint(b, uint64(keySize))
				}

				b, err = f.keyCodec.encode(b, key)
				if err != nil {
					return b, err
				}
			}

			if valSize > 0 {
				b = appendVarint(b, f.valWireTag)

				if (f.valFlags & embedded) != 0 {
					b = appendVarint(b, uint64(valSize))
				}

				b, err = f.valCodec.encode(b, val)
				if err != nil {
					return b, err
				}
			}
		}

		if len(b) == origLen {
			b = append(b, zero...)
		}
		return b, nil
	}
}

func formatWireTag(wire uint64) reflect.StructTag {
	return reflect.StructTag(fmt.Sprintf(`protobuf:"%s,%d,opt"`, wireType(wire&7), wire>>3))
}

func mapDecodeFuncOf(t reflect.Type, m *mapField, w *walker) decodeFunc {
	structType := reflect.StructOf([]reflect.StructField{
		{Name: "Key", Type: t.Key(), Tag: formatWireTag(m.keyWireTag)},
		{Name: "Elem", Type: t.Elem(), Tag: formatWireTag(m.valWireTag)},
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

		n, err := info.decode(b, s)
		if err == nil {
			v := MapAssign(mtype, *m, s)
			Assign(vtype, v, unsafe.Pointer(uintptr(s)+valueOffset))
		}

		Assign(stype, s, structZero)
		structPool.Put(s)
		return n, err
	}
}
