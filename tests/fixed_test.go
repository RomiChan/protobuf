package tests

import (
	"testing"

	"github.com/RomiChan/protobuf/proto"
)

func TestFixed(t *testing.T) {
	type message struct {
		Fixed32 uint32 `protobuf:"fixed32,1,opt"`
		Fixed64 uint64 `protobuf:"fixed64,2,opt"`
	}
	m := &message{
		Fixed32: 0x01020304,
		Fixed64: 0x0102030405060708,
	}
	if proto.Size(m) != 14 { // 1+4+1+8
		t.Fatalf("Size of struct with fixed32 and fixed64 fields is not 14.")
	}
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	var m2 message
	if err := proto.Unmarshal(b, &m2); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}
	if m2.Fixed32 != 0x01020304 {
		t.Errorf("m2.Fixed32 = %x, want 0x01020304", m2.Fixed32)
	}
	if m2.Fixed64 != 0x0102030405060708 {
		t.Errorf("m2.Fixed64 = %x, want 0x0102030405060708", m2.Fixed64)
	}
}

func TestFixedPointer(t *testing.T) {
	type message struct {
		Fixed32 *uint32 `protobuf:"fixed32,1,opt"`
		Fixed64 *uint64 `protobuf:"fixed64,2,opt"`
	}
	var fixed32 uint32 = 0x01020304
	var fixed64 uint64 = 0x0102030405060708
	m := &message{
		Fixed32: &fixed32,
		Fixed64: &fixed64,
	}
	if proto.Size(m) != 14 { // 1+4+1+8
		t.Fatalf("Size of struct with fixed32 and fixed64 fields is not 14.")
	}
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	var m2 message
	if err := proto.Unmarshal(b, &m2); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}
	if *m2.Fixed32 != 0x01020304 {
		t.Errorf("m2.Fixed32 = %x, want 0x01020304", m2.Fixed32)
	}
	if *m2.Fixed64 != 0x0102030405060708 {
		t.Errorf("m2.Fixed64 = %x, want 0x0102030405060708", m2.Fixed64)
	}
}
