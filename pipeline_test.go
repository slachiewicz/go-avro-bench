package avrobench

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	hamba "github.com/hamba/avro/v2"
	isko "github.com/iskorotkov/avro/v2"
	goavro "github.com/linkedin/goavro/v2"
	twmb "github.com/twmb/avro"
)

// The benchmarks in this file model bento's internal/impl/confluent/serde_avro.go
// hot path end to end, not the single-stage decode BenchmarkDecodeDynamic
// measures. Decode there is native, _, err := codec.NativeFromBinary(b) followed
// by jb, err := codec.TextualFromNative(nil, native) — binary in, JSON bytes
// out — and encode is the mirror: NativeFromTextual then BinaryFromNative.
// A pipeline also picks its goavro codec from config, between NewCodec (unions
// render as the wrapped {"string":"x"} form) and NewCodecForStandardJSONFull
// (bare values); both appear here as separate arms because the union finding
// in RESULTS.md was measured in only one of the two.

// schemaRegistryID is the one schema ID every fixture is framed under. A real
// registry client keys a codec cache by ID; a single-entry map still pays for
// the lookup, which is the cost this harness wants inside the timer.
const schemaRegistryID = 42

// stripConfluentHeader removes the magic byte and big-endian schema ID that
// schema_registry_decode strips before every decode, returning the ID so the
// codec-cache lookup is part of the measured work rather than assumed away.
func stripConfluentHeader(b []byte) (uint32, []byte) {
	return binary.BigEndian.Uint32(b[1:5]), b[5:]
}

