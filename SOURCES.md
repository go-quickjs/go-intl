# Upstream sources

Every input go-intl generates from, pinned. Changing any pin is its own commit
with the gates re-measured, never folded into a behavior change.

## The anchor: ICU 78.3

go-intl targets **ICU 78.3**, because that is the ICU that produced the golden
corpus go-intl is held against. Reported by the oracle itself:

```console
$ node -e "console.log(process.versions.icu, process.versions.unicode, process.versions.cldr, process.versions.tz)"
78.3 17.0 48.0 2026b
```

Generating from a newer CLDR than the oracle was built with makes some corpus
differences genuine upstream drift rather than bugs, which is time spent
chasing nothing. Both move together, deliberately, or neither moves.

Note this differs from ICU4X's own pins (CLDR `49.0.0-ALPHA2`, icuexport
`icu4x/2026-08-31/79.x`). ICU4X is an architectural reference, not a version
authority.

## Pins

| Source | Pin | Feeds | Confirmed |
|---|---|---|---|
| ICU | 78.3 | the anchor; the golden corpus oracle | yes, from node |
| CLDR (`cldr-json`) | 48.0 | numbers, dates, units, names, plurals, zones | yes, from node |
| Unicode (UCD) | 17.0 | properties, normalization | yes, from node |
| IANA tzdb | 2026b | zone rules | yes, from node — but see below |
| `icuexportdata` | `icu4x-icuexportdata-78.3.zip` | UCA and tailorings, normalizer, properties, case, dictionaries | yes |
| LSTM models | `v0.1.0` | Thai, Khmer, Lao, Burmese word breaking | not version-tied to ICU |

CLDR is fetched **per component from npm**, not as the 79 MB `json-full.zip`
the release page offers. `cldr-core` is 205 KB and carries all of the
supplemental data, so a generator takes only the package it reads:

```sh
curl -sLO https://registry.npmjs.org/cldr-core/-/cldr-core-48.0.0.tgz
```

| Package | Version | sha256 | Used by |
|---|---|---|---|
| `cldr-core` | 48.0.0 | `3b739a175e47e50905050a34612584fd5590c9352b11f36e3498eacff1c9aa94` | `internal/localegen`, `internal/numbergen` |
| `cldr-numbers-full` | 48.0.0 | `d3d12515b0f6f7c4b5f82586b52b16164a0cffbf4d45a9f2bd894380a96b1cb3` | `internal/numbergen` |
| `cldr-misc-full` | 48.0.0 | `c72d7aef0022206f0c65bbad9fb3d5524078c89066febb397d94cca9e2983176` | `internal/listgen` |
| `cldr-units-full` | 48.0.0 | `702cec5d8caa9c1c323188eab426262003f45d1a58fc7ad073be4c7128ee8137` | `internal/unitgen` |

**What is vendored and what is not.** A supplemental file of a few tens of
kilobytes is vendored beside the generator that reads it, so that generator
runs with nothing to fetch. A per-locale package is not: `cldr-numbers-full` is
37 MB unpacked, so `numbergen` takes the path to an unpacked copy and this file
is what pins which copy.

Other URLs:

- `https://github.com/unicode-org/cldr-json/releases/download/{tag}/cldr-{tag}-json-full.zip`
- `https://github.com/unicode-org/icu/releases/download/release-78.3/icu4x-icuexportdata-78.3.zip`
  — sha256 `eb63a12439f3fd9199886808275900229a5638fb0ee88d3c3c528eca7b811e60`, 5.6 MB
- `https://github.com/unicode-org/lstm_word_segmentation/releases`

## Which CLDR 48

CLDR ships 48.0.0, 48.1.0 and 48.2.x. Node reports ICU 78.3's CLDR as "48.0",
so that is the pin. The distinction has not mattered yet: 48.0.0 and 48.2.0
carry **identical** likely subtags, 7,788 entries with no differences between
them. If it ever does matter the corpus decides, since the corpus is the ground
truth and the version string is only a label.

