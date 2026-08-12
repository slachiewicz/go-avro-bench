# Go Avro libraries: who uses what, and where the archived ones went

Reference notes gathered while working out which codec Bento should stand on.
Every claim here is checkable from the command shown beside it.

## There is no official Go implementation

`apache/avro` ships `lang/` for c, c++, csharp, java, js, perl, php, py, ruby
and rust. There is no `go`. Every option below is third-party, so "use the
reference implementation" is not available as a tie-breaker.

## The libraries

| Library | Stars | Last push | Status |
| --- | ---: | --- | --- |
| `linkedin/goavro/v2` | 1068 | 2026-01-21 | Maintained. Dynamic only — no struct mode. |
| `hamba/avro/v2` | 509 | 2026-01-18 | **Archived.** README: "This project is no longer maintained. If you wish to update or extend `avro`, please do so in a fork." |
| `actgardner/gogen-avro/v9` | 372 | 2024-02-18 | Dormant. Code generation, needs a build step. |
| `heetch/avro` | 95 | 2025-06-20 | Low activity. Code generation plus schema-registry support. |
| `go-avro/avro` | 55 | 2022-05-17 | Dead. |
| `khezen/avro` | 49 | 2024-11-27 | Dormant. |
| `twmb/avro` | 29 | 2026-08-11 | Active. By the `franz-go` author. Single dependency: `klauspost/compress`. Last tag 2026-04-30. |
| `iskorotkov/avro/v2` | 16 | 2026-07-29 | Fork of `hamba/avro`, carries the GO-2026-5048 fix. |

Two things about `twmb/avro` that the table cannot show. It has not cut a tag
since 2026-04-30 despite steady development, and the gap is not cosmetic: main
is 25-35% faster than `v1.7.2` at dynamic decode and 58% faster to parse
([RESULTS.md](RESULTS.md)). Adopting it means pinning an untagged commit, which
is what `redpanda-data/connect` does. And it is one maintainer at 29 stars —
against `hamba`'s 509 at the point it was archived, which is a reminder that
stars did not keep that one alive either.

## The archive event and what followed

