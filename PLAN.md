# Plan

Building `github.com/go-quickjs/go-intl` and replacing go-quickjs's
`internal/icu` with it. Read [DESIGN.md](DESIGN.md) first; this file is the
staging and the running state.

## The contract

Three numbers, measured 2026-09-23 against test262 `045bf6f9` on Windows 10
(zone `America/New_York`), with go-quickjs at `4977606`:

| Gate | Value |
|---|---|
| test262 full | 98,560 variants, **93,010 pass, 0 fail**, 5,550 skip |
| test262 intl402 | 6,726 variants, **6,704 pass, 0 fail**, 22 skip |
| Golden vs ICU | **7,948 / 7,949** |
| `go test ./...` in go-quickjs | green |

Every one of these is a floor. Nothing lands that lowers any of them.

Reproduce with:

```sh
cd ../go-quickjs
TEST262_DIR=/d/Data/test262 go test ./conformance -run TestConformance -conformance.max-failures=0 -timeout=90m
TEST262_DIR=/d/Data/test262 go test ./conformance -run TestConformance -conformance.dir=intl402 -conformance.max-failures=0
go test ./...
```

The timezone fix that took intl402 to zero failures is Windows-specific, so the
numbers differ by platform. **Run the gate on Linux and Windows both** - that
bug was invisible on one of them.

## The strangler contract

This is what makes "no regression" a process guarantee rather than a hope:

1. **go-quickjs is not touched until a service is proven.** `internal/icu` keeps
   serving every service while go-intl is built beside it.
2. go-intl develops against the golden corpus on its own. It is free to be
   incomplete or behind; nothing depends on it yet.
3. A service switches over in go-quickjs only when **both** hold:
   - its slice of the corpus is at or above what `internal/icu` scores, and
   - **it implements every option `internal/icu` implements**, checked against
     the service's test262 intl402 files rather than against the corpus.
4. If a switch regresses anything, it does not land. The old path stays.
5. **No switch-over happens until every service is implemented and the
   maintainer has said to go ahead.** Meeting the gates makes a service
   *eligible*, not scheduled. Do not wire go-quickjs to go-intl, add a
   `replace` directive, or touch `internal/icu` before that word is given.

**Corpus parity is necessary and nowhere near sufficient**, and the second
condition above exists because the first alone was wrong. The corpus has 2,970
NumberFormat cases in **thirteen** option combinations; test262 has 249
NumberFormat files and exercises `unit` style, scientific and engineering
notation, significant digits, nine rounding modes, rounding increments,
`trailingZeroDisplay`, accounting currency sign and currency names - none of
which the corpus touches and all of which go-quickjs passes today. A gate that
looked only at corpus output would have approved a switch that broke several
hundred conformance tests.

The lesson generalizes: **the corpus measures whether the output is right, not
whether the surface is complete.** Both have to be gated, and only one of them
is what the corpus is for.

Expect go-intl's own numbers to dip while a service is being built from CLDR
rather than from scraped answers - divergences that are invisible today, because
the current data *is* ICU's output, become visible once the rules are
implemented. That dip happens inside go-intl where it costs nothing, never in
go-quickjs. This is the reason for rule 1.

Consequence: go-quickjs will depend on both go-intl and `internal/icu` for a
while. That is intended, not a transitional embarrassment.

## Bootstrap decision

The first vertical slice is built **from CLDR, not by wrapping the existing
tables.** Wrapping would be faster to a green test and would quietly shape the
new API around data that is a record of answers - the exact contamination this
project exists to remove. The strangler contract removes the reason to hurry.

## Version anchor

go-intl targets **ICU 78.3** - CLDR 48.2.0, Unicode 17.0 - because that is what
produced the golden corpus. The CLDR pin began at 48.0.0, on node's own report,
and the corpus moved it: see SOURCES.md. Generating from a newer CLDR than the oracle was
built with turns upstream drift into corpus differences that look like bugs.
Both move together or neither moves. [SOURCES.md](SOURCES.md) holds every pin,
what is still unverified, and the defects go-intl inherits.

## Stages

Each stage is one shippable increment with a gate. Stages 5 and 6 are more than
one session; the rest are roughly one each.

### 0. Bootstrap

