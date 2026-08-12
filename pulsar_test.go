package avrobench

import (
	"testing"

	hamba "github.com/hamba/avro/v2"
	isko "github.com/iskorotkov/avro/v2"
	twmb "github.com/twmb/avro"
)

// findCase returns the named Case or fails the benchmark; a small helper so
// the cache-effect benchmark below can pick one representative schema
// without looping the whole corpus.
func findCase(b *testing.B, name string) Case {
	b.Helper()
	for _, c := range Cases {
		if c.Name == name {
			return c
		}
	}
	b.Fatalf("case %q not found", name)
	return Case{}
}

// BenchmarkPulsarPattern models apache/pulsar-client-go's pulsar/schema.go:
// avro.Parse(schemaDef) once, when the AvroSchema is constructed, then
// avro.Unmarshal(codec, data, v) per message into whatever type the caller
// passed to Schema.Decode — the codec never sees or chooses that type. This
// is a cached-codec, typed access pattern, and it is what every consumer of
// pulsar's Go client actually pays per message today, whichever library
// backs it.
//
// goavro is absent: it has no struct/typed mode, only NativeFromBinary into
// map[string]any, so it cannot fill a caller-supplied v the way
// AvroSchema.Decode's signature requires. Pulsar could not use it as-is.
func BenchmarkPulsarPattern(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			b.Run("hamba", func(b *testing.B) {
				schema := hamba.MustParse(c.SchemaJSON) // initAvroCodec, once
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dst := c.NewTyped() // AvroSchema.Decode(data, v): fresh v per message
					if err := hamba.Unmarshal(schema, c.Payload, dst); err != nil {
						b.Fatal(err)
					}
				}
			})

			// iskorotkov: the CVE-patched fork proposed in
			// apache/pulsar-client-go#1518.
			b.Run("iskorotkov", func(b *testing.B) {
				schema := isko.MustParse(c.SchemaJSON)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dst := c.NewTyped()
					if err := isko.Unmarshal(schema, c.Payload, dst); err != nil {
						b.Fatal(err)
					}
				}
			})

			// twmb: the alternative raised in
			// apache/pulsar-client-go#1526.
			b.Run("twmb", func(b *testing.B) {
				schema := twmb.MustParse(c.SchemaJSON)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dst := c.NewTyped()
					if _, err := schema.Decode(c.Payload, dst); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkCodecCacheEffect contrasts a codec parsed once against one
// re-parsed on every message, both decoding the same payload. BenchmarkParse
// (decode_test.go) already shows parsing costs 20-70x a single decode; this
// turns that into a single decode-loop number instead of two figures a
// reader has to multiply by hand. hamba stands in for "one library" here
// because it is the one pulsar-client-go depends on today, so this is the
// cost pulsar would pay if AvroSchema's constructor-time Parse were ever
// replaced with a per-message one.
func BenchmarkCodecCacheEffect(b *testing.B) {
	c := findCase(b, "nested")

	b.Run("cached", func(b *testing.B) {
		b.Run("hamba", func(b *testing.B) {
			schema := hamba.MustParse(c.SchemaJSON) // parsed once, outside the loop
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dst := c.NewTyped()
				if err := hamba.Unmarshal(schema, c.Payload, dst); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("reparsed", func(b *testing.B) {
		b.Run("hamba", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				schema := hamba.MustParse(c.SchemaJSON) // parsed every message
				dst := c.NewTyped()
				if err := hamba.Unmarshal(schema, c.Payload, dst); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}
