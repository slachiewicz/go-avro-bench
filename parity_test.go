package avrobench

import (
	_ "embed"
	"fmt"
	"math/big"
	"testing"
	"time"

	hamba "github.com/hamba/avro/v2"
	hambasoe "github.com/hamba/avro/v2/soe"
	isko "github.com/iskorotkov/avro/v2"
	iskosoe "github.com/iskorotkov/avro/v2/soe"
	goavro "github.com/linkedin/goavro/v2"
	twmb "github.com/twmb/avro"
)

//go:embed schemas/logical.avsc
var logicalSchemaJSON string

//go:embed schemas/evo_writer.avsc
var evoWriterSchemaJSON string

//go:embed schemas/evo_reader.avsc
var evoReaderSchemaJSON string

//go:embed schemas/refs.avsc
var refsSchemaJSON string

// TestFeatureParity is not a pass/fail comparison. It is a decision table:
// for each representational choice a codec makes, record the concrete Go
// type and value every library hands back for identical bytes, so a codec
// swap can be evaluated on what changes for callers, not on ns/op. A probe
// only fails the test when a library cannot produce a decodable result at
// all (an error or panic on input the Avro spec says is valid) — a
// different but valid representation is the finding, not a failure.
func TestFeatureParity(t *testing.T) {
	t.Run("logical_types", testLogicalTypes)
	t.Run("nullable_union", testNullableUnion)
	t.Run("schema_evolution", testSchemaEvolution)
	t.Run("schema_references", testSchemaReferences)
	t.Run("default_missing_field", testDefaultMissingField)
	t.Run("single_object_encode_ownership", testSingleObjectEncodeOwnership)
}

// probeDynamic decodes payload into `any` and reports the error rather than
// failing the test, so one library's gap does not stop the rest of the row
// from being recorded. A panic — as opposed to a returned error — is always
// a genuine bug in the codec's handling of spec-valid input, so it is
// converted to an error here and the caller decides whether that is
// expected (a documented gap) or not.
func probeDynamic(lib, schemaJSON string, payload []byte) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
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
	default:
		err = fmt.Errorf("unknown lib %q", lib)
	}
	return out, err
}

// logField prints one (library, field, type, value) row of the matrix.
// summarise is conformance_test.go's formatter for the awkward cases
// ([]byte, nested maps, slices); everything else falls through to %v.
func logField(t *testing.T, lib, key string, v any) {
	t.Helper()
	t.Logf("%-11s %-14s %-28T %s", lib, key, v, summarise(v))
}

// logicalValue is the struct hamba encodes the fixture payload from. Its
// field types are the ones hamba's own codec requires for each logical
// type; every other library reads the same bytes and makes its own choice
// about what Go value to hand back — that choice is what this probe records.
type logicalValue struct {
	TSMillis    time.Time     `avro:"ts_millis"`
	TSMicros    time.Time     `avro:"ts_micros"`
	Day         time.Time     `avro:"day"`
	TODMillis   time.Duration `avro:"tod_millis"`
	ID          string        `avro:"id"`
	Amount      *big.Rat      `avro:"amount"`
	AmountFixed *big.Rat      `avro:"amount_fixed"`
	RawFixed    [4]byte       `avro:"raw_fixed"`
	Status      string        `avro:"status"`
}

func logicalFixture(t *testing.T) []byte {
	t.Helper()
	v := &logicalValue{
		TSMillis:    time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC),
		TSMicros:    time.Date(2026, 3, 15, 10, 30, 0, 123000, time.UTC),
		Day:         time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		TODMillis:   14*time.Hour + 5*time.Minute,
		ID:          "b3d1c0de-0000-4000-8000-000000000001",
		Amount:      big.NewRat(198765, 100), // 1987.65, fits precision 8 scale 2
		AmountFixed: big.NewRat(123456, 100), // 1234.56, fits fixed(4) at precision 8 scale 2
		RawFixed:    [4]byte{0xde, 0xad, 0xbe, 0xef},
		Status:      "ACTIVE",
	}
	b, err := hamba.Marshal(hamba.MustParse(logicalSchemaJSON), v)
	if err != nil {
		t.Fatalf("encoding logical fixture: %v", err)
	}
	return b
}

