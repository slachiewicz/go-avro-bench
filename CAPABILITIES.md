# Capabilities

What each codec can and cannot do. For Bento this decides more than any timing
in [RESULTS.md](RESULTS.md): two of the three replacement candidates cannot
implement its existing paths at all.

Probed by `TestFeatureParity` and the skip reasons recorded in
`encodings_test.go`, `ocf_test.go` and `pipeline_test.go`. Versions are the ones
pinned in `go.mod`: hamba v2.31.0 (final, archived), iskorotkov v2.33.2-dev,
twmb v1.7.3-dev, goavro v2.15.1-dev.

## Wire encodings

| Encoding | hamba | iskorotkov | twmb | goavro |
| --- | --- | --- | --- | --- |
| binary | yes | yes | yes | yes |
| textual (Avro JSON) | **no** | **no** | yes | yes |
| single-object | yes (`soe`) | yes (`soe`) | yes | yes |

**The textual gap is disqualifying for Bento.** Four call sites need it:
`internal/impl/confluent/serde_avro.go:203` runs `TextualFromNative` on every
schema-registry decode unconditionally — `avro_raw_json` only chooses which
textual codec, `NewCodec` or `NewCodecForStandardJSONFull`, and both are
textual — plus `internal/impl/avro/scanner.go:119`, `internal/codec/reader.go:467`,
and `internal/impl/avro/processor.go`, which exposes textual to users in both
directions.

hamba v2.31.0 and iskorotkov v2.33.2-dev have no Avro textual data codec
anywhere: the core package's only JSON methods serialise the schema itself, and
`ocf/`, `soe/`, `registry/`, `gen/`, `cmd/` and `pkg/` contain none. Adopting
either means writing that codec.

twmb's `EncodeJSON(..., TaggedUnions())` reproduces goavro's `TextualFromNative`
output on all four fixtures — same union wrapping, same bytes escaping, same
number formatting — with one difference: it emits fields in schema order where
goavro emits Go map-iteration order, which varies run to run. `serde_avro.go:207`
puts that JSON straight into user pipelines, so a migration would make output
field order deterministic. A behaviour change, and probably an improvement.

## Encoding from generic JSON

Bento's `schema_registry_encode` is JSON in, binary out
(`serde_avro.go:151-156`). `json.Unmarshal` yields `float64` for every number, and bytes arrive as a
plain string — goavro's textual output escapes them rather than base64-encoding
them, so a producer that did use base64 would need a different rule again.

| | accepts JSON-decoded `any` |
| --- | --- |
| goavro | yes — `NativeFromTextual` is its own entry point |
| twmb | yes |
| hamba | **no** — `id: avro: float64 is unsupported for Avro int` |
| iskorotkov | **no** — same, it is the same code |

A minimal schema-driven coercion walker (float64 to int/long/float, string to
bytes, enum symbols, union branch selection — no logical types, no
defaults, no aliases) rescued all four fixtures for both. It came to **220
lines**, near-duplicated across the two because `hamba.Schema` and
`isko.Schema` are structurally identical but nominally distinct types.

For scale, `redpanda-data/connect` deleted a 571-line
`normalize_for_avro_schema.go` when it moved to twmb in
[#4195](https://github.com/redpanda-data/connect/pull/4195). 220 lines is ~38%
of that for something explicitly minimal, so it is a floor on the real cost, not
an estimate of it.

## Schema evolution

| | reader/writer resolution |
| --- | --- |
| hamba | `NewSchemaCompatibility().Resolve(reader, writer)` |
| iskorotkov | same |
| twmb | `Resolve(writer, reader)` — **arguments reversed** |
| goavro | **not supported** — one `Codec` is one schema (`codec.go:113`) |

goavro's `CodecOption.IgnoreExtraFieldsFromTextual` (`codec.go:53-58`) skips
unknown JSON fields on textual decode, which is limited forward compatibility
and not resolution: every one of its six constructors takes a single schema and
there is no two-schema API.

Two traps here. goavro cannot read data written under a different schema at all,
so added fields, dropped fields and int-to-long promotion are the caller's
problem. And twmb deliberately reverses hamba's argument order, so a port
between them compiles and silently resolves backwards.

## Value representation, dynamic decode

| Probe | hamba | iskorotkov | twmb | goavro |
| --- | --- | --- | --- | --- |
| timestamp-millis / micros, date | `time.Time` | `time.Time` | `time.Time` | `time.Time` |
| time-millis | `time.Duration` | `time.Duration` | `time.Duration` | `time.Duration` |
| uuid | `string` | `string` | `string` | `string` |
| decimal (bytes- and fixed-backed) | `*big.Rat` | `*big.Rat` | `*big.Rat` | `*big.Rat` |
| fixed, non-logical | `[N]uint8` | `[N]uint8` | `[]uint8` | `[]uint8` |
| enum | `string` | `string` | `string` | `string` |
| nullable union, non-null branch | bare value | bare value | bare value | `map[string]any{"<type>": v}` |
| Avro `int` | `int` | `int` | `int32` | `int32` |

Two corrections to claims that circulate from connect's #4195 description:

- **Decimals are `*big.Rat` in all four**, twmb included. The `json.Number`
  behaviour that PR describes belongs to connect's own JSON serialisation layer,
  not to twmb's decode.
- **The `fixed` split is `[N]byte` against `[]byte`**, and it follows the fork
  line (hamba/iskorotkov one way, twmb/goavro the other). The "integer array to
  base64" break in that PR was likewise connect's JSON layer, not a property of
  any of these Go APIs.

The union row is the one that constrains a Bento migration: goavro wraps every
non-null branch in a single-key map, and Bloblang mappings over Confluent Avro
are written against that shape.

## Object container files

| | null | deflate | snappy |
| --- | --- | --- | --- |
| hamba | yes | yes | yes |
| iskorotkov | yes | yes | yes |
| twmb | yes | yes | yes |
| goavro | yes | yes | yes |

Complete parity, no skips — the only area with any. goavro and hamba name codecs
with the same strings; twmb needs an explicit `ocf.DeflateCodec(...)` or
`ocf.SnappyCodec()` since it defaults to null. Every reader resolves compression
from the file header, so no reader-side option is needed anywhere.

This matters because OCF is the only Avro path upstream `redpanda-data/benthos`
has (`internal/codec/reader.go`), so a conclusion here applies to both trees.

## Summary

Among the replacement candidates, only twmb implements every path Bento runs
today without a new codec layer. hamba and iskorotkov lack the textual codec
four Bento call sites require and need a coercion walker for the encode path.

goavro is not a candidate but the incumbent, and it covers Bento's existing
paths by definition — it is what implements them. Its two weaknesses are of
different kinds: no evolution story at all, and a union representation that is
the compatibility constraint any migration has to shim, since users' Bloblang
mappings are written against it.
