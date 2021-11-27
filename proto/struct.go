package proto

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

const (
	embedded = 1 << 0
	repeated = 1 << 1
	zigzag   = 1 << 2
)

type structField struct {
	offset  uintptr
	wiretag uint64
	tagsize uint8
	flags   uint8
	codec   *codec
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

func (f *structField) makeFlags(base flags) flags {
	return base | flags(f.flags&zigzag)
}

func structCodecOf(t reflect.Type, seen map[reflect.Type]*codec) *codec {
	c := &codec{wire: varlen}
	seen[t] = c

	numField := t.NumField()
	number := 1
	fields := make([]structField, 0, numField)

	for i := 0; i < numField; i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}

		tag, ok := f.Tag.Lookup("protobuf")
		if !ok {
			continue // no tag
		}

		field := structField{
			offset: f.Offset,
		}

		t, err := parseStructTag(tag)
		if err != nil {
			panic(err)
		}
		field.wiretag = uint64(t.fieldNumber)<<3 | uint64(t.wireType)
		if t.repeated {
			field.flags |= repeated
		}
		if t.zigzag {
			field.flags |= zigzag
		}
		switch t.wireType {
		case fixed32:
			switch baseKindOf(f.Type) {
			case reflect.Uint32:
				field.codec = fixPtrCodec(f.Type, &fixed32Codec)
			case reflect.Float32:
				field.codec = fixPtrCodec(f.Type, &float32Codec)
			}
		case fixed64:
			switch baseKindOf(f.Type) {
			case reflect.Uint64:
				field.codec = fixPtrCodec(f.Type, &fixed64Codec)
			case reflect.Float64:
				field.codec = fixPtrCodec(f.Type, &float64Codec)
			}
		}

		if field.codec == nil {
			switch baseKindOf(f.Type) {
			case reflect.Struct:
				field.flags |= embedded
				field.codec = codecOf(f.Type, seen)

			case reflect.Slice:
				elem := f.Type.Elem()

				if elem.Kind() == reflect.Uint8 { // []byte
					field.codec = codecOf(f.Type, seen)
				} else {
					if baseKindOf(elem) == reflect.Struct {
						field.flags |= embedded
					}
					field.flags |= repeated
					field.codec = codecOf(elem, seen)
					field.codec = sliceCodecOf(f.Type, field, seen)
				}

			case reflect.Map:
				key, val := f.Type.Key(), f.Type.Elem()
				k := codecOf(key, seen)
				v := codecOf(val, seen)
				m := &mapField{
					wiretag:  field.wiretag,
					keyCodec: k,
					valCodec: v,
				}
				if baseKindOf(key) == reflect.Struct {
					m.keyFlags |= embedded
				}
				if baseKindOf(val) == reflect.Struct {
					m.valFlags |= embedded
				}
				field.flags |= embedded | repeated
				field.codec = mapCodecOf(f.Type, m, seen)

			default:
				field.codec = codecOf(f.Type, seen)
			}
		}

		field.tagsize = uint8(sizeOfVarint(field.wiretag))
		fields = append(fields, field)
		number++
	}

	c.size = structSizeFuncOf(t, fields)
	c.encode = structEncodeFuncOf(t, fields)
	c.decode = structDecodeFuncOf(t, fields)
	return c
}

func baseKindOf(t reflect.Type) reflect.Kind {
	return baseTypeOf(t).Kind()
}

func baseTypeOf(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func fixPtrCodec(t reflect.Type, c *codec) *codec {
	if t.Kind() == reflect.Ptr {
		p := new(codec)
		p.wire = c.wire
		p.size = pointerSizeFuncOf(t, c)
		p.encode = pointerEncodeFuncOf(t, c)
		p.decode = pointerDecodeFuncOf(t, c)
		c = p
	}
	return c
}

func structSizeFuncOf(t reflect.Type, fields []structField) sizeFunc {
	var inlined = inlined(t)
	var unique, repeated []*structField

	for i := range fields {
		f := &fields[i]
		if f.repeated() {
			repeated = append(repeated, f)
		} else {
			unique = append(unique, f)
		}
	}

	return func(p unsafe.Pointer, flags flags) int {
		if p == nil {
			return 0
		}

		if !inlined {
			flags = flags.without(inline | toplevel)
		} else {
			flags = flags.without(toplevel)
		}
		n := 0

		for _, f := range unique {
			size := f.codec.size(f.pointer(p), f.makeFlags(flags))
			if size > 0 {
				n += int(f.tagsize) + size
				if f.embedded() {
					n += sizeOfVarint(uint64(size))
				}
				flags = flags.without(wantzero)
			}
		}

		for _, f := range repeated {
			size := f.codec.size(f.pointer(p), f.makeFlags(flags))
			if size > 0 {
				n += size
				flags = flags.without(wantzero)
			}
		}

		return n
	}
}

func structEncodeFuncOf(t reflect.Type, fields []structField) encodeFunc {
	var inlined = inlined(t)
	var unique, repeated []*structField

	for i := range fields {
		f := &fields[i]
		if f.repeated() {
			repeated = append(repeated, f)
		} else {
			unique = append(unique, f)
		}
	}

	return func(b []byte, p unsafe.Pointer, flags flags) ([]byte, error) {
		if p == nil {
			return b, nil
		}

		if !inlined {
			flags = flags.without(inline | toplevel)
		} else {
			flags = flags.without(toplevel)
		}

		var err error
		for _, f := range unique {
			fieldFlags := f.makeFlags(flags)
			elem := f.pointer(p)
			size := f.codec.size(elem, fieldFlags)

			if size > 0 {
				b = appendVarint(b, f.wiretag)

				if f.embedded() {
					b = appendVarint(b, uint64(size))
				}

				b, err = f.codec.encode(b, elem, fieldFlags)
				if err != nil {
					return b, err
				}

				flags = flags.without(wantzero)
			}
		}

		for _, f := range repeated {
			b, err = f.codec.encode(b, f.pointer(p), f.makeFlags(flags))
			if err != nil {
				return b, err
			}
		}

		return b, nil
	}
}

func structDecodeFuncOf(_ reflect.Type, fields []structField) decodeFunc {
	fieldIndex := make(map[fieldNumber]*structField, len(fields))

	for i := range fields {
		f := &fields[i]
		fieldIndex[f.fieldNumber()] = f
	}

	return func(b []byte, p unsafe.Pointer, flags flags) (int, error) {
		flags = flags.without(toplevel)
		offset := 0

		for offset < len(b) {
			fieldNumber, wireType, n, err := decodeTag(b[offset:])
			offset += n
			if err != nil {
				return offset, err
			}

			f := fieldIndex[fieldNumber]
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

			n, err = f.codec.decode(data, f.pointer(p), f.makeFlags(flags))
			offset += n
			if err != nil {
				return offset, fieldError(fieldNumber, wireType, err)
			}
		}

		return offset, nil
	}
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