// testLogicalTypes covers timestamp-millis/micros, date, time-millis, uuid,
// bytes- and fixed-backed decimal, a plain (non-logical) fixed field and an
// enum, in one pass — the fields that separate codecs from each other far
// more than the primitives every codec agrees on.
func testLogicalTypes(t *testing.T) {
	payload := logicalFixture(t)
	keys := []string{"ts_millis", "ts_micros", "day", "tod_millis", "id", "amount", "amount_fixed", "raw_fixed", "status"}
	for _, lib := range libs {
		out, err := probeDynamic(lib, logicalSchemaJSON, payload)
		if err != nil {
			t.Errorf("%s: decoding logical-types payload: %v", lib, err)
			continue
		}
		m, ok := out.(map[string]any)
		if !ok {
			t.Errorf("%s: decoded to %T, not a record", lib, out)
			continue
		}
		for _, k := range keys {
			logField(t, lib, k, m[k])
		}
	}
}

// testNullableUnion decodes the shared `union` case and confirms the
// divergence ANALYSIS.md already documents from source: goavro wraps every
// non-null union branch in a single-key map naming the branch
// ({"string": "u-99213"}), the struct-mode libraries return the bare value.
// Recorded here, alongside every other probe, rather than only in prose.
func testNullableUnion(t *testing.T) {
	var c Case
	for _, cc := range Cases {
		if cc.Name == "union" {
			c = cc
		}
	}
	if c.Name == "" {
		t.Fatal("union case not found in Cases")
	}
	keys := []string{"user_id", "score", "payload"}
	for _, lib := range libs {
		out := decodeDynamic(t, lib, c.SchemaJSON, c.Payload)
		m, ok := out.(map[string]any)
		if !ok {
			t.Errorf("%s: decoded to %T, not a record", lib, out)
			continue
		}
		for _, k := range keys {
			logField(t, lib, k, m[k])
		}
	}
}

// evoWriterValue matches schemas/evo_writer.avsc: an int id, a name, and a
// legacy_flag the reader schema (schemas/evo_reader.avsc) has since dropped.
// The reader adds an "email" field with a default and promotes id to long —
// added field, removed field and type promotion in one pair of schemas.
type evoWriterValue struct {
	ID         int32  `avro:"id"`
	Name       string `avro:"name"`
	LegacyFlag bool   `avro:"legacy_flag"`
}

