package proto

import (
	"unsafe"
)

var stringCodec = codec{
	size:   sizeOfString,
	encode: encodeString,
	decode: decodeString,
}

func sizeOfString(p unsafe.Pointer, _ flags) int {
	v := *(*string)(p)
	if v != "" {
		return sizeOfVarlen(len(v))
	}
	return 0
}

func encodeString(b []byte, p unsafe.Pointer, _ flags) ([]byte, error) {
	v := *(*string)(p)
	if v != "" {
		b = appendVarint(b, uint64(len(v)))
		b = append(b, v...)
	}
	return b, nil
}

func decodeString(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeVarlen(b)
	*(*string)(p) = string(v)
	return n, err
}

var stringRequiredCodec = codec{
	size:   sizeOfStringRequired,
	encode: encodeStringRequired,
	decode: decodeString,
}

func sizeOfStringRequired(p unsafe.Pointer, _ flags) int {
	v := *(*string)(p)
	return sizeOfVarlen(len(v))
}

func encodeStringRequired(b []byte, p unsafe.Pointer, _ flags) ([]byte, error) {
	v := *(*string)(p)
	b = appendVarint(b, uint64(len(v)))
	b = append(b, v...)
	return b, nil
}

var stringPtrCodec = codec{
	size:   sizeOfStringPtr,
	encode: encodeStringPtr,
	decode: decodeStringPtr,
}

func sizeOfStringPtr(p unsafe.Pointer, _ flags) int {
	p = deref(p)
	if p != nil {
		v := *(*string)(p)
		return sizeOfVarlen(len(v))
	}
	return 0
}

func encodeStringPtr(b []byte, p unsafe.Pointer, _ flags) ([]byte, error) {
	p = deref(p)
	if p != nil {
		v := *(*string)(p)
		b = appendVarint(b, uint64(len(v)))
		b = append(b, v...)
	}
	return b, nil
}

func decodeStringPtr(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v := (*unsafe.Pointer)(p)
	if *v == nil {
		*v = unsafe.Pointer(new(string))
	}
	s, n, err := decodeVarlen(b)
	*(*string)(*v) = string(s)
	return n, err
}
