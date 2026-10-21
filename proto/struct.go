package proto

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"

	"github.com/RomiChan/protobuf/internal/vm"
)

type structInfo struct {
	fields     []*structField
	ops        []vm.Field
	fieldIndex map[fieldNumber]*structField
	// dense is indexed by fieldNumber-1 when the field numbers are small
	// enough; nil entries are holes. It replaces the hash lookup on the
	// decode hot path.
	dense []*structField
	// hasMessage reports whether any field is a nested message, i.e. whether
	// a size tape is needed to avoid recomputing subtree sizes.
	hasMessage bool
}

type structField struct {
	offset  uintptr
	wiretag uint64
	codec   *codec
	tagsize int
	// info is the nested message's structInfo, set only for message fields.
	info *structInfo
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

// sizeNoTape is the hot path used by Size and by every subtree below a leaf
// message; it is kept free of tape bookkeeping.
func (info *structInfo) sizeNoTape(p unsafe.Pointer) int {
	if p == nil {
		return 0
	}
	n := 0
	for _, f := range info.fields {
		fp := f.pointer(p)
		if f.info != nil {
			if mp := *(*unsafe.Pointer)(fp); mp != nil {
				cn := f.info.sizeNoTape(mp)
				n += f.tagsize + sizeOfVarint(uint64(cn)) + cn
			}
			continue
		}
		n += f.codec.size(fp, f)
	}
	return n
}

// sizeTape records the body size of every nested message on t (in preorder,
// via reserve-then-set) so the encode pass can replay it instead of recomputing
// the subtree.
func (info *structInfo) sizeTape(p unsafe.Pointer, t *vm.Tape) int {
	n := 0
	for _, f := range info.fields {
		fp := f.pointer(p)
		if f.info != nil {
			if mp := *(*unsafe.Pointer)(fp); mp != nil {
				idx := t.Reserve()
				cn := f.info.sizeTape(mp, t)
				t.Set(idx, cn)
				n += f.tagsize + sizeOfVarint(uint64(cn)) + cn
			}
			continue
		}
		if f.codec.sizeT != nil {
			n += f.codec.sizeT(fp, f, t)
		} else {
			n += f.codec.size(fp, f)
		}
	}
	return n
}

func (info *structInfo) encode(b []byte, i int, p unsafe.Pointer, t *vm.Tape) int {
	if p == nil {
		return i
	}
	return vm.Encode(b, i, p, info.ops, t)
}

func (info *structInfo) decode(b []byte, p unsafe.Pointer) (int, error) {
	offset := 0
	for offset < len(b) {
		fieldNumber, wireType, n, err := decodeTag(b[offset:])
		offset += n
		if err != nil {
			return offset, err
		}

		var f *structField
		if d := info.dense; d != nil {
			if n := uint(fieldNumber) - 1; n < uint(len(d)) {
				f = d[n]
			}
		} else {
			f = info.fieldIndex[fieldNumber]
		}
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
