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
// 8-byte CRC-64-AVRO schema fingerprint, goavro.NativeFromSingle).
//
// Every arm is measured under both "typed" (decode into a generated Go
// struct) and "dynamic" (decode into map[string]any). goavro has no typed
// struct mode at all, so its typed arm is a Skip, not an omission: putting
// goavro's only mode in the same row as hamba/iskorotkov/twmb's typed numbers
// is exactly the "different work, same table" comparison this repo exists to
// call out (see README's account of the benchmark that circulates today), so
// typed and dynamic never share a row here. Where an encoding itself is
// missing (hamba/iskorotkov have no textual mode), both modes Skip with the
// same reason.
func BenchmarkEncodingDecode(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			hambaSchema := hamba.MustParse(c.SchemaJSON)
			iskoSchema := isko.MustParse(c.SchemaJSON)
			twmbSchema := twmb.MustParse(c.SchemaJSON)
			goavroCodec, err := goavro.NewCodec(c.SchemaJSON)
			if err != nil {
				b.Fatal(err)
			}

			b.Run("binary", func(b *testing.B) {
				b.Run("typed", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if err := hamba.Unmarshal(hambaSchema, c.Payload, c.NewTyped()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if err := isko.Unmarshal(iskoSchema, c.Payload, c.NewTyped()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("twmb", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.Decode(c.Payload, c.NewTyped()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.Skipf("goavro has no typed struct mode; see the dynamic arm")
					})
				})

				b.Run("dynamic", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var out any
							if err := hamba.Unmarshal(hambaSchema, c.Payload, &out); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var out any
							if err := isko.Unmarshal(iskoSchema, c.Payload, &out); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("twmb", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var out any
							if _, err := twmbSchema.Decode(c.Payload, &out); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, _, err := goavroCodec.NativeFromBinary(c.Payload); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			})

			// Textual is Avro's JSON encoding. hamba (and its iskorotkov fork)
			// only ever produce binary — Marshal/Unmarshal has no JSON mode —
			// so both modes Skip with the same reason for those two.
			b.Run("textual", func(b *testing.B) {
				b.Run("typed", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.Skipf("hamba/avro v2.31.0 has no Avro JSON (textual) encoding: Marshal/Unmarshal only produce binary")
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.Skipf("iskorotkov/avro is a hamba fork and carries the same gap: no Avro JSON (textual) encoding")
					})

					b.Run("twmb", func(b *testing.B) {
						// TaggedUnions matches the Avro JSON spec's {"branch": value}
						// envelope, which is the only form goavro produces below —
						// without it "textual encoding" would mean two different wire
						// formats for a union field.
						payload, err := twmbSchema.EncodeJSON(c.Value, twmb.TaggedUnions())
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if err := twmbSchema.DecodeJSON(payload, c.NewTyped(), twmb.TaggedUnions()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.Skipf("goavro has no typed struct mode; see the dynamic arm")
					})
				})

				b.Run("dynamic", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.Skipf("hamba/avro v2.31.0 has no Avro JSON (textual) encoding: Marshal/Unmarshal only produce binary")
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.Skipf("iskorotkov/avro is a hamba fork and carries the same gap: no Avro JSON (textual) encoding")
					})

					b.Run("twmb", func(b *testing.B) {
						// twmb's plain (untagged) dynamic decode gives back
						// unwrapped union values, same as hamba/iskorotkov above,
						// so the fixture used to build payload comes from that
						// path, not from goavro's wrapped native shape.
						var native any
						if _, err := twmbSchema.Decode(c.Payload, &native); err != nil {
							b.Fatal(err)
						}
						payload, err := twmbSchema.EncodeJSON(native, twmb.TaggedUnions())
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var out any
							if err := twmbSchema.DecodeJSON(payload, &out, twmb.TaggedUnions()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						// goavro has no typed struct mode, so the textual fixture
						// is built from the native value the fixture's own binary
						// bytes decode to, not from c.Value directly.
						native, _, err := goavroCodec.NativeFromBinary(c.Payload)
						if err != nil {
							b.Fatal(err)
						}
						payload, err := goavroCodec.TextualFromNative(nil, native)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, _, err := goavroCodec.NativeFromTextual(payload); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			})

			// Single-object encoding is the 0xC3 0x01 magic plus an 8-byte
			// CRC-64-AVRO schema fingerprint framing a binary payload — the
			// format goavro.NativeFromSingle/SingleFromNative implement.
			b.Run("single-object", func(b *testing.B) {
				b.Run("typed", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						codec, err := hambasoe.NewCodec(hambaSchema)
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
						codec, err := iskosoe.NewCodec(iskoSchema)
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
						payload, err := twmbSchema.AppendSingleObject(nil, c.Value)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.DecodeSingleObject(payload, c.NewTyped()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.Skipf("goavro has no typed struct mode; see the dynamic arm")
					})
				})

				b.Run("dynamic", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						codec, err := hambasoe.NewCodec(hambaSchema)
						if err != nil {
							b.Fatal(err)
						}
						var native any
						if err := hamba.Unmarshal(hambaSchema, c.Payload, &native); err != nil {
							b.Fatal(err)
						}
						payload, err := codec.Encode(native)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var out any
							if err := codec.Decode(payload, &out); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("iskorotkov", func(b *testing.B) {
						codec, err := iskosoe.NewCodec(iskoSchema)
						if err != nil {
							b.Fatal(err)
						}
						var native any
						if err := isko.Unmarshal(iskoSchema, c.Payload, &native); err != nil {
							b.Fatal(err)
						}
						payload, err := codec.Encode(native)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var out any
							if err := codec.Decode(payload, &out); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("twmb", func(b *testing.B) {
						var native any
						if _, err := twmbSchema.Decode(c.Payload, &native); err != nil {
							b.Fatal(err)
						}
						payload, err := twmbSchema.AppendSingleObject(nil, native)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var out any
							if _, err := twmbSchema.DecodeSingleObject(payload, &out); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						native, _, err := goavroCodec.NativeFromBinary(c.Payload)
						if err != nil {
							b.Fatal(err)
						}
						payload, err := goavroCodec.SingleFromNative(nil, native)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, _, err := goavroCodec.NativeFromSingle(payload); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			})
		})
	}
}

// BenchmarkEncodingEncode is the write side of BenchmarkEncodingDecode: same
// three encodings, same typed/dynamic split, same Skip convention for arms a
// library cannot do.
func BenchmarkEncodingEncode(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			hambaSchema := hamba.MustParse(c.SchemaJSON)
			iskoSchema := isko.MustParse(c.SchemaJSON)
			twmbSchema := twmb.MustParse(c.SchemaJSON)
			goavroCodec, err := goavro.NewCodec(c.SchemaJSON)
			if err != nil {
				b.Fatal(err)
			}

			// Dynamic encode arms need a native source value, the same way the
			// dynamic decode arms above need a native destination. Each library
			// decodes the fixture's own binary payload through its own dynamic
			// path, so the round trip stays self-consistent — none of these
			// libraries promises to accept another library's native shape.
			var hambaNative any
			if err := hamba.Unmarshal(hambaSchema, c.Payload, &hambaNative); err != nil {
				b.Fatal(err)
			}
			var iskoNative any
			if err := isko.Unmarshal(iskoSchema, c.Payload, &iskoNative); err != nil {
				b.Fatal(err)
			}
			var twmbNative any
			if _, err := twmbSchema.Decode(c.Payload, &twmbNative); err != nil {
				b.Fatal(err)
			}
			goavroNative, _, err := goavroCodec.NativeFromBinary(c.Payload)
			if err != nil {
				b.Fatal(err)
			}

			b.Run("binary", func(b *testing.B) {
				b.Run("typed", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := hamba.Marshal(hambaSchema, c.Value); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := isko.Marshal(iskoSchema, c.Value); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("twmb", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.Encode(c.Value); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.Skipf("goavro has no typed struct mode; see the dynamic arm")
					})
				})

				b.Run("dynamic", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := hamba.Marshal(hambaSchema, hambaNative); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := isko.Marshal(iskoSchema, iskoNative); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("twmb", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.Encode(twmbNative); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := goavroCodec.BinaryFromNative(nil, goavroNative); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			})

			b.Run("textual", func(b *testing.B) {
				b.Run("typed", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.Skipf("hamba/avro v2.31.0 has no Avro JSON (textual) encoding: Marshal/Unmarshal only produce binary")
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.Skipf("iskorotkov/avro is a hamba fork and carries the same gap: no Avro JSON (textual) encoding")
					})

					b.Run("twmb", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.EncodeJSON(c.Value, twmb.TaggedUnions()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.Skipf("goavro has no typed struct mode; see the dynamic arm")
					})
				})

				b.Run("dynamic", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						b.Skipf("hamba/avro v2.31.0 has no Avro JSON (textual) encoding: Marshal/Unmarshal only produce binary")
					})

					b.Run("iskorotkov", func(b *testing.B) {
						b.Skipf("iskorotkov/avro is a hamba fork and carries the same gap: no Avro JSON (textual) encoding")
					})

					b.Run("twmb", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.EncodeJSON(twmbNative, twmb.TaggedUnions()); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := goavroCodec.TextualFromNative(nil, goavroNative); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			})

			// hamba's soe.Codec.Encode appends onto the SOE header slice the
			// Codec built once in NewCodec, not a caller-supplied buffer: every
			// call starts from the same append(c.header, data...). Whether that
			// reallocates depends on the header's leftover capacity from its own
			// 2-byte-to-10-byte growth, a handful of bytes at most. Every payload
			// in this corpus is far larger than that, so it reallocates on every
			// call and the numbers below are not understated by it — confirmed
			// by probing separately with a bare "int" schema, where the sharing
			// DOES trigger and the previous call's returned slice is silently
			// overwritten by the next one. That is a correctness footgun for a
			// caller of a small schema who retains more than one encoded result
			// at a time, not just an allocation-count artifact. iskorotkov/avro's
			// soe package is the same code and shares the same behaviour.
			b.Run("single-object", func(b *testing.B) {
				b.Run("typed", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						codec, err := hambasoe.NewCodec(hambaSchema)
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
						codec, err := iskosoe.NewCodec(iskoSchema)
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
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.AppendSingleObject(nil, c.Value); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.Skipf("goavro has no typed struct mode; see the dynamic arm")
					})
				})

				b.Run("dynamic", func(b *testing.B) {
					b.Run("hamba", func(b *testing.B) {
						codec, err := hambasoe.NewCodec(hambaSchema)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := codec.Encode(hambaNative); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("iskorotkov", func(b *testing.B) {
						codec, err := iskosoe.NewCodec(iskoSchema)
						if err != nil {
							b.Fatal(err)
						}
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := codec.Encode(iskoNative); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("twmb", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := twmbSchema.AppendSingleObject(nil, twmbNative); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("goavro", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if _, err := goavroCodec.SingleFromNative(nil, goavroNative); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			})
		})
	}
}
