# Results

> **Superseded in part.** Everything below measures one stage — binary to an
> untyped value. Bento's actual path is two stages, binary to native to JSON
> bytes, and measuring the whole path reverses the ranking. See
> [CAPABILITIES.md](CAPABILITIES.md) for what each codec can and cannot do,
> which decides more than any timing here. Full rewrite pending the current run.

`go1.26.5 darwin/arm64`, Apple M1, `-count=10`, `benchstat -col /lib`.
Regenerate with `make bench && make stat`.

## The headline: for dynamic decoding, goavro is not the slow one

`sec/op`, decoding into an untyped destination — the mode a schema-driven
pipeline runs in.

| Schema | goavro | twmb | hamba | iskorotkov |
| --- | ---: | ---: | ---: | ---: |
| flat | **265.2n** | 495.6n | 807.4n | 807.1n |
| nested | **940.5n** | 1.663µ | 2.459µ | 2.445µ |
| union | 1.259µ | **1.209µ** | 1.602µ | 1.588µ |

goavro is 3.0× faster than hamba on `flat` and 2.6× faster on `nested`, and
loses to twmb by 4% on `union`. The circulating claim that hamba beats goavro by
3.6× **inverts** once both are asked to produce the same output shape.

Allocation follows the same order except on unions:

| Schema | goavro | twmb | hamba |
| --- | ---: | ---: | ---: |
| flat | **10** allocs / 448 B | 11 / 465 B | 19 / 585 B |
| nested | 35 / 1.648 Ki | **32** / 1.640 Ki | 60 / 1.991 Ki |
| union | 49 / 2.352 Ki | **24** / 1.111 Ki | 34 / 1.241 Ki |

## Where goavro genuinely loses: unions

`2.352 KiB / 49 allocs` against twmb's `1.111 KiB / 24 allocs` — roughly double
the memory. That is the single-key wrapper map goavro builds per non-null union
field (`{"string": "u-99213"}`), allocated per field per message.

Confluent schema-registry traffic is union-heavy, so this is the one place a
migration pays. It is also exactly the place where the change is visible to
users, since that wrapper is the shape Bloblang mappings are written against.

## The axis that actually matters is typed versus dynamic

Same library, same payload, `nested`:

| | hamba | twmb |
| --- | ---: | ---: |
| dynamic | 2.459µ | 1.663µ |
| typed | 333.6n | 350.1n |
| ratio | **7.4×** | **4.7×** |

Choosing a codec moves the number far less than choosing whether the schema is
known at build time. Bento cannot make that choice — schemas arrive at runtime
from a registry or config — so it is stuck on the slow side of a 5–7× gap no
library shopping will close.

This is also why the published comparison misleads: it measures hamba's typed
path against goavro's dynamic one and reports a single ranking.

## The reuse artefact, quantified

`DecodeTypedReused` keeps one destination across `b.N`, as
`nrwiersma/avro-benchmarks` does. `DecodeTyped` allocates per iteration.

| Case | reused | fresh | understated by |
| --- | ---: | ---: | ---: |
| hamba nested | 222.6n, **0 allocs** | 333.6n, 5 allocs | 33% |
| hamba union | 444.4n, 3 allocs | 644.1n, 11 allocs | 31% |
| twmb nested | 179.2n, **0 allocs** | 350.1n, 5 allocs | 49% |

Reuse reports **zero allocations per decode** for `flat` and `nested`. The
result allocation does not disappear in production; it is only moved outside the
measurement.

## Schema compilation

`Parse`, per call. Relevant only to pipelines that do not cache a codec per
schema.

| Schema | goavro | hamba | twmb |
| --- | ---: | ---: | ---: |
| flat | **11.79µ** | 32.22µ | 33.50µ |
| nested | **20.97µ** | 53.54µ | 75.80µ |
| union | **21.94µ** | 47.37µ | 71.33µ |

goavro compiles 2–3× faster. Parsing costs 20–70× a single decode, so a codec
cache matters more than any decode difference measured here.

## iskorotkov versus hamba

Indistinguishable, as a fork should be: within noise everywhere except a small
typed-decode edge on `nested` (304.8n against 333.6n). The GO-2026-5048 fix
costs nothing.

## What this means for Bento

The performance case for moving Bento's Avro components off goavro is **weak**.
Bento decodes dynamically, and goavro wins or ties there on everything except
union memory.

The real arguments for `twmb/avro` are elsewhere: roughly half the memory on
union-heavy payloads, one maintained library instead of one maintained and one
archived, and alignment with `redpanda-data/connect`, which made this exact
migration in [#4195](https://github.com/redpanda-data/connect/pull/4195) and
deleted 865 lines of normalisation code doing so.

Set against that: the union representation change breaks user Bloblang mappings,
and connect's PR documents further breaks in decimal and fixed handling. That is
a behaviour and maintenance decision. It should not be argued on speed, because
on Bento's access pattern the speed argument runs the other way.
