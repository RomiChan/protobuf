package proto

import (
	"reflect"
	"sync"
	"unsafe"

	. "github.com/RomiChan/protobuf/internal/runtime_reflect"
)

var sliceMap sync.Map // map[*codec]*codec for slice

func sliceCodecOf(t reflect.Type, c *codec, w *walker) *codec {
	if loaded, ok := sliceMap.Load(c); ok {
		return loaded.(*codec)
	}
	if w.codecs[t] != nil {
		return w.codecs[t]
	}
	s := new(codec)
	w.codecs[t] = s

	s.size = sliceSizeFuncOf(t, c)
	s.encode = sliceEncodeFuncOf(t, c)
	s.decode = sliceDecodeFuncOf(t, c)

	actualCodec, _ := sliceMap.LoadOrStore(c, s)
	return actualCodec.(*codec)
}

func sliceSizeFuncOf(t reflect.Type, c *codec) sizeFunc {
	elemSize := alignedSize(t.Elem())
	return func(p unsafe.Pointer, sf *structField) int {
		n := 0
		if v := (*Slice)(p); v != nil {
			for i := 0; i < v.Len(); i++ {
				elem := v.Index(i, elemSize)
				n += c.size(elem, sf)
			}
		}
		return n
	}
}

func sliceEncodeFuncOf(t reflect.Type, c *codec) encodeFunc {
	elemSize := alignedSize(t.Elem())
	return func(b []byte, p unsafe.Pointer, sf *structField) []byte {
		if s := (*Slice)(p); s != nil {
			for i := 0; i < s.Len(); i++ {
				elem := s.Index(i, elemSize)
				b = c.encode(b, elem, sf)
			}
		}
		return b
	}
}

func sliceDecodeFuncOf(t reflect.Type, c *codec) decodeFunc {
	elemType := t.Elem()
	elemSize := alignedSize(elemType)
	return func(b []byte, p unsafe.Pointer) (int, error) {
		s := (*Slice)(p)
		i := s.Len()

		if i == s.Cap() {
			*s = growSlice(elemType, s)
		}

		n, err := c.decode(b, s.Index(i, elemSize))
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
