package proto

import (
	"math/bits"
	"unsafe"

	"github.com/RomiChan/protobuf/internal/vm"
)

type sizeFunc = func(unsafe.Pointer, *structField) int

// tapeSizeFunc is the tape-aware counterpart of sizeFunc: while sizing it
// records the body size of every nested message it meets on t, so the encode
// pass can replay them instead of recomputing each subtree.
type tapeSizeFunc = func(p unsafe.Pointer, f *structField, t *vm.Tape) int

func sizeOfVarint(v uint64) int {
	// This computes 1 + (bits.Len64(v)-1)/7.
	// 9/64 is a good enough approximation of 1/7
	// see https://github.com/protocolbuffers/protobuf-go/commit/a30b571f93edc9b3bd5df1dd61ceaeb17aa7f7c5
	return int(9*uint32(bits.Len64(v))+64) / 64
}

func sizeOfVarlen(n int) int {
	return sizeOfVarint(uint64(n)) + n
}
