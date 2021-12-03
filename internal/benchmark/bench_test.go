package benchmark

import (
	"testing"

	"google.golang.org/protobuf/proto"

	proto2 "github.com/RomiChan/protobuf/proto"
)

var (
	PBSmall = &BenchSmall{
		Action: proto.String("benchmark"),
		Key:    []byte("data to be sent"),
	}

	PBMedium = &BenchMedium{
		Name:   proto.String("Tester"),
		Age:    proto.Int64(20),
		Height: proto.Float32(5.8),
		Weight: proto.Float64(180.7),
		Alive:  proto.Bool(true),
		Desc: []byte(`If you’ve ever heard of ProtoBuf you may be thinking 
		that the results of this benchmarking experiment will be obvious,
		JSON < ProtoBuf.`),
	}

	PBLarge = &BenchLarge{
		Name:     proto.String("Tester"),
		Age:      proto.Int64(20),
		Height:   proto.Float32(5.8),
		Weight:   proto.Float64(180.7),
		Alive:    proto.Bool(true),
		Desc:     []byte("Lets benchmark some json and protobuf"),
		Nickname: proto.String("Another name"),
		Num:      proto.Int64(2314),
		Flt:      proto.Float32(123451231.1234),
		Data: []byte(`If you’ve ever heard of ProtoBuf you may be thinking that
		the results of this benchmarking experiment will be obvious, JSON < ProtoBuf.
		My interest was in how much they actually differ in practice.
		How do they compare on a couple of different metrics, specifically serialization
		and de-serialization speeds, and the memory footprint of encoding the data.
		I was also curious about how the different serialization methods would
		behave under small, medium, and large chunks of data.`),
	}

	PBNested = &BenchNested{
		Small:  PBSmall,
		Medium: PBMedium,
		Large:  PBLarge,
	}
)

func BenchmarkGoogleProtobufMarshal(b *testing.B) {
	s := PBSmall
	m := PBMedium
	l := PBLarge
	nested := PBNested

	b.ResetTimer()

	b.Run("SmallData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto.Marshal(s)
		}
		b.SetBytes(int64(len(d)))
	})
	b.Run("MediumData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto.Marshal(m)
		}
		b.SetBytes(int64(len(d)))
	})
	b.Run("LargeData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto.Marshal(l)
		}
		b.SetBytes(int64(len(d)))
	})
	b.Run("AllData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto.Marshal(nested)
		}
		b.SetBytes(int64(len(d)))
	})
}

func BenchmarkRomiChanProtobufMarshal(b *testing.B) {
	s := PBSmall
	m := PBMedium
	l := PBLarge
	nested := PBNested

	b.ResetTimer()

	b.Run("SmallData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto2.Marshal(s)
		}
		b.SetBytes(int64(len(d)))
	})
	b.Run("MediumData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto2.Marshal(m)
		}
		b.SetBytes(int64(len(d)))
	})
	b.Run("LargeData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto2.Marshal(l)
		}
		b.SetBytes(int64(len(d)))
	})
	b.Run("AllData", func(b *testing.B) {
		b.ReportAllocs()
		var d []byte
		for n := 0; n < b.N; n++ {
			d, _ = proto2.Marshal(nested)
		}
		b.SetBytes(int64(len(d)))
	})
}

func BenchmarkGoogleProtobufUnmarshal(b *testing.B) {
	s := PBSmall
	m := PBMedium
	l := PBLarge

	sd, _ := proto.Marshal(s)
	md, _ := proto.Marshal(m)
	ld, _ := proto.Marshal(l)

	var sf BenchSmall
	var mf BenchMedium
	var lf BenchLarge

	b.ResetTimer()

	b.Run("SmallData", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(sd)))
		for n := 0; n < b.N; n++ {
			_ = proto.Unmarshal(sd, &sf)
		}
	})
	b.Run("MediumData", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(md)))
		for n := 0; n < b.N; n++ {
			_ = proto.Unmarshal(md, &mf)
		}
	})
	b.Run("LargeData", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(ld)))
		for n := 0; n < b.N; n++ {
			_ = proto.Unmarshal(ld, &lf)
		}
	})
}

func BenchmarkRomiChanProtobufUnmarshal(b *testing.B) {
	s := PBSmall
	m := PBMedium
	l := PBLarge

	sd, _ := proto.Marshal(s)
	md, _ := proto.Marshal(m)
	ld, _ := proto.Marshal(l)

	var sf BenchSmall
	var mf BenchMedium
	var lf BenchLarge

	b.ResetTimer()

	b.Run("SmallData", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(sd)))
		for n := 0; n < b.N; n++ {
			_ = proto2.Unmarshal(sd, &sf)
		}
	})
	b.Run("MediumData", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(md)))
		for n := 0; n < b.N; n++ {
			_ = proto2.Unmarshal(md, &mf)
		}
	})
	b.Run("LargeData", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(ld)))
		for n := 0; n < b.N; n++ {
			_ = proto2.Unmarshal(ld, &lf)
		}
	})
}
