package proto

import (
	"reflect"
	"unsafe"

	"github.com/RomiChan/protobuf/internal/vm"
)

var (
	optionBoolType    = reflect.TypeOf((*Option[bool])(nil)).Elem()
	optionInt32Type   = reflect.TypeOf((*Option[int32])(nil)).Elem()
	optionInt64Type   = reflect.TypeOf((*Option[int64])(nil)).Elem()
	optionUInt32Type  = reflect.TypeOf((*Option[uint32])(nil)).Elem()
	optionUInt64Type  = reflect.TypeOf((*Option[uint64])(nil)).Elem()
	optionFloat32Type = reflect.TypeOf((*Option[float32])(nil)).Elem()
	optionFloat64Type = reflect.TypeOf((*Option[float64])(nil)).Elem()
	optionStringType  = reflect.TypeOf((*Option[string])(nil)).Elem()
)

type walker struct {
	codecs map[reflect.Type]*codec
	infos  map[reflect.Type]*structInfo
}

type walkerConfig struct {
	zigzag   bool
	required bool
}

func (w *walker) codec(t reflect.Type, conf *walkerConfig) *codec {
	if c, ok := w.codecs[t]; ok {
		return c
	}
	if conf.required {
		return w.required(t, conf)
	}
	switch t.Kind() {
	case reflect.Bool:
		return &boolCodec
	case reflect.Int32:
		if conf.zigzag {
			return &zigzag32Codec
		}
		return &int32Codec
	case reflect.Int64:
		if conf.zigzag {
			return &zigzag64Codec
		}
		return &int64Codec
	case reflect.Uint32:
		return &uint32Codec
	case reflect.Uint64:
		return &uint64Codec
	case reflect.Float32:
		return &float32Codec
	case reflect.Float64:
		return &float64Codec
	case reflect.String:
		if conf.required {
			return &stringRequiredCodec
		}
		return &stringCodec
	case reflect.Slice:
		if t == rawMessageType {
			return &rawCodec
		}
		elem := t.Elem()
		switch elem.Kind() {
		case reflect.Uint8:
			return &bytesCodec
		}
	case reflect.Struct:
		return w.structCodec(t)
	case reflect.Ptr:
		return w.pointer(t, conf)
	}

	panic("unsupported type: " + t.String())
}

