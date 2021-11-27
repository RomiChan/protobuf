package proto

import (
	"unsafe"
)

var uint32Codec = codec{
	size:   sizeOfUint32,
	encode: encodeUint32,
	decode: decodeUint32,
}

func sizeOfUint32(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*uint32)(p); v != 0 || flags.has(wantzero) {
			return sizeOfVarint(uint64(v))
		}
	}
	return 0
}

func encodeUint32(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*uint32)(p); v != 0 || flags.has(wantzero) {
			b = appendVarint(b, uint64(v))
		}
	}
	return b, nil
}

func decodeUint32(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeVarint(b)
	*(*uint32)(p) = uint32(v)
	return n, err
}

var fixed32Codec = codec{
	size:   sizeOfFixed32,
	encode: encodeFixed32,
	decode: decodeFixed32,
}

func sizeOfFixed32(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*uint32)(p); v != 0 || flags.has(wantzero) {
			return 4
		}
	}
	return 0
}

func encodeFixed32(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*uint32)(p); v != 0 || flags.has(wantzero) {
			b = encodeLE32(b, v)
		}
	}
	return b, nil
}

func decodeFixed32(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeLE32(b)
	*(*uint32)(p) = v
	return n, err
}