Repo, `go.mod` (module `github.com/go-quickjs/go-intl`, Go 1.24 to match
go-quickjs), `AGENTS.md`, this file, `DESIGN.md`, [SOURCES.md](SOURCES.md).
Import the golden corpus as a Go test fixture and write the replay harness that
will run it once an API exists.

Two de-risking checks belonged here rather than in the stage that needs them.
**Both are now answered** — see SOURCES.md:

- An `icuexportdata` artifact for ICU 78.3 exists. It is not under the
  `icu4x/{date}/{major}.x` tags, which skip 78 entirely; it is attached to the
  `release-78.3` ICU release. Exact alignment with the anchor, no compromise.
- It carries every tailoring `internal/icu` has, and also supplies the
  normalizer and the break dictionaries.

*Gate:* `go test ./...` runs, the corpus is parsed and counted, and SOURCES.md
has no unverified pins left. **Met.**

What the corpus turned out to be, which is design input for every later stage:
all 7,949 cases are written in **fifteen** expression shapes, and because the
generator spells every value with `JSON.stringify`, every literal inside them is
JSON. So `internal/corpus` takes a line apart into a service, a locale, an
option bag and arguments without needing a JavaScript engine.

The coverage is lopsided and worth knowing before choosing what to build:

| Service | Cases |
|---|---|
| NumberFormat | 2,970 |
| Collator | 1,805 |
| RelativeTimeFormat | 1,260 |
| DateTimeFormat | 1,230 |
| PluralRules | 300 |
| Segmenter | 140 |
| ListFormat | 120 |
| DisplayNames | 34 |
| legacy `toLocale*` | 90 |

The counts are asserted, so a corpus regenerated against a different ICU, or one
that grows a shape the parser cannot read, fails here rather than quietly
dropping a service's coverage.

### 1. Locale

`Language`, `Script`, `Region`, `Variant` as fixed-size comparable types.
Parser, canonicalization, and the fallback chain.

*Gate:* parse, canonicalize and fallback tests pass, and every locale the
corpus uses parses to itself. **Met.**

The split that matters, taken from ICU4X: **`Locale` is what was asked for,
`DataLocale` is what a table is stored under.** `de-CH-u-ca-buddhist` loads
`de-CH` and then formats with a Buddhist calendar - the extension chooses
behavior, not a table. Conflating the two is how a locale ends up meaning "the
record loaded for a locale", which is what it means in the package this
replaces. `DataLocale` holds no slices, so it is comparable and works as a map
key, which is the point of the fixed-size subtags.

**Deferred to stage 2, because both need data, not code:** CLDR's
`parentLocales` (so `zh-Hant` does not fall back through `zh`) and likely
subtags (so `zh-TW` and `zh-Hant-TW` look in one place). Until then the chain
is truncation inheritance as UTS #35 defines it on the identifier alone, which
is right for the common shapes and wrong only where CLDR says so.

### 2. Provider seam and datagen skeleton

The `Source` interface plus the embedded implementation. The datagen command
that reads `cldr-json` and writes the Model encoding. This is the highest-
leverage stage in the project: it is what makes the library reusable.

*Gate:* round-trip test - datagen writes, `Source` reads, the bytes agree.
**Met.**

`Source` hands back bytes, not decoded values, so a source can be a directory,
an archive or a cache without knowing what any of it means. One reader,
`NewFS`, serves both the embedded tables and any `io/fs`, which is what makes
"the data can be replaced" a real claim rather than a documented intention -
there is a test that builds a fallbacker over a made-up `fstest.MapFS`.

A table is records of two data locales, sorted, binary-searched where it lies
with nothing decoded until something is looked up. The generator marshals
through the same `DataLocale` codec the reader uses, so the two cannot drift,
and its output is byte-identical across runs.

This also closed stage 1's deferrals. `Fallbacker.Chain` follows CLDR's parents
- `zh-Hant` to the root rather than through `zh`, `en-AU` through `en-001`,
`es-AR` through `es-419` - and `Maximize` and `Minimize` implement UTS #35's
likely subtags.

### 3. NumberFormat

The first full vertical slice: datagen for `cldr-numbers-full`, the algorithm,
`FormatToParts`, and both `Standard` and `NodeICU` profiles. `cldr-numbers-full`
is clean self-contained JSON, which is why this service goes first.

*Gate:* the number slice of the golden corpus at or above `internal/icu`.
**This stage is the architecture proof.** If the layering does not hold here it
will not hold anywhere, and it is cheap to change now.

