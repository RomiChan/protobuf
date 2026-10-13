package proto

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"
)

type structInfo struct {
	fields     []*structField
	fieldIndex map[fieldNumber]*structField
}

type structField struct {
	offset  uintptr
	wiretag uint64
	codec   *codec
	tagsize int
}

func (f *structField) fieldNumber() fieldNumber {
	return fieldNumber(f.wiretag >> 3)
}

func (f *structField) wireType() wireType {
	return wireType(f.wiretag & 7)
}

func (f *structField) pointer(p unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(uintptr(p) + f.offset)
}

func (info *structInfo) size(p unsafe.Pointer) int {
	if p == nil {
		return 0
	}
	n := 0
	for _, f := range info.fields {
		n += f.codec.size(f.pointer(p), f)
	}
	return n
}

func (info *structInfo) encode(b []byte, p unsafe.Pointer) []byte {
	if p == nil {
		return b
	}
	for _, f := range info.fields {
		b = f.codec.encode(b, f.pointer(p), f)
	}
	return b
}

func (info *structInfo) decode(b []byte, p unsafe.Pointer) (int, error) {
	offset := 0
	for offset < len(b) {
		fieldNumber, wireType, n, err := decodeTag(b[offset:])
		offset += n
		if err != nil {
			return offset, err
		}

		f := info.fieldIndex[fieldNumber]
		if f == nil {
			skip := 0
			switch wireType {
			case varint:
				_, skip, err = decodeVarint(b[offset:])
			case varlen:
				_, skip, err = decodeVarlen(b[offset:])
			case fixed32:
				skip = 4
			case fixed64:
				skip = 8
			default:
				err = ErrWireTypeUnknown
			}
			if (offset + skip) <= len(b) {
				offset += skip
			} else {
				offset, err = len(b), io.ErrUnexpectedEOF
			}
			if err != nil {
				return offset, fieldError(fieldNumber, wireType, err)
			}
			continue
		}

		if wireType != f.wireType() {
			return offset, fieldError(fieldNumber, wireType, fmt.Errorf("expected wire type %d", f.wireType()))
		}

		// Codecs consume exactly the bytes of their field (length-delimited
		// codecs consume their own length prefix), so the returned n bounds
		// embedded messages without decoding the value twice.
		n, err = f.codec.decode(b[offset:], f.pointer(p))
		offset += n
		if err != nil {
			return offset, fieldError(fieldNumber, wireType, err)
		}
	}

	return offset, nil
}

type structTag struct {
	// version     int
	wireType    wireType
	fieldNumber fieldNumber
	repeated    bool
	zigzag      bool
}

func parseStructTag(tag string) (structTag, error) {
	t := structTag{
		// version:    2,
	}

	for i, f := range splitFields(tag) {
		switch i {
		case 0:
			switch f {
			case "varint":
				t.wireType = varint
			case "bytes":
				t.wireType = varlen
			case "fixed32":
				t.wireType = fixed32
			case "fixed64":
				t.wireType = fixed64
			case "zigzag32":
				t.wireType = varint
				t.zigzag = true
			case "zigzag64":
				t.wireType = varint
				t.zigzag = true
			default:
				return t, fmt.Errorf("unsupported wire type in struct tag %q: %s", tag, f)
			}

		case 1:
			n, err := strconv.Atoi(f)
			if err != nil {
				return t, fmt.Errorf("unsupported field number in struct tag %q: %w", tag, err)
			}
			t.fieldNumber = fieldNumber(n)

		case 2:
			switch f {
			case "opt":
				// not sure what this is for
			case "rep":
				t.repeated = true
			default:
				return t, fmt.Errorf("unsupported field option in struct tag %q: %s", tag, f)
			}

		default:
			/*
				name, value := splitNameValue(f)
				switch name {
				case "name":
					t.name = value
				case "enum":
					t.enum = value
				case "json":
					t.json = value
				case "proto3":
					t.version = 3
				default:
					t.extensions[name] = value
				}
			*/
		}
	}

	return t, nil
}

func splitFields(s string) []string {
	return strings.Split(s, ",")
}

/*
func splitNameValue(s string) (name, value string) {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return strings.TrimSpace(s), ""
	} else {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
}
*/
