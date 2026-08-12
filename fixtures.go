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
	}
	panic("unknown case " + c.Name)
}
