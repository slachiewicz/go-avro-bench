# Results

`go1.26.5 darwin/arm64`, Apple M1, `-count=10`, `benchstat -col /lib`.
Regenerate with `make bench && make stat`.

Library versions are branch tips, not tags — which turns out to matter, see
below. hamba is pinned at v2.31.0 because that is its final release.

> **Partial.** The end-to-end pipeline, encoding and OCF families are still
> running and are not reported yet. A single `-count=10` sweep of the whole
> suite was killed by the machine partway through `EncodingDecode`, so the
> families below come from that sweep's completed portion and the rest are being
> collected in separate chunks. Nothing is mixed across code changes: the tree
> has not moved since.

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

## Still to report

End-to-end `SRDecode`/`SREncode` through both goavro codec modes, the three wire
encodings, OCF containers, and the Pulsar cached-codec pattern — the families
that measure real pipeline paths rather than a single decode.

The capability findings in [CAPABILITIES.md](CAPABILITIES.md) already decide
more than any of these timings: two of the three replacement candidates cannot
implement Bento's existing paths at all.
