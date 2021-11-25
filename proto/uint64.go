package proto

import "unsafe"

var uint64Codec = codec{
	wire:   varint,
	size:   sizeOfUint64,
	encode: encodeUint64,
	decode: decodeUint64,
}

func sizeOfUint64(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*uint64)(p); v != 0 || flags.has(wantzero) {
			return sizeOfVarint(v)
		}
	}
	return 0
}

func encodeUint64(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*uint64)(p); v != 0 || flags.has(wantzero) {
			b = appendVarint(b, v)
		}
	}
	return b, nil
}

func decodeUint64(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeVarint(b)
	*(*uint64)(p) = uint64(v)
	return n, err
}

var fixed64Codec = codec{
	wire:   fixed64,
	size:   sizeOfFixed64,
	encode: encodeFixed64,
	decode: decodeFixed64,
}

func sizeOfFixed64(p unsafe.Pointer, flags flags) int {
	if p != nil {
		if v := *(*uint64)(p); v != 0 || flags.has(wantzero) {
			return 8
		}
	}
	return 0
}

func encodeFixed64(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
	if p != nil {
		if v := *(*uint64)(p); v != 0 || flags.has(wantzero) {
			b = encodeLE64(b, v)
		}
	}
	return b, nil
}

func decodeFixed64(b []byte, p unsafe.Pointer, _ flags) (int, error) {
	v, n, err := decodeLE64(b)
	*(*uint64)(p) = v
	return n, err
}
