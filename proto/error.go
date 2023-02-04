package proto

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrWireTypeUnknown = errors.New("unknown wire type")

type UnmarshalFieldError struct {
	FieldNumber int
	WireType    int
	Err         error
}

func (e *UnmarshalFieldError) Error() string {
	return fmt.Sprintf("field number %d with wire type %d: %v", e.FieldNumber, e.WireType, e.Err)
}

func (e *UnmarshalFieldError) Unwrap() error { return e.Err }

func fieldError(f fieldNumber, t wireType, err error) error {
	return &UnmarshalFieldError{
		FieldNumber: int(f),
		WireType:    int(t),
		Err:         err,
	}
}

// An InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
// (The argument to Unmarshal must be a non-nil pointer to a struct.)
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "proto: Unmarshal(nil)"
	}

	if e.Type.Kind() != reflect.Pointer {
		return "proto: Unmarshal(non-pointer" + e.Type.String() + ")"
	}

	elem := e.Type.Elem()
	if elem.Kind() != reflect.Struct {
		return "proto: Unmarshal(bad pointer" + e.Type.String() + ")"
	}
	return "proto: Unmarshal(nil " + e.Type.String() + ")"
}