**Met, at 2,640 of 2,640 - 100% of the corpus's non-compact NumberFormat
cases.** The first run was 96.67%, and every one of the 88 failures was the
same missing rule, CLDR's currency spacing.

Data for **766 locales**, everything CLDR ships, 3.4 MB in all and about a
kilobyte each. Because a locale is its own file behind `Source`, a caller who
needs one reads a kilobyte rather than the lot. That is the fourteen-megabyte
problem answered in practice rather than in principle.

Three things the corpus settled that guesswork would have got wrong:

- **Rounding is half away from zero**, not Go's half to even. `strconv` gives
  "0" for 0.5 and "1234" for 1234.5 where ECMA-402 wants "1" and "1235", so the
  rounding is written out rather than left to the standard library.
- **Which decimal a float stands for.** ECMA-402 describes rounding the exact
  binary value; ICU rounds the shortest decimal that reads back as the same
  float. They differ for values like 0.615. This follows ICU, because ICU is
  what the corpus holds it to.
- **Currency spacing is a rule, not a special case.** "USD1.00" needs a space
  and "$1.00" does not, and CLDR says which by matching character classes. The
  set expressions are stored and compiled, not hardcoded - and the compiler
  refuses an expression it does not understand rather than quietly matching
  nothing, which would misplace spaces in some locales and nowhere else.

Not yet, and each says so rather than formatting something plausible: compact
notation and currency names both need plural rules, and scientific and
engineering notation are unwritten.

### 4. PluralRules and ListFormat

Small, data-clean, and the two services ICU4X's own ECMA-402 layer bothered to
bind. Confirms the shape repeats.

*Gate:* their corpus slices; first go-quickjs switch-over for all three services
so far. **Corpus met: PluralRules 300/300, ListFormat 120/120.**

Plural rules are a rule language, small but real: a condition over UTS #35's
operands, parsed when the rules are built rather than when a number is
selected. What is stored is CLDR's condition text. Which category a number
falls in is an answer, and no table holds one.

The operands come from the number **as it would be written**, not from its
value: 1 and 1.0 are the same quantity and different plurals, because one has a
written decimal. That is why selecting formats first.

One thing the corpus caught: the modulus keeps the fraction. UTS #35 has
`1.5 mod 10` as 1.5, not 1, which is what makes the English ordinal rule
`n % 10 = 1` hold for 1 and 21 but not for 1.5. Truncating first made 1.5 an
ordinal "one", which is not a thing.

### 3b. Compact notation

**Done, and NumberFormat is now 2,970/2,970 - the whole service.**

Two things made it more than dividing and appending a letter. The pattern is
chosen by the plural category of the *divided* amount, which is why it waited
on this stage. And the rounding is ECMA-402's "more precision", a choice
between two roundings rather than a mode: a compact number is rounded both to
no decimals and to two significant digits, and whichever keeps more wins, so
1.2345 thousand is "1.2K" but 123.456 billion is "123B".

The corpus caught the last 28 cases as one rule: compact notation groups only
when the leading group has two digits of its own, which ECMA-402 calls "min2",
so ja writes "1235万" rather than "1,235万".

### 3c. The rest of NumberFormat's surface

The corpus does not reach these and test262 does, so they block the switch-over
rather than the corpus gate:

- ~~significant digits and `roundingPriority`~~ **done**
- ~~`roundingMode`, all nine~~ **done**
- ~~`roundingIncrement` and `trailingZeroDisplay`~~ **done**
- ~~`currencySign: "accounting"`~~ **done**
- ~~`notation: "scientific"` and `"engineering"`~~ **done**
- ~~`style: "unit"`~~ **done**
- ~~`currencyDisplay: "name"`~~ **done**

- ~~the digit options `PluralRules` is also given~~ **done**; `selectRange` is
  still missing.

The rounding was rewritten to do its work on the digits rather than on the
float. Scaling a float by a power of ten to round it introduces error of its
own, and that error lands exactly on the ties the mode exists to decide.

Every expectation in the rounding tests was checked against node rather than
against a reading of the specification, and all of them agreed first time:
nine rounding modes, significant digits, both priorities, `trailingZeroDisplay`,
`roundingIncrement` and the accounting sign.

