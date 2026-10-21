// Package vm implements the flat opcode interpreter behind proto's encoder,
// in the spirit of goccy/go-json's opcode VM: message structure is compiled
// once into a []Field, and Encode interprets it with a switch loop so scalar
// fields are encoded inline without indirect closure calls.
package vm

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/RomiChan/protobuf/internal/option"
)

// Kind classifies a compiled field. Scalar kinds are inlined by Encode;
// Fallback kinds defer to the field's Enc hook.
type Kind uint8

const (
	Fallback Kind = iota
	Bool
	Int32
	Int64
	Uint32
	Uint64
	Float32
	Float64
	Fixed32
	Fixed64
	Zigzag32
	Zigzag64
	String
	Bytes
)

const (
	// PtrFlag marks a pointer-wrapped scalar field (*T).
	PtrFlag Kind = 0x40
	// OptionFlag marks an option.Option scalar field.
	OptionFlag Kind = 0x80
)

// Field is the compiled form of one message field.
type Field struct {
	Offset  uintptr
	Wiretag uint64
	Kind    Kind
	// Enc handles Fallback kinds (messages, slices, maps, ...). It receives
	// the field address and returns the new cursor. The tape carries message
	// sizes recorded by the size pass; handlers may pass it on to nested
	// messages and must tolerate a nil tape (recompute sizes instead).
	Enc func(b []byte, i int, p unsafe.Pointer, t *Tape) int
}

// Tape records message body sizes during the size pass in preorder and
// replays them during the encode pass, so each subtree's size is computed
// exactly once per Marshal.
type Tape struct {
	buf []int
	pos int
}

func (t *Tape) Reset() {
	t.buf = t.buf[:0]
	t.pos = 0
}

// Reserve appends a placeholder for a message whose size is not yet known
// (its subtree is sized first) and returns the placeholder's index.
func (t *Tape) Reserve() int {
	t.buf = append(t.buf, 0)
	return len(t.buf) - 1
}

// Set fills a placeholder returned by Reserve.
func (t *Tape) Set(idx, n int) {
	t.buf[idx] = n
}

// Next replays the next recorded size in preorder.
func (t *Tape) Next() int {
	n := t.buf[t.pos]
	t.pos++
	return n
}

func writeVarint(b []byte, i int, v uint64) int {
	for v >= 0x80 {
		b[i] = byte(v) | 0x80
		v >>= 7
		i++
	}
	b[i] = byte(v)
	return i + 1
}

func writeLE32(b []byte, i int, v uint32) int {
	binary.LittleEndian.PutUint32(b[i:], v)
	return i + 4
}

func writeLE64(b []byte, i int, v uint64) int {
	binary.LittleEndian.PutUint64(b[i:], v)
	return i + 8
}

func encodeZigZag64(v int64) uint64 {
	return (uint64(v) << 1) ^ uint64(v>>63)
}

