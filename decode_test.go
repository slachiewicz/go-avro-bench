package avrobench

import (
	"testing"

	hamba "github.com/hamba/avro/v2"
	isko "github.com/iskorotkov/avro/v2"
	goavro "github.com/linkedin/goavro/v2"
	twmb "github.com/twmb/avro"
)

// BenchmarkDecodeDynamic decodes into an untyped destination — map[string]any or
// the codec's native equivalent. This is the shape a schema-driven pipeline
// needs, because the schema is not known when the binary is built.
//
// Every iteration allocates its own result. Holding one destination across b.N
// measures a case no pipeline has.
func BenchmarkDecodeDynamic(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			b.Run("hamba", func(b *testing.B) {
				s := hamba.MustParse(c.SchemaJSON)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out any
					if err := hamba.Unmarshal(s, c.Payload, &out); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("iskorotkov", func(b *testing.B) {
				s := isko.MustParse(c.SchemaJSON)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out any
					if err := isko.Unmarshal(s, c.Payload, &out); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("twmb", func(b *testing.B) {
				s := twmb.MustParse(c.SchemaJSON)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out any
					if _, err := s.Decode(c.Payload, &out); err != nil {
						b.Fatal(err)
					}
				}
			})

			// twmb-alias is the same decode with AliasInput, which twmb added
			// on 2026-08-18: strings and byte slices point into the payload
			// instead of being copied. Only a consumer that owns each
			// message's buffer for as long as it holds the value can use it,
			// so it is a separate arm, not a replacement for the one above.
			b.Run("twmb-alias", func(b *testing.B) {
				s := twmb.MustParse(c.SchemaJSON)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var out any
					if _, err := s.Decode(c.Payload, &out, twmb.AliasInput()); err != nil {
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
	}
}

// BenchmarkDecodeTyped decodes into a generated Go struct. goavro is absent
// because it has no struct mode; that is the trade, not a gap in the harness.
func BenchmarkDecodeTyped(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
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

			// See twmb-alias under BenchmarkDecodeDynamic.
			b.Run("twmb-alias", func(b *testing.B) {
				s := twmb.MustParse(c.SchemaJSON)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := s.Decode(c.Payload, c.NewTyped(), twmb.AliasInput()); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkDecodeTypedReused is the shape nrwiersma/avro-benchmarks measures:
// one destination struct, allocated before the timer, refilled every iteration.
// Kept so the gap against BenchmarkDecodeTyped is visible and labelled instead
// of silently inflating a comparison.
func BenchmarkDecodeTypedReused(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			b.Run("hamba", func(b *testing.B) {
				s := hamba.MustParse(c.SchemaJSON)
				dst := c.NewTyped()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := hamba.Unmarshal(s, c.Payload, dst); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("twmb", func(b *testing.B) {
				s := twmb.MustParse(c.SchemaJSON)
				dst := c.NewTyped()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := s.Decode(c.Payload, dst); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkParse measures schema compilation. Pipelines that cache a codec per
// schema pay this once; ones that compile per message pay it per message.
func BenchmarkParse(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			b.Run("hamba", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := hamba.Parse(c.SchemaJSON); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("twmb", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := twmb.Parse(c.SchemaJSON); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("goavro", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := goavro.NewCodec(c.SchemaJSON); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
