# Results

`go1.26.5 darwin/arm64`, Apple M1, `-count=10`, `benchstat -col /lib`.
Regenerate with `make bench && make stat`.

Library versions are branch tips, not tags — which turns out to matter, see
below. hamba is pinned at v2.31.0 because that is its final release.

The tables were collected against twmb `v1.7.3-0.20260811190909` and
iskorotkov `v2.33.2-0.20260610201905` on Go 1.26.5. `go.mod` has since moved
to twmb `16bc6f5` (v1.9.0 plus 30 commits), iskorotkov `362766d` (v2.34.0
plus 6) and Go 1.27.1. A full recollection on those pins is in
`runs/2026-09-18/`; the section "Pins moved" at the end says what it changed
and why the tables here were kept.

434 benchmark arms, 10 samples each, 5.0 billion iterations, collected in
separate processes per family from one tree. Raw data is in `part1.txt` through
`part5.txt`. The `v1.7.2` column in the version section below is the only figure
from an earlier run, kept because its raw file has since been overwritten.

## The path Bento actually runs

`schema_registry_decode` is two stages, not one: `NativeFromBinary` then
`TextualFromNative` (`serde_avro.go:198-203`). Measuring only the first, as the
tables further down do, gets the ranking wrong — the second stage is the larger
part of the cost.

`DecodeStage`, goavro, `flat`: native **264.3n**, textual **960.7n**. The stage
this file used to omit is 78% of the work.

### End to end, Confluent-framed binary in, JSON bytes out

| Schema | twmb | goavro-plain | goavro-stdjson | hamba | iskorotkov |
| --- | ---: | ---: | ---: | ---: | ---: |
| flat | **891.1n** | 1.220µ | 1.223µ | 1.829µ | 1.806µ |
| nested | **2.576µ** | 3.969µ | 3.977µ | 5.373µ | 5.345µ |
| union | **2.369µ** | 3.083µ | 2.894µ | 3.248µ | 3.251µ |
| wide | **6.046µ** | 10.17µ | 9.350µ | 10.48µ | 10.57µ |

twmb wins every case, by 1.22× to 1.55× over whichever goavro mode is faster,
and by 1.37× to 2.09× over hamba. On allocations the gap is wider: `wide` costs
twmb **50** against goavro's 137-158 and hamba's 169.

That reverses what the single-stage numbers say, where goavro leads on `flat`
and `nested`. Both are correct measurements of different things; only one of
them is the thing a pipeline pays.

### Encoding, JSON bytes in, framed binary out

| Schema | twmb-bare | twmb-wrapped | goavro-plain | goavro-stdjson |
| --- | ---: | ---: | ---: | ---: |
| flat | 1.485µ | 1.342µ | **1.207µ** | 1.223µ |
| nested | 4.160µ | 3.714µ | 3.853µ | 3.826µ |
| union | **2.845µ** | 3.308µ | 3.283µ | 4.645µ |
| wide | **8.061µ** | 11.94µ | 11.46µ | 25.21µ |

goavro leads `flat`. On `nested` twmb-wrapped is ahead by 3%, which is outside
the intervals but too narrow to lean on. twmb leads `union` and `wide` outright. Encoding is the one direction where goavro is
genuinely competitive.

`bare` and `wrapped` are the two JSON dialects: bare values against goavro's
tagged-union envelope. Each is paired with the goavro mode that speaks the same
dialect.

goavro's std-JSON mode costs **25.21µ on `wide`, 2.2× its own plain mode**, both
at ±0-1%. Union-heavy records are exactly where that mode gets chosen.

### What the coercion walker costs

hamba and iskorotkov cannot encode a `json.Unmarshal` value at all, so those
arms are absent above. With the 220-line walker from
[CAPABILITIES.md](CAPABILITIES.md) in front of them:

| Schema | hamba | iskorotkov | twmb-bare (no walker) |
| --- | ---: | ---: | ---: |
| flat | 2.390µ | 2.395µ | **1.500µ** |
| nested | 6.785µ | 6.707µ | **4.135µ** |
| union | 4.332µ | 4.362µ | **2.848µ** |
| wide | 13.19µ | 13.19µ | **8.071µ** |

Shimmed, they lose to unshimmed twmb by 1.52× to 1.64×. The walker does not buy back
the gap; it only makes the comparison possible. `twmb-bare` is the control
because it needs no walker; `twmb-wrapped` is faster still on flat and nested,
so this understates the gap.

## Object container files

The one family where all four libraries support everything — null, deflate and
snappy, no skips. `OCFWrite`, 1000 records, deflate:

| | hamba | iskorotkov | twmb | goavro |
| --- | ---: | ---: | ---: | ---: |
| flat, typed | 756.1µ | 767.1µ | **234.4µ** | n/a |
| nested, typed | 1.006m | 1.011m | **478.9µ** | n/a |
| union, typed | 1.340m | 1.360m | **671.0µ** | n/a |
| wide, typed | 1.408m | 1.497m | **871.2µ** | n/a |
| flat, dynamic | 1.349m | 1.329m | 357.3µ | **303.4µ** |
| nested, dynamic | 2.748m | 2.884m | 792.7µ | **640.3µ** |
| union, dynamic | 2.415m | 2.453m | 968.5µ | **855.2µ** |
| wide, dynamic | 5.942m | 5.801m | **1.806m** | 2.507m |

Against hamba, twmb writes typed 3.2× faster on `flat` but only 1.6-2.1× on the
other shapes, and dynamic 2.5-3.8× faster depending on shape. goavro leads the
dynamic column everywhere except `wide`, where twmb takes it back by 1.39×.
goavro has no typed mode to offer.


## Library version is worth as much as library choice

The same `twmb/avro` code path, tag `v1.7.2` against main
(`v1.7.3-0.20260811190909`), everything else identical:

| | v1.7.2 | main | |
| --- | ---: | ---: | --- |
| DecodeDynamic flat | 495.6n | **371.5n** | −25% |
| DecodeDynamic nested | 1.663µ | **1.080µ** | −35% |
| DecodeDynamic union | 1.209µ | **983.2n** | −19% |
| Parse flat | 33.50µ | **14.01µ** | −58% |
| Parse flat allocs | 336 | **190** | −43% |

When that was measured twmb had not cut a tag since 2026-04-30, so anyone
benchmarking it from its latest release was measuring something materially
slower than its main. `v1.8.0` (2026-08-14) and `v1.9.0` (2026-09-06) have
since closed that gap. `redpanda-data/connect` pins a commit, not a tag.

hamba, iskorotkov and goavro reproduced within a few percent across those two
runs, so this is the library moving, not the harness.

## Dynamic decode — the mode a schema-driven pipeline runs in

`sec/op`:

| Schema | goavro | twmb | hamba | iskorotkov |
| --- | ---: | ---: | ---: | ---: |
| flat | **254.1n** | 371.5n | 827.8n | 842.7n |
| nested | **915.0n** | 1.080µ | 2.457µ | 2.479µ |
| union | 1.238µ | **983.2n** | 1.597µ | 1.611µ |
| wide | 3.625µ | **2.707µ** | 3.961µ | 4.035µ |

`allocs/op`:

| Schema | goavro | twmb | hamba | iskorotkov |
| --- | ---: | ---: | ---: | ---: |
| flat | **10** | 11 | 19 | 19 |
| nested | 35 | **29** | 60 | 60 |
| union | 49 | **22** | 34 | 34 |
| wide | 82 | **22** | 79 | 79 |

The split is by schema shape, not by library quality. goavro wins on plain
scalars and nested records. twmb wins as soon as unions appear, and the margin
grows with field count: on `wide` — 44 fields, mostly nullable unions, the shape
Confluent schema-registry traffic takes — twmb is 1.34× faster on **a quarter of
the allocations**.

goavro's cost there is structural. It wraps every non-null union branch in a
single-key map, so each such field costs an extra allocation.

hamba and iskorotkov are the slowest dynamic decoders in every case. Their
reputation comes from the typed path below.

## Typed decode

| Schema | hamba | iskorotkov | twmb |
| --- | ---: | ---: | ---: |
| flat | 123.9n | 112.8n | **112.0n** |
| nested | 340.5n | **309.8n** | 376.9n |
| union | 656.0n | **548.8n** | 670.5n |
| wide | 1.720µ | 890.5n | **823.3n** |

goavro cannot appear: it has no struct mode. A capability difference, reported
as one rather than folded into a ranking.

**The fork is not just hamba plus a CVE fix.** iskorotkov is 1.93× faster than
hamba on `wide` typed decode (890.5n against 1.720µ) at identical allocations,
and ahead on nested and union too. Whatever it has changed since forking is
worth more than the vulnerability patch it was made for.

## Typed versus dynamic is the bigger axis

Same library, same payload, `wide`:

| | hamba | twmb |
| --- | ---: | ---: |
| dynamic | 3.961µ | 2.707µ |
| typed | 1.720µ | 823.3n |
| ratio | 2.3× | **3.3×** |

Choosing a codec moves the number less than choosing whether the schema is known
at build time. A pipeline taking schemas at runtime sits on the slow side of that
gap whatever it depends on.

This is also how the widely-circulated comparison misleads: it measures hamba's
typed path against goavro's dynamic one and reports a single ranking.

## The reuse artefact

`DecodeTypedReused` keeps one destination across `b.N`, as
`nrwiersma/avro-benchmarks` does. `DecodeTyped` allocates per iteration.

