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

Generating from a CLDR the oracle was not built with turns upstream drift into
corpus differences that look like bugs. Both move together, deliberately, or
neither moves.

Note this differs from ICU4X's own pins (CLDR `49.0.0-ALPHA2`, icuexport
`icu4x/2026-08-31/79.x`). ICU4X is an architectural reference, not a version
authority.

## Pins

| Source | Pin | Feeds | Confirmed |
|---|---|---|---|
| ICU | 78.3 | the anchor; the golden corpus oracle | yes, from node |
| CLDR (`cldr-json`) | **48.2.0** | numbers, dates, units, names, plurals, zones | yes, by the corpus |
| Unicode (UCD) | 17.0.0 | normalization | yes, from node |
| IANA tzdb | 2026b | zone rules | yes, from node — but see below |
| `icuexportdata` | `icu4x-icuexportdata-78.3.zip` | UCA and tailorings (`collgen`); later properties, case, dictionaries | yes |
| ICU data sources | `icu4c-78.3-data.zip` | the collation tree, defaults and search jamo rules (`collgen`); numbering-system entries (`numbergen`); calendar glue and interval patterns (`dategen`); zone names, region names for zones, and the zone metadata (`zonegen`) | yes |
| LSTM models | `v0.1.0` | Thai, Khmer, Lao, Burmese word breaking | not version-tied to ICU |

CLDR is fetched **per component from npm**, not as the 79 MB `json-full.zip`
the release page offers. `cldr-core` is 200 KB and carries all of the
supplemental data, so a generator takes only the package it reads:

```sh
curl -sLO https://registry.npmjs.org/cldr-core/-/cldr-core-48.2.0.tgz
```

| Package | Version | sha256 | Used by |
|---|---|---|---|
| `cldr-core` | 48.2.0 | `5310e0c7a06c1feb83dc8e54c8584bbe0b9c2ea8320172a18d541986b124585d` | `localegen`, `numbergen`, `pluralgen` (plurals, ordinals and plural ranges, vendored), `dategen` (time data and week data, vendored) |
| `cldr-numbers-full` | 48.2.0 | `2d17a1453c559a62112caeed52e0bcfe3cb8539c99d239ae7b7ed4d0827679d9` | `numbergen` |
| `cldr-misc-full` | 48.2.0 | `c6ba8384d7ea8701cf86935db0461379231ddd970cc41c249f4a33b9857ded3c` | `listgen` |
| `cldr-units-full` | 48.2.0 | `754d55f183570c53029a77493302f432fb3e905df35a715f9cb022e2ebcb093c` | `unitgen` |
| `cldr-dates-full` | 48.2.0 | `0256f1cefeca14f7d515be4872dda48fcdd7e75381430f25aafd0143eae5b430` | `reltimegen`, `namegen`, `dategen` |
| `cldr-localenames-full` | 48.2.0 | `7f2ac7fd3b5d90f56ad127f9ed5ae67b58b3e556818f55530b9f47bb81bcde57` | `namegen` |
| `cldr-cal-buddhist-full` | 48.2.0 | `3429cb832bef99a978863f11a6afd8b36f5ce981ab18c51481648cef479400ff` | `dategen` |
| `cldr-cal-persian-full` | 48.2.0 | `937af05a2e84a3a38e9fd6880888c622ff14db6577fc254d71a91d2e0442f41f` | `dategen` |

Each calendar beyond the Gregorian one is its own CLDR package, so the
remaining fourteen arrive as fourteen more rows here rather than as a change
to anything.

**What is vendored and what is not.** A supplemental file of a few tens of
kilobytes is vendored beside the generator that reads it, so that generator
runs with nothing to fetch. A per-locale package is not: `cldr-numbers-full` is
37 MB unpacked, so `numbergen` takes the path to an unpacked copy and this file
is what pins which copy.

Other URLs:

