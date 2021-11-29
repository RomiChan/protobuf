package proto

import (
	"unsafe"
)

var bytesCodec = codec{
	size:   sizeOfBytes,
	encode: encodeBytes,
	decode: decodeBytes,
}

func sizeOfBytes(p unsafe.Pointer, _ flags) int {
	if p != nil {
		if v := *(*[]byte)(p); v != nil {
			return sizeOfVarlen(len(v))
		}
	}
	return 0
}

func encodeBytes(b []byte, p unsafe.Pointer, _ flags) ([]byte, error) {
	if p != nil {
		if v := *(*[]byte)(p); v != nil {
			b = appendVarint(b, uint64(len(v)))
			b = append(b, v...)
		}
	}
	return b, nil
}

func decodeBytes(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeVarlen(b)
	pb := (*[]byte)(p)
	if *pb == nil {
		*pb = make([]byte, 0, len(v))
	}
	*pb = append((*pb)[:0], v...)
	return n, err
}

func makeBytes(p unsafe.Pointer, n int) []byte {
	return *(*[]byte)(unsafe.Pointer(&sliceHeader{
		Data: p,
		Len:  n,
		Cap:  n,
	}))
}

type sliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}
