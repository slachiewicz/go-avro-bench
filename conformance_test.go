package avrobench

import (
	"fmt"
	"sort"
	"testing"

	hamba "github.com/hamba/avro/v2"
	isko "github.com/iskorotkov/avro/v2"
	goavro "github.com/linkedin/goavro/v2"
	twmb "github.com/twmb/avro"
)

func decodeDynamic(t *testing.T, lib, schemaJSON string, payload []byte) any {
	t.Helper()
	var out any
	var err error
	switch lib {
	case "hamba":
		err = hamba.Unmarshal(hamba.MustParse(schemaJSON), payload, &out)
	case "iskorotkov":
		err = isko.Unmarshal(isko.MustParse(schemaJSON), payload, &out)
	case "twmb":
		_, err = twmb.MustParse(schemaJSON).Decode(payload, &out)
	case "goavro":
		var codec *goavro.Codec
		codec, err = goavro.NewCodec(schemaJSON)
		if err == nil {
			out, _, err = codec.NativeFromBinary(payload)
		}
	}
	if err != nil {
		t.Fatalf("%s: %v", lib, err)
	}
	return out
}

var libs = []string{"hamba", "iskorotkov", "twmb", "goavro"}

// TestDynamicShapes records what each codec hands back for the same bytes.
// The values are not required to be identical — they are not — and the
// differences are the point: swapping codecs under a schema-driven pipeline
// changes the value users see, not just the time it takes to produce it.
func TestDynamicShapes(t *testing.T) {
	for _, c := range Cases {
		t.Run(c.Name, func(t *testing.T) {
			for _, lib := range libs {
				got := decodeDynamic(t, lib, c.SchemaJSON, c.Payload)
				m, ok := got.(map[string]any)
				if !ok {
					t.Logf("%-11s -> %T (not a map)", lib, got)
					continue
				}
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					t.Logf("%-11s %-14s %-22T %v", lib, k, m[k], summarise(m[k]))
				}
			}
		})
	}
}

func summarise(v any) string {
	switch t := v.(type) {
	case []byte:
		return fmt.Sprintf("%d bytes", len(t))
	case map[string]any:
		return fmt.Sprintf("map with %d keys", len(t))
	case []any:
		return fmt.Sprintf("slice of %d", len(t))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// TestRoundTripTyped asserts every struct-mode codec reproduces the fixture
// bytes from the fixture value, so the decode benchmarks are all reading a
// payload each of them agrees is valid.
func TestRoundTripTyped(t *testing.T) {
	for _, c := range Cases {
		t.Run(c.Name, func(t *testing.T) {
			enc := map[string]func() ([]byte, error){
				"hamba":      func() ([]byte, error) { return hamba.Marshal(hamba.MustParse(c.SchemaJSON), c.Value) },
				"iskorotkov": func() ([]byte, error) { return isko.Marshal(isko.MustParse(c.SchemaJSON), c.Value) },
				"twmb":       func() ([]byte, error) { return twmb.MustParse(c.SchemaJSON).Encode(c.Value) },
			}
			for lib, fn := range enc {
				b, err := fn()
				if err != nil {
					t.Errorf("%s encode: %v", lib, err)
					continue
				}
				if string(b) != string(c.Payload) {
					t.Errorf("%s: encoding differs from fixture (%d vs %d bytes)", lib, len(b), len(c.Payload))
				}
			}
		})
	}
}
