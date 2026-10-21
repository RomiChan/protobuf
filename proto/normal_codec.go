package proto

import (
	"io"
	"math"
	"unsafe"
)

var boolCodec = codec{
	size:   sizeOfBool,
	encode: encodeBool,
	decode: decodeBool,
}

func sizeOfBool(p unsafe.Pointer, f *structField) int {
	if *(*bool)(p) {
		return 1 + int(f.tagsize)
	}
	return 0
}

func encodeBool(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if *(*bool)(p) {
		i = writeVarint(b, i, f.wiretag)
		if *(*bool)(p) { // keep this for code generate
			b[i] = 1
		} else {
			b[i] = 0
		}
		i++
	}
	return i
}

func decodeBool(b []byte, p unsafe.Pointer) (int, error) {
	if len(b) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	*(*bool)(p) = b[0] != 0
	return 1, nil
}

var bytesCodec = codec{
	size:   sizeOfBytes,
	encode: encodeBytes,
	decode: decodeBytes,
}

func sizeOfBytes(p unsafe.Pointer, f *structField) int {
	v := *(*[]byte)(p)
	if v != nil {
		return sizeOfVarlen(len(v)) + f.tagsize
	}
	return 0
}

func encodeBytes(b []byte, i int, p unsafe.Pointer, f *structField) int {
	v := *(*[]byte)(p)
	if v != nil {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, uint64(len(v)))
		i += copy(b[i:], v)
	}
	return i
}

func decodeBytes(b []byte, p unsafe.Pointer) (int, error) {
	v, n, err := decodeVarlen(b)
	pb := (*[]byte)(p)
	if *pb == nil {
		*pb = make([]byte, 0, len(v))
	}
	*pb = append((*pb)[:0], v...)
	return n, err
}

var stringCodec = codec{
	size:   sizeOfString,
	encode: encodeString,
	decode: decodeString,
}

func sizeOfString(p unsafe.Pointer, f *structField) int {
	v := *(*string)(p)
	if v != "" {
		return sizeOfVarlen(len(v)) + f.tagsize
	}
	return 0
}

func encodeString(b []byte, i int, p unsafe.Pointer, f *structField) int {
	v := *(*string)(p)
	if v != "" {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, uint64(len(v)))
		i += copy(b[i:], v)
	}
	return i
}

func decodeString(b []byte, p unsafe.Pointer) (int, error) {
	v, n, err := decodeVarlen(b)
	if n == 0 {
		*(*string)(p) = ""
		return 0, err
	}
	*(*string)(p) = string(v)
	return n, err
}

var float32Codec = codec{
	size:   sizeOfFloat32,
	encode: encodeFloat32,
	decode: decodeFloat32,
}

func sizeOfFloat32(p unsafe.Pointer, f *structField) int {
	if *(*float32)(p) != 0 || math.Signbit(float64(*(*float32)(p))) {
		return 4 + int(f.tagsize)
	}
	return 0
}

func encodeFloat32(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*float32)(p); v != 0 || math.Signbit(float64(v)) {
		i = writeVarint(b, i, f.wiretag)
		i = writeLE32(b, i, math.Float32bits(v))
	}
	return i
}

func decodeFloat32(b []byte, p unsafe.Pointer) (int, error) {
	v, n, err := decodeLE32(b)
	*(*float32)(p) = math.Float32frombits(v)
	return n, err
}

var float64Codec = codec{
	size:   sizeOfFloat64,
	encode: encodeFloat64,
	decode: decodeFloat64,
}

func sizeOfFloat64(p unsafe.Pointer, f *structField) int {
	if *(*float64)(p) != 0 || math.Signbit(*(*float64)(p)) {
		return 8 + int(f.tagsize)
	}
	return 0
}

func encodeFloat64(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*float64)(p); v != 0 || math.Signbit(v) {
		i = writeVarint(b, i, f.wiretag)
		i = writeLE64(b, i, math.Float64bits(v))
	}
	return i
}

func decodeFloat64(b []byte, p unsafe.Pointer) (int, error) {
	v, n, err := decodeLE64(b)
	*(*float64)(p) = math.Float64frombits(v)
	return n, err
}

var int32Codec = codec{
	size:   sizeOfInt32,
	encode: encodeInt32,
	decode: decodeInt32,
}

func sizeOfInt32(p unsafe.Pointer, f *structField) int {
	v := *(*int32)(p)
	if v != 0 {
		return sizeOfVarint(uint64(v)) + f.tagsize
	}
	return 0
}

func encodeInt32(b []byte, i int, p unsafe.Pointer, f *structField) int {
	v := *(*int32)(p)
	if v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, uint64(v))
	}
	return i
}

func decodeInt32(b []byte, p unsafe.Pointer) (int, error) {
	u, n, err := decodeVarint(b)
	*(*int32)(p) = int32(int64(u))
	return n, err
}

