package proto

import (
	"io"
	"unsafe"
)

var boolCodec = codec{
	wire:   varint,
	size:   sizeOfBool,
	encode: encodeBool,
	decode: decodeBool,
}

func sizeOfBool(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if *(*bool)(p) || flags.has(wantzero) {
			return 1
		}
	}
	return 0
}

func encodeBool(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if *(*bool)(p) || flags.has(wantzero) {
			if *(*bool)(p) {
				b = append(b, 1)
			} else {
				b = append(b, 0)
			}
		}
	}
	return b, nil
}

func decodeBool(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	if len(b) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	*(*bool)(p) = b[0] != 0
	return 1, nil
}