// BenchmarkSRDecode is the full decode path: Confluent-framed binary in, JSON
// bytes out. goavro does this with one codec built either way; the other
// libraries have no schema-aware JSON encoder, so they decode into `any` and
// fall back to encoding/json — the comparison this benchmark exists to show.
func BenchmarkSRDecode(b *testing.B) {
	for _, c := range Cases {
		framed := ConfluentFrame(schemaRegistryID, c.Payload)

		b.Run(c.Name, func(b *testing.B) {
			b.Run("goavro-plain", func(b *testing.B) {
				codec, err := goavro.NewCodec(c.SchemaJSON)
				if err != nil {
					b.Fatal(err)
				}
				codecs := map[uint32]*goavro.Codec{schemaRegistryID: codec}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					id, payload := stripConfluentHeader(framed)
					native, _, err := codecs[id].NativeFromBinary(payload)
					if err != nil {
						b.Fatal(err)
					}
					if _, err := codecs[id].TextualFromNative(nil, native); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("goavro-stdjson", func(b *testing.B) {
				codec, err := goavro.NewCodecForStandardJSONFull(c.SchemaJSON)
				if err != nil {
					b.Skipf("NewCodecForStandardJSONFull: %v", err)
				}
				codecs := map[uint32]*goavro.Codec{schemaRegistryID: codec}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					id, payload := stripConfluentHeader(framed)
					native, _, err := codecs[id].NativeFromBinary(payload)
					if err != nil {
						b.Fatal(err)
					}
					if _, err := codecs[id].TextualFromNative(nil, native); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("hamba", func(b *testing.B) {
				schemas := map[uint32]hamba.Schema{schemaRegistryID: hamba.MustParse(c.SchemaJSON)}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					id, payload := stripConfluentHeader(framed)
					var out any
					if err := hamba.Unmarshal(schemas[id], payload, &out); err != nil {
						b.Fatal(err)
					}
					if _, err := json.Marshal(out); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("iskorotkov", func(b *testing.B) {
				schemas := map[uint32]isko.Schema{schemaRegistryID: isko.MustParse(c.SchemaJSON)}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					id, payload := stripConfluentHeader(framed)
					var out any
					if err := isko.Unmarshal(schemas[id], payload, &out); err != nil {
						b.Fatal(err)
					}
					if _, err := json.Marshal(out); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("twmb", func(b *testing.B) {
				schemas := map[uint32]*twmb.Schema{schemaRegistryID: twmb.MustParse(c.SchemaJSON)}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					id, payload := stripConfluentHeader(framed)
					var out any
					if _, err := schemas[id].Decode(payload, &out); err != nil {
						b.Fatal(err)
					}
					if _, err := json.Marshal(out); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkSREncode is the mirror of BenchmarkSRDecode: JSON bytes in,
// Confluent-framed binary out. The two goavro arms need JSON shaped for their
// own codec (wrapped unions for NewCodec, bare values for
// NewCodecForStandardJSONFull), so each builds its own input once, outside the
// timer, from the same fixture payload. hamba, iskorotkov and twmb all receive
// the bare-value form, since none of them understand goavro's union envelope.
func BenchmarkSREncode(b *testing.B) {
	for _, c := range Cases {
		plainCodec, err := goavro.NewCodecForStandardJSONFull(c.SchemaJSON)
		var plainJSON []byte
		if err == nil {
			var native any
			native, _, err = plainCodec.NativeFromBinary(c.Payload)
			if err == nil {
				plainJSON, err = plainCodec.TextualFromNative(nil, native)
			}
		}
		if err != nil {
			b.Fatalf("%s: building plain JSON input: %v", c.Name, err)
		}

		wrappedCodec, err := goavro.NewCodec(c.SchemaJSON)
		if err != nil {
			b.Fatal(err)
		}
		wrappedNative, _, err := wrappedCodec.NativeFromBinary(c.Payload)
		if err != nil {
			b.Fatal(err)
		}
		wrappedJSON, err := wrappedCodec.TextualFromNative(nil, wrappedNative)
		if err != nil {
			b.Fatal(err)
		}

		b.Run(c.Name, func(b *testing.B) {
			b.Run("goavro-plain", func(b *testing.B) {
				codecs := map[uint32]*goavro.Codec{schemaRegistryID: wrappedCodec}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					codec := codecs[schemaRegistryID]
					datum, _, err := codec.NativeFromTextual(wrappedJSON)
					if err != nil {
						b.Fatal(err)
					}
					bin, err := codec.BinaryFromNative(nil, datum)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, bin)
				}
			})

			b.Run("goavro-stdjson", func(b *testing.B) {
				codecs := map[uint32]*goavro.Codec{schemaRegistryID: plainCodec}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					codec := codecs[schemaRegistryID]
					datum, _, err := codec.NativeFromTextual(plainJSON)
					if err != nil {
						b.Fatal(err)
					}
					bin, err := codec.BinaryFromNative(nil, datum)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, bin)
				}
			})

			b.Run("hamba", func(b *testing.B) {
				schemas := map[uint32]hamba.Schema{schemaRegistryID: hamba.MustParse(c.SchemaJSON)}
				var probe any
				if err := json.Unmarshal(plainJSON, &probe); err != nil {
					b.Fatal(err)
				}
				if _, err := hamba.Marshal(schemas[schemaRegistryID], probe); err != nil {
					b.Skipf("hamba.Marshal on json.Unmarshal(any) input: %v", err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var v any
					if err := json.Unmarshal(plainJSON, &v); err != nil {
						b.Fatal(err)
					}
					datum, err := hamba.Marshal(schemas[schemaRegistryID], v)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, datum)
				}
			})

			b.Run("iskorotkov", func(b *testing.B) {
				schemas := map[uint32]isko.Schema{schemaRegistryID: isko.MustParse(c.SchemaJSON)}
				var probe any
				if err := json.Unmarshal(plainJSON, &probe); err != nil {
					b.Fatal(err)
				}
				if _, err := isko.Marshal(schemas[schemaRegistryID], probe); err != nil {
					b.Skipf("isko.Marshal on json.Unmarshal(any) input: %v", err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var v any
					if err := json.Unmarshal(plainJSON, &v); err != nil {
						b.Fatal(err)
					}
					datum, err := isko.Marshal(schemas[schemaRegistryID], v)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, datum)
				}
			})

			b.Run("twmb", func(b *testing.B) {
				schemas := map[uint32]*twmb.Schema{schemaRegistryID: twmb.MustParse(c.SchemaJSON)}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var v any
					if err := json.Unmarshal(plainJSON, &v); err != nil {
						b.Fatal(err)
					}
					datum, err := schemas[schemaRegistryID].Encode(v)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, datum)
				}
			})
		})
	}
}

// coerceHamba adapts v — the `any` tree json.Unmarshal produces — to the Go
// types hamba's Marshal requires: every JSON number arrives as float64
// regardless of the schema's declared width, and there is no bare-scalar
// convention for a union or enum value. This is a minimal stand-in for the
// shim redpanda-data/connect's normalize_for_avro_schema.go used to provide;
// it does not attempt logical types, and it trusts the schema rather than
// re-deriving it, which a production normaliser handling arbitrary input
// could not do.
func coerceHamba(s hamba.Schema, v any) (any, error) {
	switch t := s.(type) {
	case *hamba.RecordSchema:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("record %s: got %T", t.FullName(), v)
		}
		out := make(map[string]any, len(m))
		for _, f := range t.Fields() {
			cv, err := coerceHamba(f.Type(), m[f.Name()])
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", t.FullName(), f.Name(), err)
			}
			out[f.Name()] = cv
		}
		return out, nil
	case *hamba.UnionSchema:
		if v == nil {
			return nil, nil
		}
		for _, branch := range t.Types() {
			if branch.Type() == hamba.Null {
				continue
			}
			if cv, err := coerceHamba(branch, v); err == nil {
				return cv, nil
			}
		}
		return nil, fmt.Errorf("union %v: no branch accepts %T", t.Types(), v)
	case *hamba.ArraySchema:
		if v == nil {
			return nil, nil
		}
		a, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("array: got %T", v)
		}
		out := make([]any, len(a))
		for i, e := range a {
			cv, err := coerceHamba(t.Items(), e)
			if err != nil {
				return nil, err
			}
			out[i] = cv
		}
		return out, nil
	case *hamba.MapSchema:
		if v == nil {
			return nil, nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("map: got %T", v)
		}
		out := make(map[string]any, len(m))
		for k, e := range m {
			cv, err := coerceHamba(t.Values(), e)
			if err != nil {
				return nil, err
			}
			out[k] = cv
		}
		return out, nil
	case *hamba.EnumSchema:
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("enum %s: got %T", t.FullName(), v)
		}
		return str, nil
	case *hamba.PrimitiveSchema:
		switch t.Type() {
		case hamba.Int:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("int: got %T", v)
			}
			return int32(f), nil
		case hamba.Long:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("long: got %T", v)
			}
			return int64(f), nil
		case hamba.Float:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("float: got %T", v)
			}
			return float32(f), nil
		case hamba.Double:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("double: got %T", v)
			}
			return f, nil
		case hamba.Bytes:
			// The fixture's JSON input comes from goavro's TextualFromNative,
			// which renders bytes as a raw (non-base64) string. A producer that
			// base64-encodes bytes through generic JSON would need a different
			// rule here — one more reason this is a probe, not a normaliser.
			str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("bytes: got %T", v)
			}
			return []byte(str), nil
		default:
			return v, nil
		}
	default:
		return v, nil
	}
}

