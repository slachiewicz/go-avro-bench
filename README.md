# go-avro-bench

Comparing Go Avro codecs on equal terms, to pick one deliberately.

Companion to [ANALYSIS.md](ANALYSIS.md), which covers who depends on which
library, what the `hamba/avro` archival did to the ecosystem, and the one
behavioural difference that constrains any migration regardless of speed.

## Why not use the existing numbers

The table that circulates for Go Avro comes from
[`nrwiersma/avro-benchmarks`](https://github.com/nrwiersma/avro-benchmarks). Three
things about it:

- **Same author as the winner.** `nrwiersma` owns both that repo and
  `hamba/avro`. The repo is archived; last push 2023-07-27, results reported on
  Go 1.20.
- **The decode loops do different work.** hamba allocates its destination struct
  once, before `b.ResetTimer()`, and refills it every iteration — including the
  inner slice's backing array. goavro calls `NativeFromBinary`, building a fresh
  `map[string]any` tree each pass. `4 allocs / 64 B` against `35 allocs / 1688 B`
  is largely *reuse a struct* versus *build a map*, and a pipeline that keeps its
  decoded values never gets the first deal.
- **One narrow schema.** Five scalars plus an array of nested records. No unions,
  maps, enums or logical types — exactly the features that separate codecs.

hamba is genuinely fast, and reuse does not explain all of 287.9 ns against
1026 ns. The direction holds; the magnitude does not transfer to a consumer that
needs dynamic output.

## What this harness does differently

**Fresh destination every iteration.** `BenchmarkDecodeTyped` allocates through
`Case.NewTyped()` inside the loop. `BenchmarkDecodeTypedReused` keeps the old
behaviour so the gap is visible and labelled rather than silently folded into a
comparison.

**Dynamic and typed measured apart.** `BenchmarkDecodeDynamic` decodes into
`any`; `BenchmarkDecodeTyped` into a struct. goavro has no struct mode, so it
appears only in the dynamic set — an API difference reported as one, not
smuggled into a single ranking.

**Schemas that exercise the divergences.** `flat` (primitives), `nested`
(the `nrwiersma` record, for continuity) and `union` (nullable unions, enum,
map, array, bytes — the shape Confluent schema-registry traffic takes).

**Schema compilation timed separately.** `BenchmarkParse`, because a pipeline
caching one codec per schema and a pipeline compiling per message have different
costs.

**One payload, every decoder.** Fixtures are encoded once; `TestRoundTripTyped`
asserts each struct-mode codec reproduces those exact bytes, so nobody is
measured against a payload only their own encoder agrees with.

## Libraries

| Import path | Mode |
| --- | --- |
| `github.com/hamba/avro/v2` | dynamic + typed — archived, baseline |
| `github.com/iskorotkov/avro/v2` | dynamic + typed — maintained fork of hamba |
| `github.com/twmb/avro` | dynamic + typed |
| `github.com/linkedin/goavro/v2` | dynamic only |

Not included, with reasons: `actgardner/gogen-avro` needs a code-generation step
and is dormant since 2024; `heetch/avro` is code-generation plus a type registry,
a different integration model; `go-avro/avro` and `khezen/avro` are unmaintained.
`gogen-avro` is the strongest of these and is the obvious next addition if a
build step is acceptable.

## Running

```sh
make bench      # -count=10 into bench.txt
make stat       # benchstat over the result
make test       # conformance and round-trip
make shapes     # print what each codec returns for the same bytes
```

Report the Go version and machine alongside any numbers. These are allocation
and latency measurements on one box; `allocs/op` is stable and comparable,
`ns/op` needs a quiet machine before it means anything.

## Status

Harness and analysis complete; results not yet interpreted here. `bench.txt` is
generated, not committed.
