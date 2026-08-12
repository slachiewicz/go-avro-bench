# Results

`go1.26.5 darwin/arm64`, Apple M1, `-count=10`, `benchstat -col /lib`.
Regenerate with `make bench && make stat`.

Library versions are branch tips, not tags — which turns out to matter, see
below. hamba is pinned at v2.31.0 because that is its final release.

Collected in five processes rather than one sweep, all from the same tree.
434 benchmark arms, 4,340 timed runs, roughly 2.7 billion iterations.

## The path Bento actually runs

`schema_registry_decode` is two stages, not one: `NativeFromBinary` then
`TextualFromNative` (`serde_avro.go:198-203`). Measuring only the first, as the
tables further down do, gets the ranking wrong — the second stage is the larger
part of the cost.

`DecodeStage`, goavro, `flat`: native **334.9n**, textual **1.148µ**. The stage
this file used to omit is 77% of the work.

### End to end, Confluent-framed binary in, JSON bytes out

| Schema | twmb | goavro-plain | goavro-stdjson | hamba | iskorotkov |
| --- | ---: | ---: | ---: | ---: | ---: |
| flat | **1.056µ** | 1.433µ | 1.494µ | 2.244µ | 2.322µ |
| nested | **3.124µ** | 4.760µ | 4.766µ | 6.539µ | 6.549µ |
| union | **2.940µ** ±19% | 3.800µ | 3.497µ | 3.865µ | 3.832µ |
| wide | **6.582µ** | 11.02µ | 10.07µ | 11.27µ | 11.35µ |

twmb wins every case, by 1.36× to 1.71× over the better goavro mode. On
allocations the gap is wider: `wide` costs twmb **50** against goavro's 137-158
and hamba's 169.

That reverses what the single-stage numbers say, where goavro leads on `flat`
and `nested`. Both are correct measurements of different things; only one of
them is the thing a pipeline pays.

### Encoding, JSON bytes in, framed binary out

| Schema | twmb-bare | twmb-wrapped | goavro-plain | goavro-stdjson |
| --- | ---: | ---: | ---: | ---: |
| flat | 1.677µ | 1.512µ | 1.385µ | **1.372µ** |
| nested | 4.954µ | **4.391µ** | 4.450µ | 4.568µ |
| union | **3.322µ** | 4.020µ | 3.864µ | 5.640µ |
| wide | **10.04µ** | 15.88µ | 13.53µ | **31.37µ** |

`bare` and `wrapped` are the two JSON dialects: bare values against goavro's
tagged-union envelope. Each is paired with the goavro mode that speaks the same
dialect.

goavro's std-JSON mode costs **31.37µ on `wide`, 2.3× its own plain mode**
(±4% against ±1%, so not noise). Union-heavy records are exactly where that mode
gets chosen.

### What the coercion walker costs

hamba and iskorotkov cannot encode a `json.Unmarshal` value at all, so those
arms are absent above. With the 220-line walker from
[CAPABILITIES.md](CAPABILITIES.md) in front of them:

| Schema | hamba | iskorotkov | twmb-bare (no walker) |
| --- | ---: | ---: | ---: |
| nested | 9.105µ | 8.373µ | **4.905µ** |
| union | 5.281µ | 5.258µ | **3.580µ** |
| wide | 16.60µ | 16.54µ | **9.734µ** |

Shimmed, they lose to unshimmed twmb by 1.7-1.9×. The walker does not buy back
the gap; it only makes the comparison possible.

## Object container files

The one family where all four libraries support everything — null, deflate and
snappy, no skips. `OCFWrite`, 1000 records, deflate:

| | hamba | iskorotkov | twmb | goavro |
| --- | ---: | ---: | ---: | ---: |
| flat, typed | 756.1µ | 767.1µ | **234.4µ** | n/a |
| flat, dynamic | 1.349m | 1.329m | 357.3µ | **303.4µ** |
| nested, typed | 1.006m | 1.011m | **478.9µ** | n/a |
| nested, dynamic | 2.748m | 2.884m | 792.7µ | **640.3µ** |

twmb is 3.2× faster than hamba writing typed, and hamba is 3.8× behind twmb
writing dynamic. goavro leads the dynamic column, having no typed mode to offer.


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

twmb has not cut a tag since 2026-04-30 while developing steadily, so anyone
benchmarking it from its latest release is measuring something materially slower
than its main. `redpanda-data/connect` pins an untagged commit.

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

goavro and twmb are close now — on its previous tag twmb was 2.4× behind here.
twmb allocates least: 190 against goavro's 271 and hamba's 621.

Parsing costs roughly 20–70× a single decode either way, so whether a pipeline
caches a codec per schema matters more than which codec it caches.

## What decides it is not in this file

Two of the three replacement candidates cannot implement Bento's existing paths
at all: hamba and iskorotkov ship no Avro textual codec, which four call sites
need. See [CAPABILITIES.md](CAPABILITIES.md). No timing here outranks that.
