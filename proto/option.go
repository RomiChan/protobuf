package proto

import (
	"github.com/RomiChan/protobuf/internal/option"
)

type Option[T any] = option.Option[T]

func Some[T any](val T) Option[T] {
	return option.Some(val)
}

func None[T any]() Option[T] {
	return option.None[T]()
}

// Bool stores v in a new bool value and returns a pointer to it.
func Bool(v bool) Option[bool] { return Some(v) }

// Int32 stores v in a new int32 value and returns a pointer to it.
func Int32(v int32) Option[int32] { return Some(v) }

// Int64 stores v in a new int64 value and returns a pointer to it.
func Int64(v int64) Option[int64] { return Some(v) }

// Float32 stores v in a new float32 value and returns a pointer to it.
func Float32(v float32) Option[float32] { return Some(v) }

// Float64 stores v in a new float64 value and returns a pointer to it.
func Float64(v float64) Option[float64] { return Some(v) }

// Uint32 stores v in a new uint32 value and returns a pointer to it.
func Uint32(v uint32) Option[uint32] { return Some(v) }

// Uint64 stores v in a new uint64 value and returns a pointer to it.
func Uint64(v uint64) Option[uint64] { return Some(v) }

// String stores v in a new string value and returns a pointer to it.
func String(v string) Option[string] { return Some(v) }