// coerceIsko is coerceHamba against iskorotkov/avro/v2's schema types. hamba
// and its fork expose identical shapes but are separate named types in
// separate packages, so nothing here can be shared without an adapter layer —
// itself part of the cost a hamba/iskorotkov migration would carry.
func coerceIsko(s isko.Schema, v any) (any, error) {
	switch t := s.(type) {
	case *isko.RecordSchema:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("record %s: got %T", t.FullName(), v)
		}
		out := make(map[string]any, len(m))
		for _, f := range t.Fields() {
			cv, err := coerceIsko(f.Type(), m[f.Name()])
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", t.FullName(), f.Name(), err)
			}
			out[f.Name()] = cv
		}
		return out, nil
	case *isko.UnionSchema:
		if v == nil {
			return nil, nil
		}
		for _, branch := range t.Types() {
			if branch.Type() == isko.Null {
				continue
			}
			if cv, err := coerceIsko(branch, v); err == nil {
				return cv, nil
			}
		}
		return nil, fmt.Errorf("union %v: no branch accepts %T", t.Types(), v)
	case *isko.ArraySchema:
		if v == nil {
			return nil, nil
		}
		a, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("array: got %T", v)
		}
		out := make([]any, len(a))
		for i, e := range a {
			cv, err := coerceIsko(t.Items(), e)
			if err != nil {
				return nil, err
			}
			out[i] = cv
		}
		return out, nil
	case *isko.MapSchema:
		if v == nil {
			return nil, nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("map: got %T", v)
		}
		out := make(map[string]any, len(m))
		for k, e := range m {
			cv, err := coerceIsko(t.Values(), e)
			if err != nil {
				return nil, err
			}
			out[k] = cv
		}
		return out, nil
	case *isko.EnumSchema:
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("enum %s: got %T", t.FullName(), v)
		}
		return str, nil
	case *isko.PrimitiveSchema:
		switch t.Type() {
		case isko.Int:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("int: got %T", v)
			}
			return int32(f), nil
		case isko.Long:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("long: got %T", v)
			}
			return int64(f), nil
		case isko.Float:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("float: got %T", v)
			}
			return float32(f), nil
		case isko.Double:
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("double: got %T", v)
			}
			return f, nil
		case isko.Bytes:
			str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("bytes: got %T", v)
			}
			return []byte(str), nil
		default:
			return v, nil
		}
	default:
		return v, nil
	}
}