Only the 45 units ECMA-402 sanctions are carried. CLDR has 268, and the list
exists because every language has a name for each of the 45.

Five pairs -- kilometre per hour among them -- have a wording of their own
rather than one composed from the parts, and ICU prefers it. English composes
to the same string either way, so testing one locale would have passed while
the rule was wrong; Chinese is where it shows, composing to "987公里/小时"
where ICU answers "987 km/h".

*Gate:* every option `internal/icu` accepts is accepted here, and the corpus
stays at 2,970/2,970. **Met**, apart from `PluralRules.selectRange`, which
`internal/icu` does not implement either.

### 5. DateTimeFormat (multi-session)

The large one. Calendars and calendrical arithmetic, patterns, then skeleton
resolution and time zones. ICU4X's `calendrical_calculations` is the one crate
worth reading closely - self-contained, no zerovec entanglement - as a
cross-check on Hebrew, Persian and Islamic arithmetic.

*Gate:* the date slice of the corpus, with both profiles; the Temporal-facing
surface go-quickjs needs.

### 6. RelativeTimeFormat, DisplayNames, DurationFormat (multi-session)

Straightforward once the pattern layer from stage 5 exists. DisplayNames is the
one that justifies the split-data design: it is most of the 14 MB.

### 7. Collator

Needs `icuexportdata` for UCA and tailorings, plus normalization. Where ICU4X
did its hardest work.

**Prerequisite: done.** go-intl carries its own normalizer, generated from the
Unicode Character Database at 17.0.0, rather than inheriting go-quickjs's
Unicode 13 tables. UCA runs on NFD, so the collator sits directly on it, and
every character up to U+2FFFF was checked against node in all four forms:
779,392 normalizations, no differences.

*Gate:* collation order across the corpus's locales, including the CJK
tailorings `internal/icu` carries.

**Done.** The collator reads ICU's own collation tables, as ICU 78.3 exports
them for ICU4X: a code point trie to 32-bit elements, expansion tables, and
UCharsTrie contexts for contractions and prefixes. Those are inputs to the
algorithm - ICU builds them from CLDR's rules with a rule compiler of many
thousands of lines, and reading them is what every ICU runtime does. The
algorithm is ICU's: the element tags, discontiguous contractions, numeric
runs, script reordering, and the level-by-level comparison of
`CollationCompare`, on NFD text.

What the export does not carry had to come from somewhere, and each gap was
found by the differential below rather than guessed:

- **The combining diacritics and the conjoining jamo are not in the trie.**
  The export keeps them in tables of their own (`dia`, `jamo`), and a collator
  that reads only the trie gives U+0308 an unassigned weight.
- **A tailoring's jamo are lost entirely.** The search collations make a
  trailing consonant equal to the leading one, and the export drops that; ICU4X
  lives without it. Those rules have one shape, `&ᄀᄀ =ᄁ`, so `collgen` reads
  them from ICU's rule sources as runs of jamo - inputs, weighed at run time -
  and refuses any jamo rule of another shape.
- **How collation locales inherit is not in the export.** ICU's collation tree
  differs from the ordinary one: Bokmål collates as Norwegian, Cantonese as
  traditional Chinese, and simplified and traditional Chinese default to
  different collations. That comes from ICU's `data/coll` sources:
  `LOCALE_DEPS.json` and each locale's `default`.
- **Han order.** The export offers `implicithan` and `unihan`. Node sorts
  U+3400 before U+9FA0, which only radical-and-stroke order does, so go-intl
  takes `unihan`. The `fast`/`small` choice turned out not to apply: the
  collation tries carry their own type, and the normalizer does not use the
  export at all.

Beyond the corpus, `testdata/collator_node.js` records the order node puts
1,113 words in - every script the tailorings touch, kana, Hangul, Han, digits,
text out of canonical order - for **every installed collation locale under
every option set and every collation type it supports: 2,903 cases, 2,901 of
them exact.** The other two are one known gap:

- **`ko-u-co-searchjl`** gives jamo secondary weights of its own and prefix
  contexts. Its weights exist only in ICU's compiled data and allocating them
  is the rule compiler's job. Nothing in go-quickjs or test262 reaches it.
- **`en-US-POSIX`** (`en-US-u-va-posix`) carries a variant, which a data
  locale cannot hold, so its ASCII-order collation is not generated and the
  locale sorts as English. Node supports it; test262 and go-quickjs only
  canonicalize the tag, never collate with it.