One thing that looks like a contradiction and is not: CLDR 48 reports
`_unicodeVersion: "16.0.0"` while ICU 78.3 reports Unicode 17.0. Those describe
different things — CLDR's is the Unicode release its own data was built
against, ICU's is the Unicode release ICU integrated. The authority for
character properties is `uprops` in the 78.3 export, not CLDR's metadata.

## The 78.3 export

ICU publishes most `icuexportdata` artifacts under `icu4x/{date}/{major}.x`
tags, and **there is no 78.x among them** — they run 71, 72, 73, 75, 76, 77, 79.
Looking only there would have concluded the anchor could not be matched.

It is attached to the ICU release itself instead: `release-78.3` carries
`icu4x-icuexportdata-78.3.zip`. So the export is available at exactly the
version that produced the golden corpus, and no compromise between anchor and
export is needed.

Contents, 931 entries:

| Directory | Holds |
|---|---|
| `collation/` | 342 files each under `implicithan/` and `unihan/` |
| `norm/` | `nfd`, `nfkd`, `compositions`, `decompositionex`, `uts46d`, in `fast/` and `small/` |
| `uprops/` | 108 property files, in `fast/` and `small/` |
| `ucase/` | case mapping, in `fast/` and `small/` |
| `segmenter/dictionary/` | `cjdict`, `thaidict`, `khmerdict`, `laodict`, `burmesedict` |

The tailorings go-quickjs carries are all present: `zh_pinyin`, `zh_stroke`,
`zh_unihan`, `ja_standard`, `ja_unihan`, `ko_standard`, `ko_search`,
`de_phonebook`, `es_traditional`, plus `root_standard`, `root_emoji`,
`root_eor` and `root_search`.

**One pinned 5.6 MB artifact replaces three separate ad-hoc inputs**: the
node-scraped collation data, the Perl-derived normalizer tables, and the break
dictionaries currently vendored as loose `.txt` copies from the ICU source tree.

Two choices it forces, both to settle in stage 2:

- **`fast` or `small`** for norm, uprops and ucase — ICU4X's table-size against
  lookup-speed tradeoff.
- **`implicithan` or `unihan`** for how Han characters order. ICU's default is
  implicit computation, with `zh_unihan` and `ja_unihan` offered separately.

**tzdb is 2026b in ICU 78.3, but go-quickjs bundles 2026c.** Its
`internal/icu/timezones.go` calls the bundle "the same version used by the
Node/ICU release", which is no longer accurate; `TestBundledTimeZoneMatchesICURelease`
does not actually check the version, only that every zone loads and one
Vancouver rule holds. Nothing currently fails because of it. go-intl should pin
deliberately rather than inherit the drift.

## Known defects in the data go-intl inherits

**Normalization is Unicode 13.0.0 while everything else is 17.0.** go-quickjs's
`internal/normalize/tables.go` says it was generated "from the Unicode 13.0.0
database, as shipped with the system's Perl" — an accidental input, four
versions behind the `\p{...}` property tables in the same binary, which are at
17.0.0.

This is not cosmetic for go-intl: **UCA requires NFD**, so the Collator sits
directly on this table. Every character added or recomposed between Unicode 13
and 17 is a latent sorting divergence that would surface late, inside the
hardest stage. Replace the normalizer's data source before stage 7 starts.

The replacement is already in hand: `norm/` in the 78.3 export carries `nfd`,
`nfkd`, `compositions` and `decompositionex` at the anchor's Unicode 17.0. So
this is a datagen task in stage 2, not research.

**Vendored `windowsZones.json` was taken from `cldr-json` `main`, not a tagged
release.** Its content self-reports CLDR 48, which is correct for the anchor,
but the provenance is unpinned. Re-fetch it from the CLDR 48.0 tag when that
generator is brought over.
