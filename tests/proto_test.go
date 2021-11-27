package tests

import (
	"testing"

	"github.com/RomiChan/protobuf/proto"
)

func TestPrivateField(t *testing.T) {
	type private struct {
		a uint64 `protobuf:"varint,1,opt"`
		b uint64 `protobuf:"varint,2,opt"`
	}
	type public struct {
		A uint64 `protobuf:"varint,1,opt"`
		B uint64 `protobuf:"varint,2,opt"`
	}
	s := &private{1, 2}
	if proto.Size(s) != 0 {
		t.Errorf("Size of private field should be zero.")
	}
	data, err := proto.Marshal(s)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("Marshal of private field should be empty.")
	}

	data, err = proto.Marshal(&public{2, 1})
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("Marshal of public field should not be empty.")
	}
	err = proto.Unmarshal(data, s)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if s.a == 2 || s.b == 1 {
		t.Errorf("Unmarshal of private field: %v", s)
	}
}