// Encode writes the message at p to b starting at cursor i and returns the
// new cursor. The caller guarantees b has exactly enough capacity, so no
// bounds grow happens here. t carries the message sizes recorded by the size
// pass and may be nil when there are no message fields.
func Encode(b []byte, i int, p unsafe.Pointer, ops []Field, t *Tape) int {
	for k := range ops {
		op := &ops[k]
		fp := unsafe.Pointer(uintptr(p) + op.Offset)
		switch op.Kind {
		case Bool:
			if *(*bool)(fp) {
				i = writeVarint(b, i, op.Wiretag)
				b[i] = 1
				i++
			}
		case Int32:
			if v := *(*int32)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(v))
			}
		case Int64:
			if v := *(*int64)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(v))
			}
		case Uint32:
			if v := *(*uint32)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(v))
			}
		case Uint64:
			if v := *(*uint64)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, v)
			}
		case Float32:
			if v := *(*float32)(fp); v != 0 || math.Signbit(float64(v)) {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE32(b, i, math.Float32bits(v))
			}
		case Float64:
			if v := *(*float64)(fp); v != 0 || math.Signbit(v) {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE64(b, i, math.Float64bits(v))
			}
		case Fixed32:
			if v := *(*uint32)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE32(b, i, v)
			}
		case Fixed64:
			if v := *(*uint64)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE64(b, i, v)
			}
		case Zigzag32:
			if v := *(*int32)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, encodeZigZag64(int64(v)))
			}
		case Zigzag64:
			if v := *(*int64)(fp); v != 0 {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, encodeZigZag64(v))
			}
		case String:
			if v := *(*string)(fp); v != "" {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(len(v)))
				i += copy(b[i:], v)
			}
		case Bytes:
			if v := *(*[]byte)(fp); v != nil {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(len(v)))
				i += copy(b[i:], v)
			}

		case PtrFlag | Bool:
			if q := *(*unsafe.Pointer)(fp); q != nil && *(*bool)(q) {
				i = writeVarint(b, i, op.Wiretag)
				b[i] = 1
				i++
			}
		case PtrFlag | Int32:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*int32)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, uint64(v))
				}
			}
		case PtrFlag | Int64:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*int64)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, uint64(v))
				}
			}
		case PtrFlag | Uint32:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*uint32)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, uint64(v))
				}
			}
		case PtrFlag | Uint64:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*uint64)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, v)
				}
			}
		case PtrFlag | Float32:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*float32)(q); v != 0 || math.Signbit(float64(v)) {
					i = writeVarint(b, i, op.Wiretag)
					i = writeLE32(b, i, math.Float32bits(v))
				}
			}
		case PtrFlag | Float64:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*float64)(q); v != 0 || math.Signbit(v) {
					i = writeVarint(b, i, op.Wiretag)
					i = writeLE64(b, i, math.Float64bits(v))
				}
			}
		case PtrFlag | Fixed32:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*uint32)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeLE32(b, i, v)
				}
			}
		case PtrFlag | Fixed64:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*uint64)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeLE64(b, i, v)
				}
			}
		case PtrFlag | Zigzag32:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*int32)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, encodeZigZag64(int64(v)))
				}
			}
		case PtrFlag | Zigzag64:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*int64)(q); v != 0 {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, encodeZigZag64(v))
				}
			}
		case PtrFlag | String:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*string)(q); v != "" {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, uint64(len(v)))
					i += copy(b[i:], v)
				}
			}
		case PtrFlag | Bytes:
			if q := *(*unsafe.Pointer)(fp); q != nil {
				if v := *(*[]byte)(q); v != nil {
					i = writeVarint(b, i, op.Wiretag)
					i = writeVarint(b, i, uint64(len(v)))
					i += copy(b[i:], v)
				}
			}

		case OptionFlag | Bool:
			if o := (*option.Option[bool])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				if o.Unwrap() {
					b[i] = 1
				} else {
					b[i] = 0
				}
				i++
			}
		case OptionFlag | Int32:
			if o := (*option.Option[int32])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(o.Unwrap()))
			}
		case OptionFlag | Int64:
			if o := (*option.Option[int64])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(o.Unwrap()))
			}
		case OptionFlag | Uint32:
			if o := (*option.Option[uint32])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(o.Unwrap()))
			}
		case OptionFlag | Uint64:
			if o := (*option.Option[uint64])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, o.Unwrap())
			}
		case OptionFlag | Float32:
			if o := (*option.Option[float32])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE32(b, i, math.Float32bits(o.Unwrap()))
			}
		case OptionFlag | Float64:
			if o := (*option.Option[float64])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE64(b, i, math.Float64bits(o.Unwrap()))
			}
		case OptionFlag | Fixed32:
			if o := (*option.Option[uint32])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE32(b, i, o.Unwrap())
			}
		case OptionFlag | Fixed64:
			if o := (*option.Option[uint64])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeLE64(b, i, o.Unwrap())
			}
		case OptionFlag | Zigzag32:
			if o := (*option.Option[int32])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, encodeZigZag64(int64(o.Unwrap())))
			}
		case OptionFlag | Zigzag64:
			if o := (*option.Option[int64])(fp); o.IsSome() {
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, encodeZigZag64(o.Unwrap()))
			}
		case OptionFlag | String:
			if o := (*option.Option[string])(fp); o.IsSome() {
				v := o.Unwrap()
				i = writeVarint(b, i, op.Wiretag)
				i = writeVarint(b, i, uint64(len(v)))
				i += copy(b[i:], v)
			}

		default:
			i = op.Enc(b, i, fp, t)
		}
	}
	return i
}
