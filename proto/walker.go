package proto

import (
	"reflect"
	"unsafe"
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
			n := info.size(p) + f.tagsize
			n += sizeOfVarint(uint64(n))
			return n
		}
		return 0
	}
	c.encode = func(b []byte, p unsafe.Pointer, f *structField) []byte {
		p = deref(p)
		if p != nil {
			b = appendVarint(b, f.wiretag)
			n := info.size(p)
			b = appendVarint(b, uint64(n))
			return info.encode(b, p)
		}
		return b
	}
	c.decode = func(b []byte, p unsafe.Pointer) (int, error) {
		v := (*unsafe.Pointer)(p)
		if *v == nil {
			*v = unsafe.Pointer(reflect.New(elem).Pointer())
		}
		_, n, err := decodeVarint(b)
		if err != nil {
			return n, err
		}
		l, err := info.decode(b[n:], *v)
		return n + l, err
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
		switch t.wireType {
		case fixed32:
			switch baseKindOf(f.Type) {
			case reflect.Uint32:
				if f.Type.Kind() == reflect.Ptr {
					field.codec = &fixed32PtrCodec
				} else {
					field.codec = &fixed32Codec
				}
			case reflect.Float32:
				if f.Type.Kind() == reflect.Ptr {
					field.codec = &float32PtrCodec
				} else {
					field.codec = &float32Codec
				}
			}
		case fixed64:
			switch baseKindOf(f.Type) {
			case reflect.Uint64:
				if f.Type.Kind() == reflect.Ptr {
					field.codec = &fixed64PtrCodec
				} else {
					field.codec = &fixed64Codec
				}
			case reflect.Float64:
				if f.Type.Kind() == reflect.Ptr {
					field.codec = &float64PtrCodec
				} else {
					field.codec = &float64Codec
				}
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

			case reflect.Slice:
				elem := f.Type.Elem()
				if elem.Kind() == reflect.Uint8 { // []byte
					field.codec = &bytesCodec
				} else {
					conf.required = true
					field.codec = w.codec(elem, conf)
					field.codec = sliceCodecOf(f.Type, field.codec, w)
				}

			case reflect.Map:
				conf.required = true // map key and val should be encoded always
				key, val := f.Type.Key(), f.Type.Elem()
				m := &mapField{wiretag: field.wiretag}

				t, _ := parseStructTag(f.Tag.Get("protobuf_key"))
				keyField := &structField{wiretag: uint64(t.fieldNumber)<<3 | uint64(t.wireType)}
				keyField.tagsize = sizeOfVarint(keyField.wiretag)
				conf.zigzag = t.zigzag
				keyField.codec = w.codec(key, conf)

				t, _ = parseStructTag(f.Tag.Get("protobuf_val"))
				valFiled := &structField{wiretag: uint64(t.fieldNumber)<<3 | uint64(t.wireType)}
				valFiled.tagsize = sizeOfVarint(valFiled.wiretag)
				conf.zigzag = t.zigzag
				valFiled.codec = w.codec(val, conf)

				m.keyField = keyField
				m.valField = valFiled
				field.codec = w.mapCodec(f.Type, m)

			default:
				field.codec = w.codec(f.Type, conf)
			}
		}
		field.tagsize = sizeOfVarint(field.wiretag)
		fields = append(fields, &field)
	}

	// copy to save capacity
	fields2 := make([]*structField, len(fields))
	copy(fields2, fields)
	info.fields = fields2

	info.fieldIndex = make(map[fieldNumber]*structField, len(info.fields))
	for _, f := range info.fields {
		info.fieldIndex[f.fieldNumber()] = f
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
	case reflect.Bool:
		return &boolPtrCodec
	case reflect.Int32:
		if conf.zigzag {
			return &zigzag32PtrCodec
		}
		return &int32PtrCodec
	case reflect.Int64:
		if conf.zigzag {
			return &zigzag64PtrCodec
		}
		return &int64PtrCodec
	case reflect.Uint32:
		return &uint32PtrCodec
	case reflect.Uint64:
		return &uint64PtrCodec
	case reflect.Float32:
		return &float32PtrCodec
	case reflect.Float64:
		return &float64PtrCodec
	case reflect.String:
		return &stringPtrCodec
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
		if p != nil {
			p = *(*unsafe.Pointer)(p)
			return c.size(p, f)
		}
		return 0
	}
}

func pointerEncodeFuncOf(_ reflect.Type, c *codec) encodeFunc {
	return func(b []byte, p unsafe.Pointer, f *structField) []byte {
		if p != nil {
			p = deref(p)
			return c.encode(b, p, f)
		}
		return b
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