| Case | reused | fresh | understated |
| --- | ---: | ---: | ---: |
| hamba wide | 1.300µ, **0 allocs** | 1.720µ, 21 allocs | 24% |
| twmb wide | 410.9n, **0 allocs** | 823.3n, 21 allocs | **50%** |
| hamba nested | 236.1n, **0 allocs** | 340.5n, 5 allocs | 31% |
| twmb nested | 211.6n, **0 allocs** | 376.9n, 5 allocs | 44% |

Reuse reports **zero allocations per decode**. The result allocation has not
gone anywhere; it has moved outside the measurement. On `wide` it halves twmb's
apparent cost.

## Schema compilation

| Schema | goavro | twmb | hamba |
| --- | ---: | ---: | ---: |
| flat | **12.23µ** | 14.01µ | 34.29µ |

goavro and twmb are close now; twmb's previous tag was 2.4× slower than twmb is today.
twmb allocates least: 190 against goavro's 271 and hamba's 621.

Parsing costs roughly 20–70× a single decode either way, so whether a pipeline
caches a codec per schema matters more than which codec it caches.

## Pins moved, 2026-09-17: a full recollection

`go.mod` moved twmb from `81123696` (2026-08-11) to `16bc6f5b` (v1.9.0 plus
30 commits, 2026-09-06), iskorotkov from `972eeffc` (2026-06-10) to `362766d4`
(v2.34.0 plus 6, 2026-08-19), the `go` directive from 1.26.5 to 1.27.1, and
with them `klauspost/compress` 1.18.4 → 1.19.2 and `mapstructure` 2.4.0 →
2.5.0. goavro's master had not moved. All five families were then recollected
on 2026-09-18, `-count=10`, one process each, 4,420 samples; raw data is in
`runs/2026-09-18/part1.txt` through `part5.txt`, and the figures below are
benchstat over that against the committed `part*.txt`.

The tables above were **not** replaced. The recollection was noisier than the
committed data in four families — mean interval width `Parse` ±10.5%,
`SREncode` ±8.7%, `DecodeStage` ±6.7%, `OCFWrite` ±6.2%, against ±2.4%, ±0.2%,
±0.1% and ±0.3% in the committed runs — so the `ns/op` claims here lean on the
tight rows and on isolated reruns; `allocs/op` and `B/op` are exact either
way.

Three things moved, and only one of them is an Avro library.

### twmb: schema compilation, and one allocation per decode

| Parse | committed | recollected | ns | allocs | B/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| flat | 14.01µ | **8.439µ** | −40% | 190 → **153** | −22% |
| nested | 24.54µ | **15.39µ** | −37% | 321 → **264** | −17% |
| union | 30.53µ | **23.60µ** | −23% | 454 → **377** | −30% |
| wide | 171.5µ | **124.3µ** | −28% | 2427 → **2058** | −34% |

twmb now compiles `flat` faster than goavro (8.439µ against 12.96µ); the
"Schema compilation" table above had goavro ahead. The `union` and `wide`
rows carry ±11-18% intervals, the others ±4-6%.

One allocation is gone on `flat` (11 → 10 dynamic) and `union` (22 → 21
dynamic, 10 → 9 typed) in every family that decodes — `DecodeDynamic`,
`DecodeTyped`, `SRDecode`, `EncodingDecode`, `PulsarPattern` — and
`OCFRead/union` typed drops 10% of its allocations. `nested` and `wide` are
unchanged. `ns/op` on decode is inside the noise.

### Go 1.27.1: `encoding/json` is now the v2 implementation

Go 1.27 enables `GOEXPERIMENT=jsonv2` by default. One arm, three toolchain
settings, `-count=3`, same pins:

| `SREncode/flat/twmb-bare` | ns/op | B/op | allocs |
| --- | ---: | ---: | ---: |
| Go 1.27.1, default | 1890 | 712 | 25 |
| Go 1.27.1, `GOEXPERIMENT=jsonv2` | 1937 | 712 | 25 |
| Go 1.27.1, `GOEXPERIMENT=nojsonv2` | **1432** | 864 | 29 |

`json.Unmarshal` into `any` is 32% slower and allocates less. That is not a
twmb result — `twmb-bare` is `json.Unmarshal` then `Encode` — and it reaches
every arm that goes through the standard library: hamba and iskorotkov
`SRDecode` (they `json.Marshal` the decoded tree because they have no textual
codec; `flat` 35 → 41 allocs at −20% B/op, `nested` 107 → 89 at −32%, and
+11% to +35% ns), `twmb-bare` `SREncode` (+25% to +46% ns, `flat` 29 → 25
allocs), all `SREncodeCoerced` arms, and goavro's std-JSON mode, which got
faster: `SREncode/wide/goavro-stdjson` 25.21µ → 22.41µ, 560 → 452 allocs,
−36% B/op. `twmb-wrapped` (`DecodeJSON`) and `goavro-plain`
(`NativeFromTextual`) parse their own JSON and did not move.

