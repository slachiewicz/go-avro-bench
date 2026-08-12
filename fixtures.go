package avrobench

import (
	_ "embed"

	hamba "github.com/hamba/avro/v2"
)

//go:embed schemas/flat.avsc
var FlatSchemaJSON string

//go:embed schemas/nested.avsc
var NestedSchemaJSON string

//go:embed schemas/union.avsc
var UnionSchemaJSON string

// Case is one schema plus the value and payload every codec is measured against.
// Payload is produced once, by one encoder, so each decoder reads identical bytes.
type Case struct {
	Name       string
	SchemaJSON string
	Value      any    // typed Go value matching the schema
	Payload    []byte // canonical binary encoding of Value
}

func strp(s string) *string   { return &s }
func f64p(f float64) *float64 { return &f }
func i32p(i int32) *int32     { return &i }

func flatValue() *Flat {
	return &Flat{
		ID:     234765,
		Seq:    9007199254740991,
		Name:   "wolverine",
		Ratio:  85.25,
		Total:  32.75,
		Active: true,
		Blob:   []byte("0123456789abcdef"),
	}
}

func nestedValue() *Superhero {
	return &Superhero{
		ID:            234765,
		AffiliationID: 9867,
		Name:          "Wolverine",
		Life:          85.25,
		Energy:        32.75,
		Powers: []*Superpower{
			{ID: 2345, Name: "Bone Claws", Damage: 5, Energy: 1.15, Passive: false},
			{ID: 2346, Name: "Regeneration", Damage: -2, Energy: 0.55, Passive: true},
			{ID: 2347, Name: "Adamant skeleton", Damage: -10, Energy: 0, Passive: true},
		},
	}
}

func unionValue() *Event {
	return &Event{
		ID:      "b3d1c0de-0000-4000-8000-000000000001",
		Source:  "MOBILE",
		UserID:  strp("u-99213"),
		Score:   f64p(0.8125),
		Retries: nil, // exercises the null branch
		Labels:  map[string]string{"region": "eu-west-1", "tier": "gold", "app": "checkout"},
		Tags:    []string{"a", "b", "c", "d"},
		Payload: []byte("payload-bytes"),
	}
}

func wideValue() *Wide {
	w := &Wide{ID: "evt-000000000000001", TS: 1786000000000}
	s := []string{"alpha", "beta", "gamma", "delta"}
	for i := 0; i < 16; i++ {
		if i%3 == 0 {
			continue // leave a third of the unions null
		}
		v := s[i%len(s)]
		switch i {
		case 1:
			w.S1 = &v
		case 2:
			w.S2 = &v
		case 4:
			w.S4 = &v
		case 5:
			w.S5 = &v
		case 7:
			w.S7 = &v
		case 8:
			w.S8 = &v
		case 10:
			w.S10 = &v
		case 11:
			w.S11 = &v
		case 13:
			w.S13 = &v
		case 14:
			w.S14 = &v
		}
	}
	n := int64(42)
	w.N0, w.N2, w.N4, w.N6, w.N8, w.N10 = &n, &n, &n, &n, &n, &n
	d := 3.14159
	w.D0, w.D2, w.D4, w.D6 = &d, &d, &d, &d
	w.B0, w.B2, w.B4 = true, true, true
	return w
}

// Cases is the corpus. Payloads are encoded with hamba because it is the only
// codec here that takes typed structs and handles unions from pointers; the
// bytes are plain Avro and every decoder is checked against them in
// TestConformance.
var Cases []Case

func init() {
	for _, c := range []struct {
		name   string
		schema string
		value  any
	}{
		{"flat", FlatSchemaJSON, flatValue()},
		{"nested", NestedSchemaJSON, nestedValue()},
		{"union", UnionSchemaJSON, unionValue()},
		{"wide", WideSchemaJSON, wideValue()},
	} {
		s := hamba.MustParse(c.schema)
		b, err := hamba.Marshal(s, c.value)
		if err != nil {
			panic("encoding fixture " + c.name + ": " + err.Error())
		}
		Cases = append(Cases, Case{Name: c.name, SchemaJSON: c.schema, Value: c.value, Payload: b})
	}
}

// NewTyped returns a freshly allocated destination of the right concrete type.
// Decode benchmarks call this every iteration: reusing one destination across
// b.N is the single biggest reason published Avro benchmarks disagree with
// production, because a pipeline keeps every value it decodes.
func (c Case) NewTyped() any {
	switch c.Name {
	case "flat":
		return &Flat{}
	case "nested":
		return &Superhero{}
	case "union":
		return &Event{}
	case "wide":
		return &Wide{}
	}
	panic("unknown case " + c.Name)
}

//go:embed schemas/wide.avsc
var WideSchemaJSON string

// Wide models a schema-registry record: 44 fields, most of them nullable
// unions. Per-message overhead and per-field cost separate here in a way the
// small fixtures cannot show.
type Wide struct {
	ID  string   `avro:"id"`
	TS  int64    `avro:"ts"`
	S0  *string  `avro:"s0"`
	S1  *string  `avro:"s1"`
	S2  *string  `avro:"s2"`
	S3  *string  `avro:"s3"`
	S4  *string  `avro:"s4"`
	S5  *string  `avro:"s5"`
	S6  *string  `avro:"s6"`
	S7  *string  `avro:"s7"`
	S8  *string  `avro:"s8"`
	S9  *string  `avro:"s9"`
	S10 *string  `avro:"s10"`
	S11 *string  `avro:"s11"`
	S12 *string  `avro:"s12"`
	S13 *string  `avro:"s13"`
	S14 *string  `avro:"s14"`
	S15 *string  `avro:"s15"`
	N0  *int64   `avro:"n0"`
	N1  *int64   `avro:"n1"`
	N2  *int64   `avro:"n2"`
	N3  *int64   `avro:"n3"`
	N4  *int64   `avro:"n4"`
	N5  *int64   `avro:"n5"`
	N6  *int64   `avro:"n6"`
	N7  *int64   `avro:"n7"`
	N8  *int64   `avro:"n8"`
	N9  *int64   `avro:"n9"`
	N10 *int64   `avro:"n10"`
	N11 *int64   `avro:"n11"`
	D0  *float64 `avro:"d0"`
	D1  *float64 `avro:"d1"`
	D2  *float64 `avro:"d2"`
	D3  *float64 `avro:"d3"`
	D4  *float64 `avro:"d4"`
	D5  *float64 `avro:"d5"`
	D6  *float64 `avro:"d6"`
	D7  *float64 `avro:"d7"`
	B0  bool     `avro:"b0"`
	B1  bool     `avro:"b1"`
	B2  bool     `avro:"b2"`
	B3  bool     `avro:"b3"`
	B4  bool     `avro:"b4"`
	B5  bool     `avro:"b5"`
}

// ConfluentFrame prepends the Confluent wire-format header: one magic byte then
// a big-endian schema ID. schema_registry_decode strips this before every
// decode, so an end-to-end measurement has to include it.
func ConfluentFrame(schemaID uint32, payload []byte) []byte {
	out := make([]byte, 5+len(payload))
	out[0] = 0
	out[1] = byte(schemaID >> 24)
	out[2] = byte(schemaID >> 16)
	out[3] = byte(schemaID >> 8)
	out[4] = byte(schemaID)
	copy(out[5:], payload)
	return out
}