`hamba/avro` was archived in January 2026. The maintainer declined to name a
successor and pointed users at forking
([hamba/avro#595](https://github.com/hamba/avro/issues/595)).

GO-2026-5048 (GHSA-mx64-mj3q-7prj, high, CVSS 7.5) is a decoder
denial-of-service affecting every `hamba/avro/v2` release. With the repository
archived there can be no upstream patch, so the advisory database now tracks the
fix against the fork: `github.com/iskorotkov/avro/v2 < 2.33.0`, patched in
`2.33.0`.

Two migration targets have emerged, and they are not equivalent:

- **`iskorotkov/avro/v2`** — a fork. Same API, same behaviour, CVE patched.
  A drop-in, and a bet on a 16-star repository staying alive.
- **`twmb/avro`** — a rewrite, not a fork. Different API, actively developed,
  and explicitly aiming to combine what `goavro` and `hamba` each have.

## Who depends on Avro in Bento's graph

```
$ go mod graph | grep -E ' github.com/(hamba|linkedin|iskorotkov)[^ ]*avro'
github.com/apache/arrow-go/v18      github.com/hamba/avro/v2
github.com/apache/arrow/go/v15      github.com/hamba/avro/v2
github.com/apache/pulsar-client-go  github.com/hamba/avro/v2
github.com/warpstreamlabs/bento     github.com/hamba/avro/v2
github.com/warpstreamlabs/bento     github.com/iskorotkov/avro/v2
github.com/warpstreamlabs/bento     github.com/linkedin/goavro/v2
```

### Runtime versus test, which is the distinction that matters

Module-graph presence and linked code are different questions, and for Avro they
give different answers.

```
$ go list -deps ./cmd/bento | grep -i avro
github.com/linkedin/goavro/v2
github.com/warpstreamlabs/bento/internal/impl/avro
github.com/warpstreamlabs/bento/public/components/avro
github.com/iskorotkov/avro/v2/pkg/crc64
github.com/hamba/avro/v2
```

- **Both codecs ship.** `goavro` and `hamba` (resolved to `iskorotkov` through
  the `replace`) are compiled into `cmd/bento`. Bento carries two Avro
  implementations in one binary.
- **Arrow's Avro reader is not linked.** `apache/arrow-go/v18` and
  `apache/arrow/go/v15` put `hamba/avro` in the module graph, but no Arrow Avro
  package appears in `go list -deps`. They cost `go.sum` entries and scanner
  noise, nothing more.
- **`gogen-avro` is absent entirely** — it is not a Bento dependency at any
  level.

So the vulnerability surface is narrower than the module graph suggests, but not
empty: `hamba/avro` really is linked.

## How each consumer uses it

### Bento's own components — `linkedin/goavro`

Direct requirement, `go.mod:102`. Four call sites:

```
internal/impl/avro/processor.go
internal/impl/avro/scanner.go
internal/impl/confluent/serde_avro.go
internal/codec/reader.go
```

All dynamic: `codec.NativeFromBinary` into `map[string]any`, because the schema
arrives at runtime from a registry or config. This is exercised code.

### `apache/pulsar-client-go` — `hamba/avro`

One file, `pulsar/schema.go`, three calls:

```go
avro.Parse(schemaDef)             // initAvroCodec
avro.Marshal(as.Codec, data)      // AvroSchema.Encode
avro.Unmarshal(as.Codec, data, v) // AvroSchema.Decode
```

The import is package-level, so linking `pulsar` links `hamba/avro` whether or
not the Avro schema type is ever constructed.

**Bento never constructs it.** `grep -E "AvroSchema|SchemaType" internal/impl/pulsar/*.go`
returns nothing. Bento's Pulsar input and output move bytes. The archived
codec, the `replace` directive and the `go install` breakage it causes are all
carried for code Bento does not execute.

That makes [apache/pulsar-client-go#1526](https://github.com/apache/pulsar-client-go/issues/1526)
and [#1518](https://github.com/apache/pulsar-client-go/pull/1518) the cheapest
fix available: when Pulsar moves off `hamba`, Bento's `replace` and its
`go install` breakage go with it, and no Bento code changes.

### Upstream `redpanda-data/benthos` — one `goavro` use, no problem to solve

```
$ grep -n avro go.mod            # linkedin/goavro/v2 v2.15.0
$ grep -rn linkedin/goavro --include=*.go .
internal/codec/reader.go:23
```

No `internal/impl/avro`, no `internal/impl/confluent`, no `internal/impl/pulsar`,
no Pulsar dependency at all — those connectors live in `redpanda-data/connect`.
Upstream has no `hamba` dependency and therefore no archived-library exposure.

Bento's Avro problem follows directly from keeping in-tree the connectors
upstream moved out. It is not something upstream solved and Bento missed.

Bento is also behind on the shared one: `goavro` v2.12.0 against upstream's
v2.15.0.

## The finding that constrains any migration

Codecs do not agree on what a decoded union *is*. Same schema, same bytes
(`TestDynamicShapes`, `union` case):

| Field | goavro | hamba / iskorotkov / twmb |
| --- | --- | --- |
| `user_id` (`["null","string"]`) | `map[string]any{"string": "u-99213"}` | `"u-99213"` |
| `score` (`["null","double"]`) | `map[string]any{"double": 0.8125}` | `0.8125` |
| `payload` (`["null","bytes"]`) | `map[string]any{"bytes": …}` | `[]byte` |

goavro wraps every non-null union branch in a single-key map naming the branch.
The others return the bare value.

Bento's Bloblang mappings over Confluent Avro are written against the wrapped
shape. Swapping the codec under them changes the message users see, so any
migration is a breaking change to user pipelines, not an internal refactor —
independent of whatever the benchmark says.

A smaller divergence in the same direction: Avro `int` comes back as Go `int`
from hamba and iskorotkov, and as `int32` from twmb and goavro.

## Where that leaves the options

1. **Do nothing to Bento's own code; land the Pulsar fix upstream.** Removes the
   archived codec from the binary, drops the `replace`, restores `go install`,
   changes no user-visible behaviour. Blocked only on pulsar-client-go#1518.
2. **Bump `goavro` to v2.15.0.** Closes the gap with upstream. Independent of
   everything above.
3. **Move Bento's Avro components to another codec.** What the benchmark in this
   repo is for — but the union-shape divergence means it needs a compatibility
   shim and a changelog entry, and the benchmark is not the deciding input.
