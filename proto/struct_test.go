package proto

import (
	"testing"
	"unsafe"
)

func TestStructFieldSize(t *testing.T) {
	t.Log("sizeof(structField) =", unsafe.Sizeof(structField{}))
}

/*
func TestParseStructTag(t *testing.T) {
	tests := []struct {
		str string
		tag structTag
	}{
		{
			str: `bytes,1,rep,name=next,proto3`,
			tag: structTag{
				name:        "next",
				version:     3,
				wireType:    varlen,
				fieldNumber: 1,
				extensions:  map[string]string{},
				repeated:    true,
			},
		},

		{
			str: `bytes,5,opt,name=key,proto3`,
			tag: structTag{
				name:        "key",
				version:     3,
				wireType:    varlen,
				fieldNumber: 5,
				extensions:  map[string]string{},
			},
		},

		{
			str: `fixed64,6,opt,name=seed,proto3`,
			tag: structTag{
				name:        "seed",
				version:     3,
				wireType:    fixed64,
				fieldNumber: 6,
				extensions:  map[string]string{},
			},
		},

		{
			str: `varint,8,opt,name=expire_after,json=expireAfter,proto3`,
			tag: structTag{
				name:        "expire_after",
				json:        "expireAfter",
				version:     3,
				wireType:    varint,
				fieldNumber: 8,
				extensions:  map[string]string{},
			},
		},

		{
			str: `bytes,17,opt,name=batch_key,json=batchKey,proto3,customtype=U128`,
			tag: structTag{
				name:        "batch_key",
				json:        "batchKey",
				version:     3,
				wireType:    varlen,
				fieldNumber: 17,
				extensions: map[string]string{
					"customtype": "U128",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.str, func(t *testing.T) {
			tag, err := parseStructTag(test.str)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tag, test.tag) {
				t.Errorf("struct tag mismatch\nwant: %+v\ngot: %+v", test.tag, tag)
			}
		})
	}
}
*/
