package proto

import "unsafe"

var zigzag32Codec = codec{
	size:   sizeOfZigzag32,
	encode: encodeZigzag32,
	decode: decodeZigzag32,
}

func sizeOfZigzag32(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*int32)(p); v != 0 || flags.has(wantzero) {
			return sizeOfVarint(encodeZigZag64(int64(v)))
		}
	}
	return 0
}

func encodeZigzag32(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*int32)(p); v != 0 || flags.has(wantzero) {
			b = appendVarint(b, encodeZigZag64(int64(v)))
		}
	}
	return b, nil
}

func decodeZigzag32(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	u, n, err := decodeVarint(b)
	*(*int32)(p) = int32(decodeZigZag64(u))
	return n, err
}

var zigzag64Codec = codec{
	size:   sizeOfZigzag64,
	encode: encodeZigzag64,
	decode: decodeZigzag64,
}

func sizeOfZigzag64(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*int64)(p); v != 0 || flags.has(wantzero) {
			return sizeOfVarint(encodeZigZag64(v))
		}
	}
	return 0
}

func encodeZigzag64(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*int64)(p); v != 0 || flags.has(wantzero) {
			b = appendVarint(b, encodeZigZag64(v))
		}
	}
	return b, nil
}

func decodeZigzag64(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeVarint(b)
	*(*int64)(p) = decodeZigZag64(v)
	return n, err
}