One compat-profile entry: node writes an option-chosen collation into the
resolved locale (`de` with `{collation: "eor"}` resolves to `de-u-co-eor`);
ECMA-402's ResolveLocale does not.

Cost: a collator builds in about 0.3 ms, most of it the source copying the
half-megabyte root table out of the embedded files; the tables are then read
where they lie. A comparison is 1.5-2 µs. go-quickjs should keep collators
rather than build one per `localeCompare`.

### 8. Segmenter

Needs the LSTM models and the dictionaries for Chinese, Japanese, Thai, Khmer,
Lao and Burmese. Last because it is the most self-contained.

### 9. Retire internal/icu

Remove it from go-quickjs, delete `extract.mjs`, keep `golden.mjs`. Update the
README's Intl section.

## Status

| Stage | State |
|---|---|
| 0. Bootstrap | **done** — module, pins resolved, corpus parsed |
| 1. Locale | **done** - types, parser, canonicalization, fallback |
| 2. Provider and datagen | **done** - Source, embedded FS, localegen, CLDR fallback |
| 3. NumberFormat | **done** - 2,640/2,640 corpus cases, 766 locales |
| 3b. Compact notation | **done** - NumberFormat now 2,970/2,970 |
| 3c. Rest of the surface | corpus done; `numberingSystem`, decimal input and `formatRange` missing |
| 4. PluralRules, ListFormat | 300/300 and 120/120; ListFormat **done**, PluralRules lacks `selectRange` |
| 5. DateTimeFormat | 1,230/1,230, Gregorian and Buddhist; surface incomplete, see below |
| 6. RelativeTimeFormat | 1,260/1,260; lacks `numberingSystem` |
| 6b. DisplayNames | **done** - 34/34 |
| 6c. DurationFormat | not started - not in the corpus |
| **Normalizer** | **done** - Unicode 17.0.0, 779,392 cases against node |
| 7. Collator | **done** - 1,805/1,805, and 2,901/2,903 orders against node |
| 8. Segmenter | not started |
| 9. Retire internal/icu | not started |

**Corpus coverage so far: 7,719 of 7,949 cases, every one of them exact.**

| Service | Cases | Matching |
|---|---|---|
| NumberFormat | 2,970 | 2,970 |
| PluralRules | 300 | 300 |
| RelativeTimeFormat | 1,260 | 1,260 |
| ListFormat | 120 | 120 |
| DateTimeFormat | 1,230 | 1,230 |
| DisplayNames | 34 | 34 |
| Collator | 1,805 | 1,805 |

Switched over in go-quickjs: *none yet, and none until rule 5 is satisfied.*

**Corpus parity is not eligibility.** An audit of go-quickjs's option reads
(2026-09-24) found that an earlier version of this line, "all six finished
services meet both gate conditions", was wrong: only **Collator, ListFormat
and DisplayNames** implement every option go-quickjs does. NumberFormat,
DateTimeFormat, PluralRules and RelativeTimeFormat match the corpus exactly and
still lack options go-quickjs accepts; the next section lists them.

## What is left

The first version of this table counted corpus cases, and so listed only what
the corpus exercises. The real measure is everything go-quickjs gets from
`internal/icu` today, because that is what a switch-over has to replace. This
inventory was taken from go-quickjs's own code: the option names its VM reads
for each service, and the `internal/icu` entry points it calls - about seventy.
Much of `internal/icu` serves things that are not Intl formatters at all.

### Formatter surfaces

What go-quickjs's VM accepts and go-intl does not yet:

| Service | Corpus | Missing |
|---|---|---|
| NumberFormat | done | `numberingSystem` and `-u-nu`; exact decimal input - strings and BigInts, which `Format(float64)` cannot hold; `formatRange`, `formatRangeToParts` |
| DateTimeFormat | done | `dayPeriod`; `fractionalSecondDigits`; `timeZoneName` as `shortOffset`, `longOffset`, `shortGeneric`, `longGeneric`; `numberingSystem` and `-u-nu`; offset time zones; the required and default fields of the `toLocale*` methods (90 corpus cases); `formatRange`, `formatRangeToParts`; 14 calendars |
| PluralRules | done | `selectRange`; `compactDisplay` beside `notation` |
| RelativeTimeFormat | done | `numberingSystem` |
| Segmenter | 140 cases | the service: grapheme, word and sentence breaks, and the dictionaries and LSTM models for scripts without spaces |
| DurationFormat | none | the service |

