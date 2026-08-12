package avrobench

import (
	"bytes"
	"compress/flate"
	"errors"
	"io"
	"testing"

	hamba "github.com/hamba/avro/v2"
	hambaocf "github.com/hamba/avro/v2/ocf"
	isko "github.com/iskorotkov/avro/v2"
	iskoocf "github.com/iskorotkov/avro/v2/ocf"
	goavro "github.com/linkedin/goavro/v2"
	twmb "github.com/twmb/avro"
	twmbocf "github.com/twmb/avro/ocf"
)

// ocfRecords is how many records go into the container BenchmarkOCFWrite
// builds and BenchmarkOCFRead consumes. Object Container Files are a batch
// format — nothing opens one to hold a single record — so b.N here times
// whole-container operations; ns/op and allocs/op are the cost of N records,
// not of one, and b.SetBytes reports the container size so throughput is
// comparable across codecs despite that.
const ocfRecords = 1000

// ocfCodecs are the compression codecs all four libraries happen to support
// in common. Unlike the encodings in encodings_test.go, none of these need a
// Skip anywhere below — that uniformity is itself a finding, not an omission.
var ocfCodecs = []string{"null", "deflate", "snappy"}

// buildHambaOCF encodes n copies of v into an in-memory OCF container. v is a
// typed struct for the "typed" arms and, for "dynamic", an any holding the
// shape hamba's own dynamic decode produces — the Encoder accepts both the
// same way Marshal does.
func buildHambaOCF(schema hamba.Schema, v any, codecName string, n int) ([]byte, error) {
	var buf bytes.Buffer
	enc, err := hambaocf.NewEncoderWithSchema(schema, &buf, hambaocf.WithCodec(hambaocf.CodecName(codecName)))
	if err != nil {
		return nil, err
	}
	for i := 0; i < n; i++ {
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildIskoOCF is buildHambaOCF's iskorotkov/avro equivalent; the two forks
// share the same ocf package shape.
func buildIskoOCF(schema isko.Schema, v any, codecName string, n int) ([]byte, error) {
	var buf bytes.Buffer
	enc, err := iskoocf.NewEncoderWithSchema(schema, &buf, iskoocf.WithCodec(iskoocf.CodecName(codecName)))
	if err != nil {
		return nil, err
	}
	for i := 0; i < n; i++ {
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildTwmbOCF encodes n copies of v into an in-memory OCF container. null is
// twmb/avro's default codec, so it needs no WithCodec option; deflate is
// given flate.DefaultCompression to match the level hamba, iskorotkov and
// goavro all default to below.
func buildTwmbOCF(schema *twmb.Schema, v any, codecName string, n int) ([]byte, error) {
	var opts []twmbocf.WriterOpt
	switch codecName {
	case "deflate":
		opts = append(opts, twmbocf.WithCodec(twmbocf.DeflateCodec(flate.DefaultCompression)))
	case "snappy":
		opts = append(opts, twmbocf.WithCodec(twmbocf.SnappyCodec()))
	}
	var buf bytes.Buffer
	w, err := twmbocf.NewWriter(&buf, schema, opts...)
	if err != nil {
		return nil, err
	}
	for i := 0; i < n; i++ {
		if err := w.Encode(v); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildGoavroOCF encodes n copies of native into an in-memory OCF container.
// goavro has no typed struct mode, so native is the native value the
// fixture's own binary payload decodes to, not the typed Case.Value.
// OCFWriter.Append takes the whole slice in one call and chunks it into
// blocks itself, unlike the per-record Encode loop the other three use.
func buildGoavroOCF(schemaJSON string, native any, codecName string, n int) ([]byte, error) {
	var buf bytes.Buffer
	w, err := goavro.NewOCFWriter(goavro.OCFConfig{
		W:               &buf,
		Schema:          schemaJSON,
		CompressionName: codecName,
	})
	if err != nil {
		return nil, err
	}
	items := make([]interface{}, n)
	for i := range items {
		items[i] = native
	}
	if err := w.Append(items); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BenchmarkOCFWrite writes ocfRecords copies of a Case into an in-memory OCF
// container, per library and compression codec, under both "typed" (encode
// from a generated Go struct) and "dynamic" (encode from a native value).
// goavro only ever has the dynamic path, so its typed arm Skips rather than
// leaving goavro implicitly compared against the other three's typed numbers
// — see BenchmarkEncodingDecode's doc comment for why that distinction
// matters in this repo specifically.
func BenchmarkOCFWrite(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			hambaSchema := hamba.MustParse(c.SchemaJSON)
			iskoSchema := isko.MustParse(c.SchemaJSON)
			twmbSchema := twmb.MustParse(c.SchemaJSON)

			goavroCodec, err := goavro.NewCodec(c.SchemaJSON)
			if err != nil {
				b.Fatal(err)
			}
			goavroNative, _, err := goavroCodec.NativeFromBinary(c.Payload)
			if err != nil {
				b.Fatal(err)
			}

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

			for _, codecName := range ocfCodecs {
				b.Run(codecName, func(b *testing.B) {
					b.Run("typed", func(b *testing.B) {
						b.Run("hamba", func(b *testing.B) {
							container, err := buildHambaOCF(hambaSchema, c.Value, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildHambaOCF(hambaSchema, c.Value, codecName, ocfRecords); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("iskorotkov", func(b *testing.B) {
							container, err := buildIskoOCF(iskoSchema, c.Value, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildIskoOCF(iskoSchema, c.Value, codecName, ocfRecords); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("twmb", func(b *testing.B) {
							container, err := buildTwmbOCF(twmbSchema, c.Value, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildTwmbOCF(twmbSchema, c.Value, codecName, ocfRecords); err != nil {
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
							container, err := buildHambaOCF(hambaSchema, hambaNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildHambaOCF(hambaSchema, hambaNative, codecName, ocfRecords); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("iskorotkov", func(b *testing.B) {
							container, err := buildIskoOCF(iskoSchema, iskoNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildIskoOCF(iskoSchema, iskoNative, codecName, ocfRecords); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("twmb", func(b *testing.B) {
							container, err := buildTwmbOCF(twmbSchema, twmbNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildTwmbOCF(twmbSchema, twmbNative, codecName, ocfRecords); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("goavro", func(b *testing.B) {
							container, err := buildGoavroOCF(c.SchemaJSON, goavroNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								if _, err := buildGoavroOCF(c.SchemaJSON, goavroNative, codecName, ocfRecords); err != nil {
									b.Fatal(err)
								}
							}
						})
					})
				})
			}
		})
	}
}

// BenchmarkOCFRead reads all ocfRecords records back out of a container built
// once per library, codec and mode, outside the timed loop. "typed" decodes
// into a generated Go struct; "dynamic" decodes into map[string]any. goavro
// only ever has the dynamic path, so its typed arm Skips — see
// BenchmarkEncodingDecode's doc comment for why that distinction matters here.
func BenchmarkOCFRead(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			hambaSchema := hamba.MustParse(c.SchemaJSON)
			iskoSchema := isko.MustParse(c.SchemaJSON)
			twmbSchema := twmb.MustParse(c.SchemaJSON)

			goavroCodec, err := goavro.NewCodec(c.SchemaJSON)
			if err != nil {
				b.Fatal(err)
			}
			goavroNative, _, err := goavroCodec.NativeFromBinary(c.Payload)
			if err != nil {
				b.Fatal(err)
			}

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

			for _, codecName := range ocfCodecs {
				b.Run(codecName, func(b *testing.B) {
					b.Run("typed", func(b *testing.B) {
						b.Run("hamba", func(b *testing.B) {
							container, err := buildHambaOCF(hambaSchema, c.Value, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								dec, err := hambaocf.NewDecoder(bytes.NewReader(container))
								if err != nil {
									b.Fatal(err)
								}
								for dec.HasNext() {
									if err := dec.Decode(c.NewTyped()); err != nil {
										b.Fatal(err)
									}
								}
								if err := dec.Error(); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("iskorotkov", func(b *testing.B) {
							container, err := buildIskoOCF(iskoSchema, c.Value, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								dec, err := iskoocf.NewDecoder(bytes.NewReader(container))
								if err != nil {
									b.Fatal(err)
								}
								for dec.HasNext() {
									if err := dec.Decode(c.NewTyped()); err != nil {
										b.Fatal(err)
									}
								}
								if err := dec.Error(); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("twmb", func(b *testing.B) {
							container, err := buildTwmbOCF(twmbSchema, c.Value, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								r, err := twmbocf.NewReader(bytes.NewReader(container))
								if err != nil {
									b.Fatal(err)
								}
								for {
									if err := r.Decode(c.NewTyped()); err != nil {
										if errors.Is(err, io.EOF) {
											break
										}
										b.Fatal(err)
									}
								}
							}
						})

						b.Run("goavro", func(b *testing.B) {
							b.Skipf("goavro has no typed struct mode; see the dynamic arm")
						})
					})

					b.Run("dynamic", func(b *testing.B) {
						b.Run("hamba", func(b *testing.B) {
							container, err := buildHambaOCF(hambaSchema, hambaNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								dec, err := hambaocf.NewDecoder(bytes.NewReader(container))
								if err != nil {
									b.Fatal(err)
								}
								for dec.HasNext() {
									var out any
									if err := dec.Decode(&out); err != nil {
										b.Fatal(err)
									}
								}
								if err := dec.Error(); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("iskorotkov", func(b *testing.B) {
							container, err := buildIskoOCF(iskoSchema, iskoNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								dec, err := iskoocf.NewDecoder(bytes.NewReader(container))
								if err != nil {
									b.Fatal(err)
								}
								for dec.HasNext() {
									var out any
									if err := dec.Decode(&out); err != nil {
										b.Fatal(err)
									}
								}
								if err := dec.Error(); err != nil {
									b.Fatal(err)
								}
							}
						})

						b.Run("twmb", func(b *testing.B) {
							container, err := buildTwmbOCF(twmbSchema, twmbNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								r, err := twmbocf.NewReader(bytes.NewReader(container))
								if err != nil {
									b.Fatal(err)
								}
								for {
									var out any
									if err := r.Decode(&out); err != nil {
										if errors.Is(err, io.EOF) {
											break
										}
										b.Fatal(err)
									}
								}
							}
						})

						b.Run("goavro", func(b *testing.B) {
							container, err := buildGoavroOCF(c.SchemaJSON, goavroNative, codecName, ocfRecords)
							if err != nil {
								b.Fatal(err)
							}
							b.SetBytes(int64(len(container)))
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								r, err := goavro.NewOCFReader(bytes.NewReader(container))
								if err != nil {
									b.Fatal(err)
								}
								for r.Scan() {
									if _, err := r.Read(); err != nil {
										b.Fatal(err)
									}
								}
								if err := r.Err(); err != nil {
									b.Fatal(err)
								}
							}
						})
					})
				})
			}
		})
	}
}