func (w *walker) structCodec(t reflect.Type) *codec {
	if c, ok := codecCache.Load(pointer(t)); ok {
		return c.(*codec)
	}
	if c, ok := w.codecs[t]; ok {
		return c
	}
	c := new(codec)
	w.codecs[t] = c
	elem := t.Elem()
	info := w.structInfo(elem)
	c.size = func(p unsafe.Pointer, f *structField) int {
		p = deref(p)
		if p != nil {
			n := info.sizeNoTape(p)
			n += sizeOfVarint(uint64(n)) + f.tagsize
			return n
		}
		return 0
	}
	c.encode = func(b []byte, i int, p unsafe.Pointer, f *structField) int {
		p = deref(p)
		if p != nil {
			i = writeVarint(b, i, f.wiretag)
			n := info.sizeNoTape(p)
			i = writeVarint(b, i, uint64(n))
			return info.encode(b, i, p, nil)
		}
		return i
	}
	// Tape-aware variants: the body size is recorded once by the size pass
	// and replayed here, which also covers elements of repeated message
	// fields (slice codecs forward their element codec's tape funcs).
	c.isMessage = true
	c.sizeT = func(p unsafe.Pointer, f *structField, t *vm.Tape) int {
		p = deref(p)
		if p == nil {
			return 0
		}
		idx := 0
		if t != nil {
			idx = t.Reserve()
		}
		n := info.sizeTape(p, t)
		if t != nil {
			t.Set(idx, n)
		}
		return n + sizeOfVarint(uint64(n)) + f.tagsize
	}
	c.encodeT = func(b []byte, i int, p unsafe.Pointer, f *structField, t *vm.Tape) int {
		p = deref(p)
		if p == nil {
			return i
		}
		i = writeVarint(b, i, f.wiretag)
		var n int
		if t != nil {
			n = t.Next()
		} else {
			n = info.sizeNoTape(p)
		}
		i = writeVarint(b, i, uint64(n))
		return info.encode(b, i, p, t)
	}
	c.decode = func(b []byte, p unsafe.Pointer) (int, error) {
		v := (*unsafe.Pointer)(p)
		if *v == nil {
			*v = unsafe.Pointer(reflect.New(elem).Pointer())
		}
		data, n, err := decodeVarlen(b)
		if err != nil {
			return n, err
		}
		m, err := info.decode(data, *v)
		return n - len(data) + m, err
	}
	actualCodec, _ := codecCache.LoadOrStore(pointer(t), c)
	return actualCodec.(*codec)
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

// scalarKindOf classifies scalar fields (including pointer-wrapped ones) for
// the encode VM. It must mirror the codec selection in walker.structInfo and
// walker.codec exactly; vm.Fallback keeps the closure path.
func scalarKindOf(typ reflect.Type, tag structTag) vm.Kind {
	base := typ
	var flag vm.Kind
	if base.Kind() == reflect.Ptr {
		base = base.Elem()
		flag = vm.PtrFlag
	}
	switch tag.wireType {
	case fixed32:
		switch base.Kind() {
		case reflect.Float32:
			return flag | vm.Float32
		case reflect.Uint32, reflect.Int32:
			return flag | vm.Fixed32
		}
	case fixed64:
		switch base.Kind() {
		case reflect.Float64:
			return flag | vm.Float64
		case reflect.Uint64, reflect.Int64:
			return flag | vm.Fixed64
		}
	default: // varint / varlen
		switch base.Kind() {
		case reflect.Bool:
			return flag | vm.Bool
		case reflect.Int32:
			if tag.zigzag {
				return flag | vm.Zigzag32
			}
			return flag | vm.Int32
		case reflect.Int64:
			if tag.zigzag {
				return flag | vm.Zigzag64
			}
			return flag | vm.Int64
		case reflect.Uint32:
			return flag | vm.Uint32
		case reflect.Uint64:
			return flag | vm.Uint64
		case reflect.Float32:
			return flag | vm.Float32
		case reflect.Float64:
			return flag | vm.Float64
		case reflect.String:
			return flag | vm.String
		}
	}
	return vm.Fallback
}

func (w *walker) structInfo(t reflect.Type) *structInfo {
	if info, ok := structInfoCache.Load(pointer(t)); ok {
		return info
	}
	if i, ok := w.infos[t]; ok {
		return i
	}

	info := new(structInfo)
	w.infos[t] = info
	numField := t.NumField()
	fields := make([]*structField, 0, numField)
	ops := make([]vm.Field, 0, numField)
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

		// kind classifies the field for the encode VM (internal/vm); it is
		// assigned at the same site as field.codec so the two stay in sync.
		var kind vm.Kind
		var msgEnc func(b []byte, i int, p unsafe.Pointer, t *vm.Tape) int
		switch t.wireType {
		case fixed32:
			switch f.Type {
			case optionFloat32Type:
				field.codec = &float32OptionCodec
				kind = vm.OptionFlag | vm.Float32
			case optionUInt32Type, optionInt32Type:
				field.codec = &fixed32OptionCodec
				kind = vm.OptionFlag | vm.Fixed32
			}
			switch baseKindOf(f.Type) {
			case reflect.Float32:
				field.codec = &float32Codec
				kind = vm.Float32
			case reflect.Uint32, reflect.Int32:
				field.codec = &fixed32Codec
				kind = vm.Fixed32
			}
		case fixed64:
			switch f.Type {
			case optionFloat64Type:
				field.codec = &float64OptionCodec
				kind = vm.OptionFlag | vm.Float64
			case optionUInt64Type, optionInt64Type:
				field.codec = &fixed64OptionCodec
				kind = vm.OptionFlag | vm.Fixed64
			}
			switch baseKindOf(f.Type) {
			case reflect.Float64:
				field.codec = &float64Codec
				kind = vm.Float64
			case reflect.Uint64, reflect.Int64:
				field.codec = &fixed64Codec
				kind = vm.Fixed64
			}
		}
		if field.codec == nil {
			switch f.Type {
			case optionBoolType:
				field.codec = &boolOptionCodec
				kind = vm.OptionFlag | vm.Bool
			case optionInt32Type:
				field.codec = &int32OptionCodec
				kind = vm.OptionFlag | vm.Int32
				if t.zigzag {
					field.codec = &zigzag32OptionCodec
					kind = vm.OptionFlag | vm.Zigzag32
				}
			case optionInt64Type:
				field.codec = &int64OptionCodec
				kind = vm.OptionFlag | vm.Int64
				if t.zigzag {
					field.codec = &zigzag64OptionCodec
					kind = vm.OptionFlag | vm.Zigzag64
				}
			case optionUInt32Type:
				field.codec = &uint32OptionCodec
				kind = vm.OptionFlag | vm.Uint32
			case optionUInt64Type:
				field.codec = &uint64OptionCodec
				kind = vm.OptionFlag | vm.Uint64
			case optionStringType:
				field.codec = &stringOptionCodec
				kind = vm.OptionFlag | vm.String
			}
		}
		if field.codec == nil {
			conf := &walkerConfig{
				zigzag: t.zigzag,
				// required: t.required,
			}
			switch baseKindOf(f.Type) {
			case reflect.Struct:
				field.codec = w.codec(f.Type, conf)
				// Message field: tag + length prefix + recursive encode. The
				// body size comes from the tape recorded by the size pass, or
				// is recomputed when there is no tape.
				minfo, wiretag := w.structInfo(baseTypeOf(f.Type)), field.wiretag
				field.info = minfo
				info.hasMessage = true
				msgEnc = func(b []byte, i int, p unsafe.Pointer, t *vm.Tape) int {
					if mp := *(*unsafe.Pointer)(p); mp != nil {
						i = writeVarint(b, i, wiretag)
						var n int
						if t != nil {
							n = t.Next()
						} else {
							n = minfo.sizeNoTape(mp)
						}
						i = writeVarint(b, i, uint64(n))
						i = minfo.encode(b, i, mp, t)
					}
					return i
				}

			case reflect.Slice:
				if f.Type == rawMessageType {
					field.codec = &rawCodec
					kind = vm.Bytes
					break
				}
				elem := f.Type.Elem()
				if elem.Kind() == reflect.Uint8 { // []byte
					field.codec = &bytesCodec
					kind = vm.Bytes
				} else {
					conf.required = true
					field.codec = w.codec(elem, conf)
					field.codec = sliceCodecOf(f.Type, field.codec, w)
					if field.codec.sizeT != nil {
						// Repeated message field: its elements are sized and
						// encoded through the tape as well.
						info.hasMessage = true
					}
				}

			case reflect.Map:
				key, val := f.Type.Key(), f.Type.Elem()
				m := &mapField{wiretag: field.wiretag}

				t := mapElementTag(f, "protobuf_key")
				keyField := &structField{wiretag: uint64(t.fieldNumber)<<3 | uint64(t.wireType)}
				keyField.tagsize = sizeOfVarint(keyField.wiretag)
				keyField.codec = w.mapElementCodec(key, t)

				t = mapElementTag(f, "protobuf_val")
				valField := &structField{wiretag: uint64(t.fieldNumber)<<3 | uint64(t.wireType)}
				valField.tagsize = sizeOfVarint(valField.wiretag)
				valField.codec = w.mapElementCodec(val, t)

				m.keyField = keyField
				m.valField = valField
				field.codec = w.mapCodec(f.Type, m)

			default:
				field.codec = w.codec(f.Type, conf)
				kind = scalarKindOf(f.Type, t)
			}
		}
		field.tagsize = sizeOfVarint(field.wiretag)

		// Compile the encode op for the VM (internal/vm).
		op := vm.Field{
			Offset:  f.Offset,
			Wiretag: field.wiretag,
			Kind:    kind,
		}
		switch {
		case msgEnc != nil:
			op.Enc = msgEnc
		case kind == vm.Fallback:
			c, fld := field.codec, &field
			op.Enc = func(b []byte, i int, fp unsafe.Pointer, t *vm.Tape) int {
				if t != nil && c.encodeT != nil {
					return c.encodeT(b, i, fp, fld, t)
				}
				return c.encode(b, i, fp, fld)
			}
		}
		ops = append(ops, op)
		fields = append(fields, &field)
	}

	// copy to save capacity
	fields2 := make([]*structField, len(fields))
	copy(fields2, fields)
	info.fields = fields2

	ops2 := make([]vm.Field, len(ops))
	copy(ops2, ops)
	info.ops = ops2

	info.fieldIndex = make(map[fieldNumber]*structField, len(info.fields))
	maxFieldNumber := fieldNumber(0)
	for _, f := range info.fields {
		info.fieldIndex[f.fieldNumber()] = f
		if n := f.fieldNumber(); n > maxFieldNumber {
			maxFieldNumber = n
		}
	}

	// Proto field numbers are usually dense small integers; a direct-indexed
	// table avoids a hash lookup per decoded field. Only cover the field
	// numbers with a single-byte tag (<= 16); sparse or high-numbered
	// messages keep using the map.
	if maxFieldNumber > 0 && maxFieldNumber <= 16 {
		info.dense = make([]*structField, maxFieldNumber)
		for _, f := range info.fields {
			info.dense[f.fieldNumber()-1] = f
		}
		info.fieldIndex = nil
	}

	structInfoCache.Store(pointer(t), info)
	return info
}

