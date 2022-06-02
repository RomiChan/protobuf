package proto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/RomiChan/protobuf/proto"
	"github.com/RomiChan/protobuf/proto/internal/testproto"
)

func FuzzMarshal1(f *testing.F) {
	f.Fuzz(func(t *testing.T, i int32, j int64, s string) {
		input := testproto.Proto2{
			Nested: &testproto.Proto2_NestedMessage{
				Int32Val:  Some(i),
				Int64Val:  Some(j),
				StringVal: Some(s),
			},
		}
		b, err := Marshal(&input)
		assert.NoError(t, err)

		var output testproto.Proto2
		assert.NoError(t, Unmarshal(b, &output))
		assert.Equal(t, input, output)
	})
}

func FuzzMarshalNested(f *testing.F) {
	type message4 struct {
		Value string `protobuf:"bytes,1,opt"`
	}
	type message3 struct {
		Value  string    `protobuf:"bytes,1,opt"`
		Nested *message4 `protobuf:"bytes,2,opt"`
	}
	type message2 struct {
		Value  string    `protobuf:"bytes,1,opt"`
		Nested *message3 `protobuf:"bytes,2,opt"`
	}
	type message1 struct {
		Value  string    `protobuf:"bytes,1,opt"`
		Nested *message2 `protobuf:"bytes,2,opt"`
	}
	type message struct {
		Value  string    `protobuf:"bytes,1,opt"`
		Nested *message1 `protobuf:"bytes,2,opt"`
	}
	f.Fuzz(func(t *testing.T, s, s1, s2, s3, s4 string) {
		input := message{
			Value: s,
			Nested: &message1{
				Value: s1,
				Nested: &message2{
					Value: s2,
					Nested: &message3{
						Value: s3,
						Nested: &message4{
							Value: s4,
						},
					},
				},
			},
		}
		b, err := Marshal(&input)
		assert.NoError(t, err)

		var output message
		assert.NoError(t, Unmarshal(b, &output))
		assert.Equal(t, input, output)
	})
}