- `https://github.com/unicode-org/cldr-json/releases/download/{tag}/cldr-{tag}-json-full.zip`
- `https://github.com/unicode-org/icu/releases/download/release-78.3/icu4x-icuexportdata-78.3.zip`
  — sha256 `eb63a12439f3fd9199886808275900229a5638fb0ee88d3c3c528eca7b811e60`, 5.6 MB
- `https://github.com/unicode-org/icu/releases/download/release-78.3/icu4c-78.3-data.zip`
  — sha256 `9d8b3899096aeb83e4e21ef8a40fec9e03b28db18c48452efac882ce25a91e27`, 20 MB.
  `collgen` checks both checksums before reading either archive.
- `https://github.com/unicode-org/icu/releases/download/release-78.3/icu4c-78.3-sources.tgz`
  — sha256 `3a2e7a47604ba702f345878308e6fefeca612ee895cf4a5f222e7955fabfe0c0`, 28 MB.
  Only `source/common/localefallback_data.h` is read, and it is vendored in
  `internal/icusrc`: ICU's tables of parent locales and default scripts,
  which ICU's resource fallback consults and which are not in the data
  archive.
- `https://www.unicode.org/Public/17.0.0/ucd/UnicodeData.txt` - sha256
  `2e1efc1dcb59c575...`, vendored in `internal/normgen`
- `https://www.unicode.org/Public/17.0.0/ucd/CompositionExclusions.txt` -
  vendored likewise
- `https://github.com/unicode-org/lstm_word_segmentation/releases`

## Which CLDR 48, settled

Node reports ICU 78.3's CLDR as "48.0", and that turned out to be a coarser
label than a version. The pin started at 48.0.0 on the strength of it, and
**the corpus moved it to 48.2.0**.

The evidence was one locale and its spaces. Traditional Chinese joins a date
to a time:

| | `zh-Hant` glue |
|---|---|
| CLDR 48.0.0 | `{1}{0}` - nothing between them |
| CLDR 48.2.0 | a plain space, except a thin space U+2009 after a short date |
| ICU 78.3 | the same as 48.2.0 |

Nothing in 48.0 could produce a space there. Regenerating every table at
48.2.0 took DateTimeFormat from 1,221 of 1,230 to **1,230 of 1,230** and moved
no other service by a single case, which is as clean a confirmation as this
offers.

An earlier version of this section said ICU writes a plain space where CLDR
has the thin one, and `dategen` flattened U+2009 accordingly. The corpus
cases it rested on all used a longer date, whose glue is a plain space in CLDR
already. The legacy `toLocaleString` cases use a short date, and ICU writes
the thin space there: `2024/1/5`, U+2009, `下午3:04:05`.

The version string was never the authority. The corpus was.

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

Two choices it seemed to force, both settled:

- **`implicithan` or `unihan`** for how Han characters order: **`unihan`**,
  by evidence. The two flavors differ in the root, and Node sorts U+3400
  before U+9FA0, which only radical-and-stroke order does; implicit order puts
  the URO block, U+9FA0 included, before Extension A. ICU4X ships
  `implicithan` because it is smaller; ICU4C does not.
- **`fast` or `small`** for norm, uprops and ucase: **not a choice after
  all** for anything built so far. The collation tries carry their own type,
  and the normalizer is generated from the Unicode Character Database. It will
  come back if the segmenter reads the export's property tables.

**What the export leaves out of collation**, and where it comes from instead:

- The combining diacritics U+0300-U+034E and the conjoining jamo are not in
  any trie. The root's are in `root_standard_dia` and `root_standard_jamo`,
  and a tailoring that changes the diacritics (Vietnamese, Ewe) has a `dia`
  file of its own.
- A tailoring's jamo are dropped altogether. For the search collations they
  are read from ICU's rule sources in the data archive; Korean `searchjl`
  cannot be, and is a recorded gap (PLAN.md, stage 7).
