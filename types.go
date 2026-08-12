// Package avrobench compares Go Avro codecs on equal terms.
//
// The struct tags below use `avro:"..."`, which hamba, iskorotkov and twmb all
// honour. goavro has no struct mode at all and appears only in the dynamic
// benchmarks — that asymmetry is a result, not an oversight.
package avrobench

// Flat exercises the primitive types only.
type Flat struct {
	ID     int32   `avro:"id"`
	Seq    int64   `avro:"seq"`
	Name   string  `avro:"name"`
	Ratio  float32 `avro:"ratio"`
	Total  float64 `avro:"total"`
	Active bool    `avro:"active"`
	Blob   []byte  `avro:"blob"`
}

// Superpower is the inner record of Superhero.
type Superpower struct {
	ID      int32   `avro:"id"`
	Name    string  `avro:"name"`
	Damage  float32 `avro:"damage"`
	Energy  float32 `avro:"energy"`
	Passive bool    `avro:"passive"`
}

// Superhero mirrors the record used by nrwiersma/avro-benchmarks, so numbers
// here can be lined up against the table that circulates in its README.
type Superhero struct {
	ID            int32         `avro:"id"`
	AffiliationID int32         `avro:"affiliation_id"`
	Name          string        `avro:"name"`
	Life          float32       `avro:"life"`
	Energy        float32       `avro:"energy"`
	Powers        []*Superpower `avro:"powers"`
}

// Event carries nullable unions, an enum and a map. This is the shape Confluent
// schema-registry traffic actually takes, and the one where codecs diverge most
// in both speed and in the value they hand back.
type Event struct {
	ID      string            `avro:"id"`
	Source  string            `avro:"source"`
	UserID  *string           `avro:"user_id"`
	Score   *float64          `avro:"score"`
	Retries *int32            `avro:"retries"`
	Labels  map[string]string `avro:"labels"`
	Tags    []string          `avro:"tags"`
	Payload []byte            `avro:"payload"`
}
