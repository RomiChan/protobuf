package proto

import (
	"encoding/binary"
	"unsafe"

	"github.com/RomiChan/protobuf/internal/vm"
)

func encodeZigZag64(v int64) uint64 {
	return (uint64(v) << 1) ^ uint64(v>>63)
}

// encodeFunc writes the field value to b starting at cursor i and returns the
// new cursor. Callers guarantee the buffer has enough capacity (Marshal
// preallocates the exact size), so encoders never grow b.
type encodeFunc = func(b []byte, i int, p unsafe.Pointer, f *structField) int

// tapeEncodeFunc is the tape-aware counterpart of encodeFunc: nested message
// bodies are replayed from t instead of being sized again.
type tapeEncodeFunc = func(b []byte, i int, p unsafe.Pointer, f *structField, t *vm.Tape) int

func writeVarint(b []byte, i int, v uint64) int {
	for v >= 0x80 {
		b[i] = byte(v) | 0x80
		v >>= 7
		i++
	}
	b[i] = byte(v)
	return i + 1
}

func writeVarintZigZag(b []byte, i int, v int64) int {
	return writeVarint(b, i, encodeZigZag64(v))
}

func writeLE32(b []byte, i int, v uint32) int {
	binary.LittleEndian.PutUint32(b[i:], v)
	return i + 4
}

func writeLE64(b []byte, i int, v uint64) int {
	binary.LittleEndian.PutUint64(b[i:], v)
	return i + 8
}

func writeTag(b []byte, i int, f fieldNumber, t wireType) int {
	return writeVarint(b, i, uint64(f)<<3|uint64(t))
}