// testSchemaEvolution decodes writer-schema bytes under a resolved
// reader schema — the shape a consumer needs after a producer has moved on
// to a new version. hamba and iskorotkov share the same resolver API
// (SchemaCompatibility.Resolve(reader, writer)); twmb has its own
// (Resolve(writer, reader) — note the reversed argument order, called out
// in its own doc comment as a deliberate departure from hamba's). goavro has
// no resolution API at all: one Codec is built from one schema, so reading
// old data under a new schema is left entirely to the caller. That is a
// capability gap, not a representational difference, and is recorded as one
// rather than attempted with a schema the decode was never meant to handle.
func testSchemaEvolution(t *testing.T) {
	payload, err := hamba.Marshal(hamba.MustParse(evoWriterSchemaJSON), &evoWriterValue{
		ID: 42, Name: "old-record", LegacyFlag: true,
	})
	if err != nil {
		t.Fatalf("encoding evolution fixture: %v", err)
	}
	keys := []string{"id", "name", "email", "legacy_flag"}

	t.Run("hamba", func(t *testing.T) {
		writer, reader := hamba.MustParse(evoWriterSchemaJSON), hamba.MustParse(evoReaderSchemaJSON)
		resolved, err := hamba.NewSchemaCompatibility().Resolve(reader, writer)
		if err != nil {
			t.Errorf("hamba: resolving schemas: %v", err)
			return
		}
		var out any
		if err := hamba.Unmarshal(resolved, payload, &out); err != nil {
			t.Errorf("hamba: decoding under resolved schema: %v", err)
			return
		}
		m := out.(map[string]any)
		for _, k := range keys {
			logField(t, "hamba", k, m[k])
		}
	})

	t.Run("iskorotkov", func(t *testing.T) {
		writer, reader := isko.MustParse(evoWriterSchemaJSON), isko.MustParse(evoReaderSchemaJSON)
		resolved, err := isko.NewSchemaCompatibility().Resolve(reader, writer)
		if err != nil {
			t.Errorf("iskorotkov: resolving schemas: %v", err)
			return
		}
		var out any
		if err := isko.Unmarshal(resolved, payload, &out); err != nil {
			t.Errorf("iskorotkov: decoding under resolved schema: %v", err)
			return
		}
		m := out.(map[string]any)
		for _, k := range keys {
			logField(t, "iskorotkov", k, m[k])
		}
	})

	t.Run("twmb", func(t *testing.T) {
		writer, reader := twmb.MustParse(evoWriterSchemaJSON), twmb.MustParse(evoReaderSchemaJSON)
		resolved, err := twmb.Resolve(writer, reader) // writer, reader — reversed from hamba
		if err != nil {
			t.Errorf("twmb: resolving schemas: %v", err)
			return
		}
		var out any
		if _, err := resolved.Decode(payload, &out); err != nil {
			t.Errorf("twmb: decoding under resolved schema: %v", err)
			return
		}
		m := out.(map[string]any)
		for _, k := range keys {
			logField(t, "twmb", k, m[k])
		}
	})

	t.Run("goavro", func(t *testing.T) {
		t.Log("goavro: no writer/reader schema resolution API (single-schema Codec only) — reading old data under a new schema is not supported; the caller would have to decode with the writer schema and reshape the result by hand")
	})
}

type address struct {
	City string `avro:"city"`
	Zip  string `avro:"zip"`
}

type company struct {
	HQ     address `avro:"hq"`
	Branch address `avro:"branch"`
}

// testSchemaReferences checks that a single named record ("Address"),
// declared once and referenced a second time by its full name, parses and
// decodes cleanly in every library — schema-registry schemas built from
// several small records commonly rely on this.
func testSchemaReferences(t *testing.T) {
	payload, err := hamba.Marshal(hamba.MustParse(refsSchemaJSON), &company{
		HQ:     address{City: "Warsaw", Zip: "00-001"},
		Branch: address{City: "Krakow", Zip: "30-001"},
	})
	if err != nil {
		t.Fatalf("encoding refs fixture: %v", err)
	}
	for _, lib := range libs {
		out, err := probeDynamic(lib, refsSchemaJSON, payload)
		if err != nil {
			t.Errorf("%s: named-type reuse: %v", lib, err)
			continue
		}
		m, ok := out.(map[string]any)
		if !ok {
			t.Errorf("%s: decoded to %T, not a record", lib, out)
			continue
		}
		logField(t, lib, "hq", m["hq"])
		logField(t, lib, "branch", m["branch"])
	}
}