var int64Codec = codec{
	size:   sizeOfInt64,
	encode: encodeInt64,
	decode: decodeInt64,
}

func sizeOfInt64(p unsafe.Pointer, f *structField) int {
	v := *(*int64)(p)
	if v != 0 {
		return sizeOfVarint(uint64(v)) + f.tagsize
	}
	return 0
}

func encodeInt64(b []byte, i int, p unsafe.Pointer, f *structField) int {
	v := *(*int64)(p)
	if v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, uint64(v))
	}
	return i
}

func decodeInt64(b []byte, p unsafe.Pointer) (int, error) {
	u, n, err := decodeVarint(b)
	*(*int64)(p) = int64(u)
	return n, err
}

var uint32Codec = codec{
	size:   sizeOfUint32,
	encode: encodeUint32,
	decode: decodeUint32,
}

func sizeOfUint32(p unsafe.Pointer, f *structField) int {
	if v := *(*uint32)(p); v != 0 {
		return sizeOfVarint(uint64(v)) + f.tagsize
	}
	return 0
}

func encodeUint32(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*uint32)(p); v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, uint64(v))
	}
	return i
}

func decodeUint32(b []byte, p unsafe.Pointer) (int, error) {
	u, n, err := decodeVarint(b)
	*(*uint32)(p) = uint32(u)
	return n, err
}

var fixed32Codec = codec{
	size:   sizeOfFixed32,
	encode: encodeFixed32,
	decode: decodeFixed32,
}

func sizeOfFixed32(p unsafe.Pointer, f *structField) int {
	if *(*uint32)(p) != 0 {
		return 4 + f.tagsize
	}
	return 0
}

func encodeFixed32(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*uint32)(p); v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeLE32(b, i, v)
	}
	return i
}

func decodeFixed32(b []byte, p unsafe.Pointer) (int, error) {
	v, n, err := decodeLE32(b)
	*(*uint32)(p) = v
	return n, err
}

var uint64Codec = codec{
	size:   sizeOfUint64,
	encode: encodeUint64,
	decode: decodeUint64,
}

func sizeOfUint64(p unsafe.Pointer, f *structField) int {
	if v := *(*uint64)(p); v != 0 {
		return sizeOfVarint(v) + f.tagsize
	}
	return 0
}

func encodeUint64(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*uint64)(p); v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, v)
	}
	return i
}

func decodeUint64(b []byte, p unsafe.Pointer) (int, error) {
	u, n, err := decodeVarint(b)
	*(*uint64)(p) = u
	return n, err
}

var fixed64Codec = codec{
	size:   sizeOfFixed64,
	encode: encodeFixed64,
	decode: decodeFixed64,
}

func sizeOfFixed64(p unsafe.Pointer, f *structField) int {
	if *(*uint64)(p) != 0 {
		return 8 + f.tagsize
	}
	return 0
}

func encodeFixed64(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*uint64)(p); v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeLE64(b, i, v)
	}
	return i
}

func decodeFixed64(b []byte, p unsafe.Pointer) (int, error) {
	v, n, err := decodeLE64(b)
	*(*uint64)(p) = v
	return n, err
}

var zigzag32Codec = codec{
	size:   sizeOfZigzag32,
	encode: encodeZigzag32,
	decode: decodeZigzag32,
}

func sizeOfZigzag32(p unsafe.Pointer, f *structField) int {
	if v := *(*int32)(p); v != 0 {
		return sizeOfVarint(encodeZigZag64(int64(v))) + f.tagsize
	}
	return 0
}

func encodeZigzag32(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*int32)(p); v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, encodeZigZag64(int64(v)))
	}
	return i
}

func decodeZigzag32(b []byte, p unsafe.Pointer) (int, error) {
	u, n, err := decodeVarint(b)
	*(*int32)(p) = int32(decodeZigZag64(u))
	return n, err
}

var zigzag64Codec = codec{
	size:   sizeOfZigzag64,
	encode: encodeZigzag64,
	decode: decodeZigzag64,
}

func sizeOfZigzag64(p unsafe.Pointer, f *structField) int {
	if v := *(*int64)(p); v != 0 {
		return sizeOfVarint(encodeZigZag64(v)) + f.tagsize
	}
	return 0
}

func encodeZigzag64(b []byte, i int, p unsafe.Pointer, f *structField) int {
	if v := *(*int64)(p); v != 0 {
		i = writeVarint(b, i, f.wiretag)
		i = writeVarint(b, i, encodeZigZag64(v))
	}
	return i
}

func decodeZigzag64(b []byte, p unsafe.Pointer) (int, error) {
	u, n, err := decodeVarint(b)
	*(*int64)(p) = decodeZigZag64(u)
	return n, err
}
