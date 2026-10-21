// Package option provides the optional scalar field type shared by the
// proto package and the encode VM.
package option

import "unsafe"

type Option[T any] struct {
	some  bool
	value T
}

func Some[T any](val T) Option[T] {
	return Option[T]{
		some:  true,
		value: val,
	}
}

func None[T any]() Option[T] {
	return Option[T]{some: false}
}

func (o Option[T]) IsSome() bool {
	return o.some
}

func (o Option[T]) IsNone() bool {
	return !o.some
}

func (o Option[T]) Unwrap() T {
	if o.IsSome() {
		return o.value
	}
	var zero T
	return zero
}

func (o Option[T]) UnwrapOr(defaultValue T) T {
	if o.IsSome() {
		return o.value
	}
	return defaultValue
}

// MarkSome marks the option as present; used by decoders before decoding
// into the value.
func (o *Option[T]) MarkSome() {
	o.some = true
}

// UnsafePointer returns a pointer to the held value; used by the
// pointer-based codecs.
func (o *Option[T]) UnsafePointer() unsafe.Pointer {
	return unsafe.Pointer(&o.value)
}