That shifts one ranking. `SREncode/union`: `goavro-plain` 3.168µ now edges
`twmb-wrapped` 3.422µ (±17%) and `twmb-bare` 3.701µ, where the committed
table had `twmb-bare` at 2.845µ in front. The end-to-end decode ranking
survives untouched: twmb leads `SRDecode` on every schema, by 1.37× (`flat`),
1.46× (`nested`), 1.18× (`union`) and 1.47× (`wide`) over goavro's faster
mode, and by 1.48× to 2.91× over hamba, whose gap widened because its arm
pays the `encoding/json` cost twice.

For Bento this outranks any Avro number in this section: `schema_registry_encode`
takes JSON in through `encoding/json`, so a Go 1.27 build changes that path's
cost before any codec swap does.

### Go 1.27.1 and `klauspost/compress`: OCF compression

`OCFWrite/wide/deflate`, all ±0-1%:

| | committed | recollected | ns | B/op |
| --- | ---: | ---: | ---: | ---: |
| hamba, typed | 1.408m | 1.835m | +30% | +31% |
| iskorotkov, typed | 1.497m | 1.894m | +27% | +31% |
| twmb, typed | 871.2µ | 904.8µ | +4% | +29% |
| goavro, dynamic | 2.507m | **2.369m** | −5% | +15% |

hamba, iskorotkov and goavro deflate through the standard library's
`compress/flate`; an isolated rerun of `OCFWrite/flat/deflate/typed/hamba`
under Go 1.26.5 and 1.27.1 with identical pins gives 8.2 MB against 10.8 MB
per op and +18% ns, so that is Go. twmb deflates through `klauspost/compress`
and moved 4%. The klauspost bump is visible elsewhere: twmb's snappy `B/op` on
`wide` fell from 1018 KiB to 592 KiB (−42%), and the same isolated rerun shows
no Go 1.26/1.27 difference for it.

### `mapstructure` 2.5.0: hamba parses with a third more allocations

hamba's schema parser is built on `go-viper/mapstructure`. `Parse` allocations
went from 621 to 829 on `flat` (+33%), 1020 → 1368 `nested`, 923 → 1207
`union`, 4092 → 5262 `wide`, at +8% B/op — identical under Go 1.26.5 and
1.27.1, so it is the dependency. iskorotkov's counts did not change. An
archived library cannot pin its way out of this; it is a cost of standing
still that the fork does not pay.

### Typed decode, recollected

| Schema | hamba | iskorotkov | twmb |
| --- | ---: | ---: | ---: |
| flat | 115.8n | **108.2n** | 123.6n |
| nested | 298.0n | **275.0n** | 376.1n |
| union | 625.1n | **511.8n** | 629.0n |
| wide | 1.631µ | **759.2n** | 791.1n |

iskorotkov leads every shape; the committed table had twmb ahead on `flat`
and `wide`. twmb's `flat` reads +10% here at ±3%, but an isolated `-count=3`
rerun gives 112.1n, the committed figure, so that row is run noise rather
than a regression. `wide` is real: 759.2n against 791.1n, both ±1%.

### twmb `AliasInput`

twmb added `AliasInput()` on 2026-08-18: decoded strings and byte slices
point into the payload rather than being copied. `DecodeDynamic` and
`DecodeTyped` carry it as a `twmb-alias` arm, `-count=10`:

| | twmb | twmb-alias | ns | B/op |
| --- | ---: | ---: | ---: | ---: |
| DecodeDynamic union | 884.5n, 1103 B | **822.4n**, 1008 B | −7% | −9% |
| DecodeDynamic wide | 2.619µ, 2707 B | **2.580µ**, 2641 B | −1.5% | −2% |
| DecodeTyped flat | 123.6n, 105 B, 2 allocs | **113.5n**, 80 B, **1 alloc** | −8% | −24% |
| DecodeTyped union | 629.0n, 687 B | **572.2n**, 592 B | −9% | −14% |

Smaller than it sounds, because twmb already packs every string in a decode
into one shared slab, so aliasing removes bytes, not allocations — one alloc
on typed `flat`, none elsewhere. It is a 7-9% option for a consumer that owns
each message buffer for the life of the decoded value, and unusable for one
that reuses buffers, which is why it is a separate arm.

## What decides it is not in this file

Two of the three replacement candidates cannot implement Bento's existing paths
at all: hamba and iskorotkov ship no Avro textual codec, which four call sites
need. See [CAPABILITIES.md](CAPABILITIES.md). No timing here outranks that.