### Beyond the formatters

| Area | What go-quickjs calls | What it needs |
|---|---|---|
| Locale negotiation | `Has`, `Resolve`, `ResolveTag`, `TagAliases` | each service's available locales, for `supportedLocalesOf` and `localeMatcher`; CLDR's alias data for `getCanonicalLocales` |
| `Intl.supportedValuesOf` | `Calendars`, `Collations`, `Currencies`, `NumberingSystems`, `Units`, `Zones` | the lists, from CLDR's BCP 47 data |
| `Intl.Locale` info | `LocaleCollations`, `LocaleHourCycles`, `LocaleNumberingSystem`, `ScriptDirection`, `TerritoryInfo*`, `WeekInfoForLocale` | CLDR's week data, time data and script metadata |
| Time zones | `CanonicalZone`, `Zones`, `SystemZone`, `LoadTimeZone`, `LoadLocation`, `OffsetName`, `LegacyZoneNameAt`, the Windows zone map | a pinned tzdb with transitions for Temporal, and zone canonicalization |
| Calendar arithmetic | `Date`, `DateIn`, `DateInfo`, `ResolveDate`, `MonthsInYear`, `MonthsBetweenYears` | Temporal's non-ISO calendars: Chinese, Dangi, Hebrew, the Islamic variants, Persian, Indian, Ethiopic, Coptic, Japanese, ROC, Buddhist |

The formatter surfaces come first: they are go-intl's own API, and each item
is small beside the areas below them. Time zones and calendar arithmetic are
the largest pieces left, and they are Temporal's as much as Intl's.

Anything without corpus cases needs what DisplayNames and the Collator
needed: expectations taken from node, recorded in `testdata`, where the corpus
does not reach.

## Resuming cold

A session starting with no memory of the previous one reads, in order:

1. `DESIGN.md` - the architecture and the invariant.
2. This file's **Status** table - where the work stopped.
3. `AGENTS.md` - working rules and the verification commands.

Then re-measure the four gates before changing anything, because a gate that
was not re-measured is not a gate.

Update the Status table in the same commit as the work it describes. A plan
whose state lives only in someone's head does not survive the session that
wrote it.

## Open questions

- **Reference platform for the gate.** Recommend Linux and Windows both, given
  the timezone bug only appeared on one.
- **Package layout.** One `intl` package, or sub-packages per service? Start
  with one; split only if the file count forces it.
- **API stability.** v0 until the services exist, then decide. go-quickjs is the
  only consumer until then.

## Data size, measured

**Decided: not compressed, for now.** The maintainer chose to keep go-intl
free of dependencies and revisit this later. The numbers are here so the
decision can be reopened without measuring again.

**The repository is not the problem.** It is 9.7 MB, a 9.0 MiB packfile, because
git already compresses its objects. An earlier note in this file claimed 48 MB
of data in git; that was `du` counting filesystem block slack across 5,591 small
files, and it was wrong.

**The binary is the problem.** `go:embed` puts all 37.1 MB into any program that
imports the package, so one formatting numbers in English builds to 21.4 MB.

| Approach | Size | Ratio | Cost |
|---|---|---|---|
| raw | 37.1 MB | - | none |
| flate per file | 11.8 MB | 3.13x | 48 µs a file, stdlib |
| zstd per file | 11.9 MB | 3.12x | 17 µs a file |
| **one zstd archive per marker** | **2.4 MB** | **15.7x** | 1-10 ms a marker, once |

Per file, zstd is no smaller than the standard library's flate and about three
times faster to decompress. The size only comes from compressing *across* the
files, because the locales are nearly the same shape: numbers goes 11.95 MB to
0.66 MB, names 9.87 MB to 1.00 MB.

Per-marker is the right granularity because it matches the seam already there: a
program that formats numbers inflates `numbers` once, in ten milliseconds, and
never touches `names`. The binary would go from 21.4 MB to about 5 MB.

The cost is a dependency. go-intl has none, which is worth something for a
library; this would need `klauspost/compress`, which go-quickjs already carries.
A directory source would stay uncompressed either way, so a caller supplying
their own data is unaffected.