// BenchmarkSREncodeCoerced is BenchmarkSREncode's hamba/iskorotkov arms with
// coerceHamba/coerceIsko run inside the timed loop instead of skipping, plus
// twmb unchanged as a control: twmb needs no shim, so its arm shows what the
// shim costs relative to a codec that was never going to pay it. Comparing
// this against BenchmarkSREncode's twmb numbers quantifies the shim itself;
// comparing hamba/iskorotkov here against twmb here answers the migration
// question directly.
func BenchmarkSREncodeCoerced(b *testing.B) {
	for _, c := range Cases {
		plainCodec, err := goavro.NewCodecForStandardJSONFull(c.SchemaJSON)
		if err != nil {
			b.Fatalf("%s: NewCodecForStandardJSONFull: %v", c.Name, err)
		}
		native, _, err := plainCodec.NativeFromBinary(c.Payload)
		if err != nil {
			b.Fatal(err)
		}
		plainJSON, err := plainCodec.TextualFromNative(nil, native)
		if err != nil {
			b.Fatal(err)
		}

		b.Run(c.Name, func(b *testing.B) {
			b.Run("hamba", func(b *testing.B) {
				schema := hamba.MustParse(c.SchemaJSON)
				schemas := map[uint32]hamba.Schema{schemaRegistryID: schema}
				var probe any
				if err := json.Unmarshal(plainJSON, &probe); err != nil {
					b.Fatal(err)
				}
				coerced, err := coerceHamba(schemas[schemaRegistryID], probe)
				if err != nil {
					b.Skipf("coerceHamba could not rescue %s: %v", c.Name, err)
				}
				if _, err := hamba.Marshal(schemas[schemaRegistryID], coerced); err != nil {
					b.Skipf("hamba.Marshal after coercion: %v", err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var v any
					if err := json.Unmarshal(plainJSON, &v); err != nil {
						b.Fatal(err)
					}
					cv, err := coerceHamba(schemas[schemaRegistryID], v)
					if err != nil {
						b.Fatal(err)
					}
					datum, err := hamba.Marshal(schemas[schemaRegistryID], cv)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, datum)
				}
			})

			b.Run("iskorotkov", func(b *testing.B) {
				schema := isko.MustParse(c.SchemaJSON)
				schemas := map[uint32]isko.Schema{schemaRegistryID: schema}
				var probe any
				if err := json.Unmarshal(plainJSON, &probe); err != nil {
					b.Fatal(err)
				}
				coerced, err := coerceIsko(schemas[schemaRegistryID], probe)
				if err != nil {
					b.Skipf("coerceIsko could not rescue %s: %v", c.Name, err)
				}
				if _, err := isko.Marshal(schemas[schemaRegistryID], coerced); err != nil {
					b.Skipf("isko.Marshal after coercion: %v", err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var v any
					if err := json.Unmarshal(plainJSON, &v); err != nil {
						b.Fatal(err)
					}
					cv, err := coerceIsko(schemas[schemaRegistryID], v)
					if err != nil {
						b.Fatal(err)
					}
					datum, err := isko.Marshal(schemas[schemaRegistryID], cv)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, datum)
				}
			})

			b.Run("twmb", func(b *testing.B) {
				schemas := map[uint32]*twmb.Schema{schemaRegistryID: twmb.MustParse(c.SchemaJSON)}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var v any
					if err := json.Unmarshal(plainJSON, &v); err != nil {
						b.Fatal(err)
					}
					datum, err := schemas[schemaRegistryID].Encode(v)
					if err != nil {
						b.Fatal(err)
					}
					_ = ConfluentFrame(schemaRegistryID, datum)
				}
			})
		})
	}
}

// BenchmarkDecodeStage splits goavro's two decode calls apart so the split
// between them is visible instead of folded into one BenchmarkSRDecode number,
// for both codec modes.
func BenchmarkDecodeStage(b *testing.B) {
	for _, c := range Cases {
		b.Run(c.Name, func(b *testing.B) {
			b.Run("goavro-plain-native", func(b *testing.B) {
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

			b.Run("goavro-plain-textual", func(b *testing.B) {
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

			b.Run("goavro-stdjson-native", func(b *testing.B) {
				codec, err := goavro.NewCodecForStandardJSONFull(c.SchemaJSON)
				if err != nil {
					b.Skipf("NewCodecForStandardJSONFull: %v", err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, _, err := codec.NativeFromBinary(c.Payload); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("goavro-stdjson-textual", func(b *testing.B) {
				codec, err := goavro.NewCodecForStandardJSONFull(c.SchemaJSON)
				if err != nil {
					b.Skipf("NewCodecForStandardJSONFull: %v", err)
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
	}
}