- The collation tree - aliases, parents, default types, the installed
  locales - is not in the export. It is in the data archive's `data/coll`:
  `LOCALE_DEPS.json` and each locale's `default`. It is not the ordinary
  tree: CLDR's `parentLocales` has a `collations` section of its own, and ICU
  folds that into these files.

**tzdb is 2026b in ICU 78.3, but go-quickjs bundles 2026c.** Its
`internal/icu/timezones.go` calls the bundle "the same version used by the
Node/ICU release", which is no longer accurate; `TestBundledTimeZoneMatchesICURelease`
does not actually check the version, only that every zone loads and one
Vancouver rule holds. Nothing currently fails because of it. go-intl should pin
deliberately rather than inherit the drift.

The zone data format matters as well as the version. The tz database gives
Ireland a negative daylight saving in winter; ICU builds `zoneinfo64` from
the rearguard form, where Irish summer is daylight time. go-intl reads Go's
zone data today and so disagrees with Node about Dublin's specific names, a
named gap in PLAN.md.

## Known defects in the data go-intl inherits

**Normalization is Unicode 13.0.0 while everything else is 17.0.** go-quickjs's
`internal/normalize/tables.go` says it was generated "from the Unicode 13.0.0
database, as shipped with the system's Perl" — an accidental input, four
versions behind the `\p{...}` property tables in the same binary, which are at
17.0.0.

**go-intl does not inherit it.** It carries its own normalizer, generated from
the Unicode Character Database at 17.0.0 by `internal/normgen`, which is the
primary source those tables are themselves built from. ICU exports its
normalizer in `icuexportdata` too, but in ICU4X's trie encoding, which would
have to be implemented to read; the database is line-based text and says the
same thing.

Every character up to U+2FFFF was put through all four forms and compared with
node: 779,392 normalizations, no differences.

**Vendored `windowsZones.json` was taken from `cldr-json` `main`, not a tagged
release.** Its content self-reports CLDR 48, which is correct for the anchor,
but the provenance is unpinned. Re-fetch it from the CLDR 48.0 tag when that
generator is brought over.

## Where cldr-json is not all of CLDR

CLDR's root uses locale-relative aliases: this value is whatever the *asking*
locale has at that other path. cldr-json resolves such an alias as if it
pointed at the root's own value, or leaves the path out. ICU's resource
bundle sources keep the aliases, and ICU resolves them as CLDR means. Every
place go-intl has met this is read from those sources, from the pinned data
archive, through `internal/icusrc`:

| What | cldr-json | ICU, and so go-intl |
|---|---|---|
| a numbering system a locale does not use | missing | the locale's own few fields, then the root's entry, then the locale's Latin data |
| a calendar's date-time atTime glue | the root's for other calendars; the locale's own plain glue where it overrides only that | the first bundle up the chain that has one, else the Gregorian one |
| zone names | each locale resolved by CLDR's inheritance | merged field by field along ICU's chain; a locale with no zone bundle falls back by ICU's resource rules and ICU's own default scripts, so `sr-Cyrl-ME` reads the Latin `sr_Latn_ME` |

## The space before AM and PM

CLDR separates a time from its day period with U+202F, a narrow no-break
space, in 440 locales, and ICU writes it. **Node writes a plain space**, and
the golden corpus has plain spaces, but that is V8's doing and not ICU's: V8
replaces U+202F in everything it formats (`Replace202F` in
`js-date-time-format.cc`), reverting ICU 72's change for the web's sake.

So the date tables keep CLDR's character, and the NodeICU profile makes V8's
replacement, as a named divergence (`compat.go`). An earlier version of
`dategen` replaced the character in the data itself, on the belief that ICU
did; reading V8 showed otherwise.

CLDR offers "-alt-ascii" patterns that look like they say the same thing and do
not: `en-GB`'s medium time is `HH:mm:ss` and its ascii alternate is
`h:mm:ss a`, which is a different clock rather than a different space.
Preferring those alternates put 75 locales on the wrong clock before the corpus
caught it. Only the character is taken.