// testDefaultMissingField encodes from a map missing the "email" key —
// present in schemas/evo_reader.avsc with a default, absent from the Go
// value being encoded — and checks whether each library fills the schema
// default (rather than erroring on a missing field) on the way to bytes,
// then decodes back to show what value actually landed on the wire.
func testDefaultMissingField(t *testing.T) {
	encoders := map[string]func() ([]byte, error){
		"hamba": func() ([]byte, error) {
			return hamba.Marshal(hamba.MustParse(evoReaderSchemaJSON), map[string]any{"id": int64(7), "name": "Alice"})
		},
		"iskorotkov": func() ([]byte, error) {
			return isko.Marshal(isko.MustParse(evoReaderSchemaJSON), map[string]any{"id": int64(7), "name": "Alice"})
		},
		"twmb": func() ([]byte, error) {
			return twmb.MustParse(evoReaderSchemaJSON).Encode(map[string]any{"id": int64(7), "name": "Alice"})
		},
		"goavro": func() ([]byte, error) {
			codec, err := goavro.NewCodec(evoReaderSchemaJSON)
			if err != nil {
				return nil, err
			}
			return codec.BinaryFromNative(nil, map[string]any{"id": int64(7), "name": "Alice"})
		},
	}
	for _, lib := range libs {
		payload, err := encoders[lib]()
		if err != nil {
			t.Logf("%-11s encode from map missing default field: %v", lib, err)
			continue
		}
		out, err := probeDynamic(lib, evoReaderSchemaJSON, payload)
		if err != nil {
			t.Errorf("%s: decoding own default-filled output: %v", lib, err)
			continue
		}
		m := out.(map[string]any)
		logField(t, lib, "email", m["email"])
	}
}

// tinyRecord encodes to five bytes, small enough to fit in whatever spare
// capacity a codec's cached single-object header carries.
type tinyRecord struct {
	N int32 `avro:"n"`
}

const tinySchemaJSON = `{"type":"record","name":"Tiny","fields":[{"name":"n","type":"int"}]}`

// testSingleObjectEncodeOwnership encodes two different values back to back
// through each library's single-object path and checks whether the first
// result survives the second call. A codec that appends the payload to its
// cached header hands both callers the same backing array whenever the
// payload fits in the header's spare capacity, so the earlier result changes
// under the caller — visible only on small records, which is why the fixtures
// in Cases never showed it. iskorotkov fixed this in its soe package
// (PR #37, merged 2026-08-18); hamba v2.31.0 is the last release and carries it.
func testSingleObjectEncodeOwnership(t *testing.T) {
	// One codec per library, held across both calls the way a pipeline holds
	// it. twmb and goavro have no codec object for this path: the caller
	// passes the destination buffer, so ownership is theirs by construction.
	hambaCodec, err := hambasoe.NewCodec(hamba.MustParse(tinySchemaJSON))
	if err != nil {
		t.Fatalf("hamba: %v", err)
	}
	iskoCodec, err := iskosoe.NewCodec(isko.MustParse(tinySchemaJSON))
	if err != nil {
		t.Fatalf("iskorotkov: %v", err)
	}
	twmbSchema := twmb.MustParse(tinySchemaJSON)
	goavroCodec, err := goavro.NewCodec(tinySchemaJSON)
	if err != nil {
		t.Fatalf("goavro: %v", err)
	}
	encoders := map[string]func(v *tinyRecord) ([]byte, error){
		"hamba":      func(v *tinyRecord) ([]byte, error) { return hambaCodec.Encode(v) },
		"iskorotkov": func(v *tinyRecord) ([]byte, error) { return iskoCodec.Encode(v) },
		"twmb":       func(v *tinyRecord) ([]byte, error) { return twmbSchema.AppendSingleObject(nil, v) },
		"goavro": func(v *tinyRecord) ([]byte, error) {
			return goavroCodec.SingleFromNative(nil, map[string]any{"n": v.N})
		},
	}
	for _, lib := range libs {
		enc := encoders[lib]
		a, err := enc(&tinyRecord{N: 1})
		if err != nil {
			t.Errorf("%s: first encode: %v", lib, err)
			continue
		}
		snapshot := append([]byte(nil), a...)
		b, err := enc(&tinyRecord{N: 2})
		if err != nil {
			t.Errorf("%s: second encode: %v", lib, err)
			continue
		}
		intact := string(a) == string(snapshot)
		shared := &a[0] == &b[0]
		t.Logf("%-11s first result intact after second encode: %-5v shares backing array: %v", lib, intact, shared)
		if !intact {
			t.Logf("%-11s first was %x, became %x", lib, snapshot, a)
		}
	}
}
