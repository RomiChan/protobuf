package proto

import (
	"io"
	"unsafe"
)

var boolCodec = codec{
	size:   sizeOfBool,
	encode: encodeBool,
	decode: decodeBool,
}

func sizeOfBool(p unsafe.Pointer, flags flags) int {
	if *(*bool)(p) || flags.has(wantzero) {
		return 1
	}
	return 0
}

func encodeBool(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if *(*bool)(p) || flags.has(wantzero) {
		if *(*bool)(p) {
			b = append(b, 1)
		} else {
			b = append(b, 0)
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

var boolPtrCodec = codec{
	size:   sizeOfBoolPtr,
	encode: encodeBoolPtr,
	decode: decodeBoolPtr,
}

func sizeOfBoolPtr(p unsafe.Pointer, _ flags) int {
	p = deref(p)
	if p != nil {
		return 1
	}
	return 0
}

func encodeBoolPtr(b []byte, p unsafe.Pointer, _ flags) ([]byte, error) {
	p = deref(p)
	if p != nil {
		if *(*bool)(p) {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}
	return b, nil
}

func decodeBoolPtr(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v := (*unsafe.Pointer)(p)
	if *v == nil {
		*v = unsafe.Pointer(new(bool))
	}
	if len(b) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	*(*bool)(*v) = b[0] != 0
	return 1, nil
}
