package avrobench

import (
	"testing"

	hamba "github.com/hamba/avro/v2"
	hambasoe "github.com/hamba/avro/v2/soe"
	isko "github.com/iskorotkov/avro/v2"
	iskosoe "github.com/iskorotkov/avro/v2/soe"
	goavro "github.com/linkedin/goavro/v2"
	twmb "github.com/twmb/avro"
)

// BenchmarkEncodingDecode covers the three wire encodings bento's
// internal/impl/avro/processor.go asks goavro for: binary (the baseline
// every other benchmark in this repo already measures), textual (Avro JSON,
// goavro.NativeFromTextual) and single-object (the 0xC3 0x01 magic plus an
// 8-byte CRC-64-AVRO schema fingerprint, goavro.NativeFromSingle). Binary is
// the only one universal across all four libraries, so this doubles as a
// feature-parity probe: an unsupported arm records a Skip with a precise
// reason rather than being left out of the table.
func BenchmarkEncodingDecode(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			b.Run("binary", func(b *testing.B) {
				b.Run("hamba", func(b *testing.B) {
					s := hamba.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := hamba.Unmarshal(s, c.Payload, c.NewTyped()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("iskorotkov", func(b *testing.B) {
					s := isko.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := isko.Unmarshal(s, c.Payload, c.NewTyped()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("twmb", func(b *testing.B) {
					s := twmb.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := s.Decode(c.Payload, c.NewTyped()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("goavro", func(b *testing.B) {
					codec, err := goavro.NewCodec(c.SchemaJSON)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, _, err := codec.NativeFromBinary(c.Payload); err != nil {
							b.Fatal(err)
						}
					}
				})
			})

			// Textual is Avro's JSON encoding. hamba (and its iskorotkov fork)
			// only ever produce binary — Marshal/Unmarshal has no JSON mode — so
			// those two arms are a parity result, not a number.
			b.Run("textual", func(b *testing.B) {
				b.Run("hamba", func(b *testing.B) {
					b.Skipf("hamba/avro v2.31.0 has no Avro JSON (textual) encoding: Marshal/Unmarshal only produce binary")
				})

				b.Run("iskorotkov", func(b *testing.B) {
					b.Skipf("iskorotkov/avro is a hamba fork and carries the same gap: no Avro JSON (textual) encoding")
				})

				b.Run("twmb", func(b *testing.B) {
					s := twmb.MustParse(c.SchemaJSON)
					// TaggedUnions matches the Avro JSON spec's {"branch": value}
					// envelope, which is the only form goavro produces below —
					// without it the two libraries would disagree on what
					// "textual encoding" even means for a union field.
					payload, err := s.EncodeJSON(c.Value, twmb.TaggedUnions())
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := s.DecodeJSON(payload, c.NewTyped(), twmb.TaggedUnions()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("goavro", func(b *testing.B) {
					codec, err := goavro.NewCodec(c.SchemaJSON)
					if err != nil {
						b.Fatal(err)
					}
					// goavro has no typed struct mode, so the textual fixture is
					// built from the native value the fixture's own binary bytes
					// decode to, not from c.Value directly.
					native, _, err := codec.NativeFromBinary(c.Payload)
					if err != nil {
						b.Fatal(err)
					}
					payload, err := codec.TextualFromNative(nil, native)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, _, err := codec.NativeFromTextual(payload); err != nil {
							b.Fatal(err)
						}
					}
				})
			})

			// Single-object encoding is the 0xC3 0x01 magic plus an 8-byte
			// CRC-64-AVRO schema fingerprint framing a binary payload — the
			// format goavro.NativeFromSingle/SingleFromNative implement.
			b.Run("single-object", func(b *testing.B) {
				b.Run("hamba", func(b *testing.B) {
					s := hamba.MustParse(c.SchemaJSON)
					codec, err := hambasoe.NewCodec(s)
					if err != nil {
						b.Fatal(err)
					}
					payload, err := codec.Encode(c.Value)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := codec.Decode(payload, c.NewTyped()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("iskorotkov", func(b *testing.B) {
					s := isko.MustParse(c.SchemaJSON)
					codec, err := iskosoe.NewCodec(s)
					if err != nil {
						b.Fatal(err)
					}
					payload, err := codec.Encode(c.Value)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := codec.Decode(payload, c.NewTyped()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("twmb", func(b *testing.B) {
					s := twmb.MustParse(c.SchemaJSON)
					payload, err := s.AppendSingleObject(nil, c.Value)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := s.DecodeSingleObject(payload, c.NewTyped()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("goavro", func(b *testing.B) {
					codec, err := goavro.NewCodec(c.SchemaJSON)
					if err != nil {
						b.Fatal(err)
					}
					native, _, err := codec.NativeFromBinary(c.Payload)
					if err != nil {
						b.Fatal(err)
					}
					payload, err := codec.SingleFromNative(nil, native)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, _, err := codec.NativeFromSingle(payload); err != nil {
							b.Fatal(err)
						}
					}
				})
			})
		})
	}
}

// BenchmarkEncodingEncode is the write side of BenchmarkEncodingDecode: same
// three encodings, same skip convention for the arms a library cannot do.
func BenchmarkEncodingEncode(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			b.Run("binary", func(b *testing.B) {
				b.Run("hamba", func(b *testing.B) {
					s := hamba.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := hamba.Marshal(s, c.Value); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("iskorotkov", func(b *testing.B) {
					s := isko.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := isko.Marshal(s, c.Value); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("twmb", func(b *testing.B) {
					s := twmb.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := s.Encode(c.Value); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("goavro", func(b *testing.B) {
					codec, err := goavro.NewCodec(c.SchemaJSON)
					if err != nil {
						b.Fatal(err)
					}
					native, _, err := codec.NativeFromBinary(c.Payload)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := codec.BinaryFromNative(nil, native); err != nil {
							b.Fatal(err)
						}
					}
				})
			})

			b.Run("textual", func(b *testing.B) {
				b.Run("hamba", func(b *testing.B) {
					b.Skipf("hamba/avro v2.31.0 has no Avro JSON (textual) encoding: Marshal/Unmarshal only produce binary")
				})

				b.Run("iskorotkov", func(b *testing.B) {
					b.Skipf("iskorotkov/avro is a hamba fork and carries the same gap: no Avro JSON (textual) encoding")
				})

				b.Run("twmb", func(b *testing.B) {
					s := twmb.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := s.EncodeJSON(c.Value, twmb.TaggedUnions()); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("goavro", func(b *testing.B) {
					codec, err := goavro.NewCodec(c.SchemaJSON)
					if err != nil {
						b.Fatal(err)
					}
					native, _, err := codec.NativeFromBinary(c.Payload)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := codec.TextualFromNative(nil, native); err != nil {
							b.Fatal(err)
						}
					}
				})
			})

			b.Run("single-object", func(b *testing.B) {
				b.Run("hamba", func(b *testing.B) {
					s := hamba.MustParse(c.SchemaJSON)
					codec, err := hambasoe.NewCodec(s)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := codec.Encode(c.Value); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("iskorotkov", func(b *testing.B) {
					s := isko.MustParse(c.SchemaJSON)
					codec, err := iskosoe.NewCodec(s)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := codec.Encode(c.Value); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("twmb", func(b *testing.B) {
					s := twmb.MustParse(c.SchemaJSON)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := s.AppendSingleObject(nil, c.Value); err != nil {
							b.Fatal(err)
						}
					}
				})

				b.Run("goavro", func(b *testing.B) {
					codec, err := goavro.NewCodec(c.SchemaJSON)
					if err != nil {
						b.Fatal(err)
					}
					native, _, err := codec.NativeFromBinary(c.Payload)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := codec.SingleFromNative(nil, native); err != nil {
							b.Fatal(err)
						}
					}
				})
			})
		})
	}
}
