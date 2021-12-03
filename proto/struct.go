package proto

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"
)

const (
	embedded = 1 << 0
	repeated = 1 << 1
)

type structInfo struct {
	fields     []*structField
	fieldIndex map[fieldNumber]*structField
}

type structField struct {
	offset  uintptr
	wiretag uint64
	codec   *codec
	tagsize uint8
	flags   uint8
}

func (f *structField) String() string {
	return fmt.Sprintf("[%d,%s]", f.fieldNumber(), f.wireType())
}

func (f *structField) fieldNumber() fieldNumber {
	return fieldNumber(f.wiretag >> 3)
}

func (f *structField) wireType() wireType {
	return wireType(f.wiretag & 7)
}

func (f *structField) embedded() bool {
	return (f.flags & embedded) != 0
}

func (f *structField) repeated() bool {
	return (f.flags & repeated) != 0
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
		if f.repeated() {
			size := f.codec.size(f.pointer(p))
			if size > 0 {
				n += size
			}
			continue
		}
		size := f.codec.size(f.pointer(p))
		if size > 0 {
			n += int(f.tagsize) + size
		}
	}
	return n
}

func (info *structInfo) encode(b []byte, p unsafe.Pointer) ([]byte, error) {
	if p == nil {
		return b, nil
	}

	var err error
	for _, f := range info.fields {
		if f.repeated() {
			b, err = f.codec.encode(b, f.pointer(p))
			if err != nil {
				return b, err
			}
			continue
		}
		elem := f.pointer(p)
		size := f.codec.size(elem)
		if size > 0 {
			b = appendVarint(b, f.wiretag)
			b, err = f.codec.encode(b, elem)
			if err != nil {
				return b, err
			}
		}
	}
	return b, nil
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
			size := uint64(0)
			switch wireType {
			case varint:
				_, skip, err = decodeVarint(b[offset:])
			case varlen:
				size, skip, err = decodeVarint(b[offset:])
				if err == nil {
					if size > uint64(len(b)-skip) {
						err = io.ErrUnexpectedEOF
					} else {
						skip += int(size)
					}
				}
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

		// `data` will only contain the section of the input buffer where
		// the data for the next field is available. This is necessary to
		// limit how many bytes will be consumed by embedded messages.
		var data []byte
		switch wireType {
		case varint:
			_, n, err := decodeVarint(b[offset:])
			if err != nil {
				return offset, fieldError(fieldNumber, wireType, err)
			}
			data = b[offset : offset+n]

		case varlen:
			l, n, err := decodeVarint(b[offset:])
			if err != nil {
				return offset + n, fieldError(fieldNumber, wireType, err)
			}
			if l > uint64(len(b)-(offset+n)) {
				return len(b), fieldError(fieldNumber, wireType, io.ErrUnexpectedEOF)
			}
			if f.embedded() {
				offset += n
				data = b[offset : offset+int(l)]
			} else {
				data = b[offset : offset+n+int(l)]
			}

		case fixed32:
			if (offset + 4) > len(b) {
				return len(b), fieldError(fieldNumber, wireType, io.ErrUnexpectedEOF)
			}
			data = b[offset : offset+4]

		case fixed64:
			if (offset + 8) > len(b) {
				return len(b), fieldError(fieldNumber, wireType, io.ErrUnexpectedEOF)
			}
			data = b[offset : offset+8]

		default:
			return offset, fieldError(fieldNumber, wireType, ErrWireTypeUnknown)
		}

		n, err = f.codec.decode(data, f.pointer(p))
		offset += n
		if err != nil {
			return offset, fieldError(fieldNumber, wireType, err)
		}
	}

	return offset, nil
}

type structTag struct {
	name        string
	enum        string
	json        string
	version     int
	wireType    wireType
	fieldNumber fieldNumber
	extensions  map[string]string
	repeated    bool
	zigzag      bool
}

func parseStructTag(tag string) (structTag, error) {
	t := structTag{
		version:    2,
		extensions: make(map[string]string),
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
		}
	}

	return t, nil
}

func splitFields(s string) []string {
	return strings.Split(s, ",")
}

func splitNameValue(s string) (name, value string) {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return strings.TrimSpace(s), ""
	} else {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
}
