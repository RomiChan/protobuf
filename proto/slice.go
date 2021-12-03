package proto

import (
	"reflect"
	"unsafe"

	. "github.com/RomiChan/protobuf/internal/runtime_reflect"
)

func sliceCodecOf(t reflect.Type, f structField, w *walker) *codec {
	s := new(codec)
	w.codecs[t] = s

	s.size = sliceSizeFuncOf(t, &f)
	s.encode = sliceEncodeFuncOf(t, &f)
	s.decode = sliceDecodeFuncOf(t, &f)
	return s
}

func sliceSizeFuncOf(t reflect.Type, f *structField) sizeFunc {
	elemSize := alignedSize(t.Elem())
	codec := f.codec
	return func(p unsafe.Pointer, sf *structField) int {
		n := 0
		if v := (*Slice)(p); v != nil {
			for i := 0; i < v.Len(); i++ {
				elem := v.Index(i, elemSize)
				n += codec.size(elem, sf)
			}
		}
		return n
	}
}

func sliceEncodeFuncOf(t reflect.Type, f *structField) encodeFunc {
	elemSize := alignedSize(t.Elem())
	codec := f.codec
	return func(b []byte, p unsafe.Pointer, sf *structField) ([]byte, error) {
		var err error
		if s := (*Slice)(p); s != nil {
			for i := 0; i < s.Len(); i++ {
				elem := s.Index(i, elemSize)
				b, err = codec.encode(b, elem, sf)
				if err != nil {
					return b, err
				}
			}
		}
		return b, nil
	}
}

func sliceDecodeFuncOf(t reflect.Type, f *structField) decodeFunc {
	elemType := t.Elem()
	elemSize := alignedSize(elemType)
	return func(b []byte, p unsafe.Pointer) (int, error) {
		s := (*Slice)(p)
		i := s.Len()

		if i == s.Cap() {
			*s = growSlice(elemType, s)
		}

		n, err := f.codec.decode(b, s.Index(i, elemSize))
		if err == nil {
			s.SetLen(i + 1)
		}
		return n, err
	}
}

func alignedSize(t reflect.Type) uintptr {
	a := t.Align()
	s := t.Size()
	return align(uintptr(a), s)
}

func align(align, size uintptr) uintptr {
	if align != 0 && (size%align) != 0 {
		size = ((size / align) + 1) * align
	}
	return size
}

func growSlice(t reflect.Type, s *Slice) Slice {
	cap := 2 * s.Cap()
	if cap == 0 {
		cap = 10
	}
	p := pointer(t)
	d := MakeSlice(p, s.Len(), cap)
	CopySlice(p, d, *s)
	return d
}
