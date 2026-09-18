# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A benchmark and conformance harness comparing four Go Avro codecs on equal
terms — `hamba/avro/v2` (archived), `iskorotkov/avro/v2` (hamba fork),
`twmb/avro` and `linkedin/goavro/v2` — written to decide which codec Bento
(`redpanda-data/connect`) should stand on. It is a working draft: the README
says not to cite the numbers as settled, and conclusions have been reversed
during writing. The deliverables are the prose files, not a library:
`RESULTS.md` (timings), `CAPABILITIES.md` (what each codec can do),
`ANALYSIS.md` (ecosystem and dependents).

## Commands

```sh
make test                     # go test ./...  — conformance, round-trip, feature parity
make shapes                   # TestDynamicShapes -v: what each codec returns for the same bytes
make bench                    # all benchmarks, -count=10, -timeout 30m, tee'd to bench.txt (gitignored)
make stat                     # benchstat bench.txt
go vet ./...
```

`benchstat` is not on PATH here; use
`go run golang.org/x/perf/cmd/benchstat@latest -col /lib bench.txt`.
`-col /lib` pivots the per-library leaf of the benchmark name into columns,
which is why every benchmark nests `<family>/<schema>/.../<lib>`.

Single test or benchmark:

```sh
go test -run 'TestFeatureParity/nullable_union' -v .
go test -run '^$' -bench 'BenchmarkDecodeDynamic/union' -benchmem -count=10 .
```

Published numbers were collected one family per process (`-bench
'^BenchmarkOCF'` etc.) into `part1.txt`–`part5.txt`, which are committed raw
data; the table below says which family lives in which file. Later full
recollections live under `runs/<date>/part*.txt` and are cited from
`RESULTS.md` without replacing the top-level set. Run benchmarks on a quiet
machine: `allocs/op` is stable, `ns/op` is not — a Go toolchain bump can move
`encoding/json` and `compress/flate` arms by 30% on its own, so rerun one arm
under both toolchains (`GOTOOLCHAIN=goX -modfile=<copy with old go line>`)
before attributing a shift to a library.

## Layout

One package, `avrobench`, no `main`. Non-test code is two files:

- `types.go` — the typed destinations (`Flat`, `Superhero`, `Event`, `Wide`)
  with `avro:"..."` tags, which hamba, iskorotkov and twmb all honour.
- `fixtures.go` — `Cases`, the corpus. Each `Case` is a schema (embedded from
  `schemas/*.avsc`), a typed value, and a `Payload` encoded **once, with
  hamba, at init**, so every decoder reads identical bytes.
  `Case.NewTyped()` allocates a fresh destination; `ConfluentFrame` adds the
  5-byte schema-registry header.

Benchmark families, one per file, and the raw file each was collected into:

| File | Benchmarks | Models |
| --- | --- | --- |
| `decode_test.go` | `DecodeDynamic`, `DecodeTyped`, `DecodeTypedReused`, `Parse` | single-stage decode; the nrwiersma comparison (`part1`) |
| `pipeline_test.go` | `SRDecode`, `SREncode`, `SREncodeCoerced`, `DecodeStage` | Bento's `serde_avro.go`: framed binary ↔ JSON bytes (`part2`) |
| `pulsar_test.go` | `PulsarPattern`, `CodecCacheEffect` | pulsar-client-go's cached-codec typed path (`part3`) |
| `encodings_test.go` | `EncodingDecode`, `EncodingEncode` | binary / textual / single-object wire encodings (`part4`) |
| `ocf_test.go` | `OCFRead`, `OCFWrite` | object container files, null/deflate/snappy, 1000 records per op (`part5`) |

Tests: `conformance_test.go` (`TestDynamicShapes`, `TestRoundTripTyped` —
each struct codec must reproduce the fixture bytes exactly) and
`parity_test.go` (`TestFeatureParity` — a decision table, not pass/fail; it
only fails when a codec errors or panics on spec-valid input).

## Conventions the harness depends on

These are the method; changing one silently changes what the numbers mean.

- **Fresh destination every iteration.** Decode loops allocate inside the
  loop. `BenchmarkDecodeTypedReused` is the one deliberate exception, kept and
  labelled so the reuse artefact is visible.
- **Typed and dynamic never share a row.** goavro has no struct mode, so its
  typed arm is `b.Skipf("goavro has no typed struct mode; see the dynamic arm")`,
  not an omission. hamba and iskorotkov have no Avro textual codec; both skip
  with the same reason string. Skip reasons are findings — `CAPABILITIES.md`
  is written from them and from `TestFeatureParity` output. Reuse the existing
  reason strings verbatim.
- **Library as the leaf of the benchmark name**, always one of
  `hamba`, `iskorotkov`, `twmb`, `goavro` (the `libs` slice in
  `conformance_test.go`), with import aliases `hamba`, `isko`, `twmb`,
  `goavro`. Pipeline benchmarks add dialect suffixes (`goavro-plain`,
  `goavro-stdjson`, `twmb-bare`, `twmb-wrapped`) because goavro's two codec
  constructors and twmb's `TaggedUnions()` produce different JSON; the decode
  families add `twmb-alias` (`AliasInput()`, zero-copy strings) as a
  labelled variant, like `DecodeTypedReused`, never as the default arm.
- **twmb's `Resolve(writer, reader)` reverses hamba's
  `Resolve(reader, writer)`.** A port between them compiles and resolves
  backwards.
- **Dependencies are pinned at branch tips** (pseudo-versions in `go.mod`),
  except hamba at v2.31.0, its final release. Bumping a pin is a result in
  itself (`RESULTS.md`, "Library version is worth as much as library choice");
  re-collect the affected families and record the old figure before overwriting.
  The hamba/iskorotkov "no textual encoding" skips are hard-coded, not probed,
  so after a bump grep the module source for a textual codec before keeping
  them; `TestFeatureParity` and `TestDynamicShapes` log every representation,
  so diff their `-v` output between the old and new pins.

## Adding a schema or a library

New schema: `schemas/<name>.avsc` + `//go:embed` var, a struct in `types.go`,
a value constructor and an entry in the `Cases` init loop in `fixtures.go`,
and a case in `NewTyped()`. `TestRoundTripTyped` then checks every struct
codec against the hamba-encoded bytes.

New library: add it to `libs`, `decodeDynamic` and `probeDynamic`, then an arm
in every family, skipping with a stated reason wherever it cannot do the work.

## Writing the prose files

Never write a number that was not read from `part*.txt` or fresh benchmark
output in the same session; recompute ratios from those values. Tables in
`RESULTS.md` cite the family and file they came from. Bento call-site
references (`serde_avro.go:203` etc.) are to `redpanda-data/connect` and
should be re-checked against the current tree before being repeated.
