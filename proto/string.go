package proto

import (
	"unsafe"
)

var stringCodec = codec{
	size:   sizeOfString,
	encode: encodeString,
	decode: decodeString,
}

func sizeOfString(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*string)(p); v != "" || flags.has(wantzero) {
			return sizeOfVarlen(len(v))
		}
	}
	return 0
}

func encodeString(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*string)(p); v != "" || flags.has(wantzero) {
			b = appendVarint(b, uint64(len(v)))
			b = append(b, v...)
		}
	}
	return b, nil
}

func decodeString(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeVarlen(b)
	*(*string)(p) = string(v)
	return n, err
}