// @@@ Pointers @@@

func deref(p unsafe.Pointer) unsafe.Pointer {
	return *(*unsafe.Pointer)(p)
}

func (w *walker) pointer(t reflect.Type, conf *walkerConfig) *codec {
	switch t.Elem().Kind() {
	case reflect.Struct:
		return w.structCodec(t)
	}
	// common value
	p := new(codec)
	w.codecs[t] = p
	c := w.codec(t.Elem(), conf)
	p.size = pointerSizeFuncOf(t, c)
	p.encode = pointerEncodeFuncOf(t, c)
	p.decode = pointerDecodeFuncOf(t, c)
	return p
}

func (w *walker) required(t reflect.Type, conf *walkerConfig) *codec {
	if c, ok := w.codecs[t]; ok {
		return c
	}

	switch t.Kind() {
	case reflect.Bool:
		return &boolRequiredCodec
	case reflect.Int32:
		if conf.zigzag {
			return &zigzag32RequiredCodec
		}
		return &int32RequiredCodec
	case reflect.Int64:
		if conf.zigzag {
			return &zigzag64RequiredCodec
		}
		return &int64RequiredCodec
	case reflect.Uint32:
		return &uint32RequiredCodec
	case reflect.Uint64:
		return &uint64RequiredCodec
	case reflect.Float32:
		return &float32RequiredCodec
	case reflect.Float64:
		return &float64RequiredCodec
	case reflect.String:
		return &stringRequiredCodec
	case reflect.Slice:
		if t == rawMessageType {
			return &rawCodec
		}
		elem := t.Elem()
		switch elem.Kind() {
		case reflect.Uint8:
			return &bytesCodec
		}
	case reflect.Struct:
		panic("nested message must be pointer:" + t.String())
	case reflect.Ptr:
		return w.pointer(t, conf)
	}

	panic("unsupported type: " + t.String())
}

func pointerSizeFuncOf(_ reflect.Type, c *codec) sizeFunc {
	return func(p unsafe.Pointer, f *structField) int {
		p = deref(p)
		if p != nil {
			return c.size(p, f)
		}
		return 0
	}
}

func pointerEncodeFuncOf(_ reflect.Type, c *codec) encodeFunc {
	return func(b []byte, i int, p unsafe.Pointer, f *structField) int {
		p = deref(p)
		if p != nil {
			return c.encode(b, i, p, f)
		}
		return i
	}
}

func pointerDecodeFuncOf(t reflect.Type, c *codec) decodeFunc {
	t = t.Elem()
	return func(b []byte, p unsafe.Pointer) (int, error) {
		v := (*unsafe.Pointer)(p)
		if *v == nil {
			*v = unsafe.Pointer(reflect.New(t).Pointer())
		}
		return c.decode(b, *v)
	}
}
