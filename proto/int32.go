package proto

import (
	"unsafe"
)

var int32Codec = codec{
	size:   sizeOfInt32,
	encode: encodeInt32,
	decode: decodeInt32,
}

func sizeOfInt32(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*int32)(p); v != 0 || flags.has(wantzero) {
			return sizeOfVarint(flags.uint64(int64(v)))
		}
	}
	return 0
}

func encodeInt32(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*int32)(p); v != 0 || flags.has(wantzero) {
			b = appendVarint(b, flags.uint64(int64(v)))
		}
	}
	return b, nil
}

func decodeInt32(b []byte, p unsafe.Pointer, flags flags) (int, error) {
	u, n, err := decodeVarint(b)
	*(*int32)(p) = int32(flags.int64(u))
	return n, err
}
