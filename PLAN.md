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

- ~~the digit options `PluralRules` is also given~~ **done**
- ~~`PluralRules.selectRange`~~ **done**: ICU's StandardPluralRanges, from
  CLDR's plural ranges by language, over the ends as rounded. 145,340 cases
  against node, all matching.
- ~~exact decimal input~~ **done**: `Decimal`, `ParseDecimal` (ECMA-402's
  reading of a numeric string, hexadecimal and "Infinity" included) and
  `FormatDecimal`. The formatter now works on decimal digits throughout
  rather than on a float, so a string or a BigInt is written with every
  digit it has; a float is its shortest decimal, as before. A string too
  large for a float is infinite, as V8 has it; one too small is kept exactly,
  as V8 keeps it and ECMA-402 would not. Comparing with node turned up four
  rules that hold for floats too: `signDisplay` asks whether the number is
  zero as written (0.0001 at two decimals has no sign under `exceptZero`); a
  compact number rounding into the next power of ten takes that power's
  pattern (999,999.5 is "1M"); an amount of exactly 0 or 1 takes CLDR's
  explicit pattern where there is one, and a pattern with no digits replaces
  the number (French "mille"); and NaN and infinity are "other" for unit and
  currency names. Compact notation's word is now a `compact` part, with the
  spaces around it literals, as ICU trims them. 6,970 strings in ten
  locales, as strings and parts, all match.
- ~~`formatRange`, `formatRangeToParts`~~ **done**: ICU's NumberRangeFormatter
  at V8's defaults. The formatter now builds a number in ICU's layers -- the
  digits, the scientific exponent, the pattern's affixes, the unit or
  currency name -- and a range writes once the layers its ends share: a unit
  always, in the plural the range takes ("1–2 kilometers"), and the affixes
  when they match and run to two characters or more (German "5,00–10,00 €",
  English "$5.00 – $10.00"). Ends written alike are one number with the
  approximately sign where the sign goes. Sources come from ICU's spans,
  which count a shared prefix without the currency spacing applied with it
  and so sit a character off with a currency code; V8 reports them as they
  are. Comparing with node fixed three things for single numbers too: a
  sign shown in a pattern's negative form goes where its minus is (and a
  number shown unsigned uses the positive form), fields are trimmed of
  spaces and bidirectional marks as ICU trims them (Arabic's minus is a
  hyphen after a literal left-to-right mark), and a unit's name is a `unit`
  part. `UnitDisplay`'s zero value was long; it is now short, ECMA-402's
  default, as DESIGN.md requires of every option. 4,320 ranges in sixteen
  locales, strings and parts, all match.
- ~~`PluralRules` in compact and scientific notation~~ **done**: a number is
  selected as ICU's number formatter writes it, with the power of ten apart
  (`c`) and the digits of the whole value, so "1.5M" is French "many".
  ICU tries a language's rules in its resource table's order -- few, many,
  one, two, zero, then other -- which decides where rules overlap: French
  5E-1 is "many", not "one". A negative number is rounded with its sign.
  30,960 cases against node, all matching.

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
stays at 2,970/2,970. **Met.**

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

**DurationFormat: done.** It is made of the other formatters, as V8 makes it
(js-duration-format.cc): each unit a NumberFormat in unit style or a bare
number, joined by a ListFormat of units, the numeric ones run together with
the time separator. The options resolve by GetDurationUnitOptions; V8
reports a sub-second unit that joins the seconds as "numeric" where the
proposal says "fractional", and never pads the hours by locale.

Where the proposal leaves room, V8's choices are what Node does and are
kept: numeric seconds join whatever part was written last ("1 hr, 46
min:40"), numeric minutes join it whenever the hours are numeric in style,
and a zero minute is forced between hours and seconds only in two-digit
style. The time separator is ICU's DateFormatSymbols': the first
NumberElements/<system>/symbols table in the locale's ICU chain, with no
fallback past it, so Urdu in Persian digits writes a colon although the
root gives those digits "٫" -- and V8 then keeps only ".", "：" and "٫".
numbergen now carries it (`TimeSeparators`, only where it is not a colon).

`testdata/duration_node.js` records 21,882 cases: seventeen option bags and
twenty-four durations in forty locales, three of each in every other
locale. All match but named gaps. One is V8's: it sums a fraction of a
second in an int64 of nanoseconds, and 1e20 of them converts to INT64_MIN,
a result C++ leaves undefined; go-intl sums exactly, as the proposal does.
The others are locale negotiation's: ICU has no data at all for 21 of
CLDR's locales (az-Arab, en-Dsrt, zh-Latn, ...), so V8 resolves them by
truncation, az-Arab to az; and ICU's unit tree has no sr_Cyrl_ME, whose
fallback reads sr_Latn_ME, so Node writes its units in Latin beside
Cyrillic list patterns.

The sweep also found that plural rules were looked up along CLDR's parent
locales, where ICU truncates: Serbian in Latin, Bosnian in Cyrillic and Fula
in Adlam counted as the root does, "1 sati". Fixed, with a regression test.

A DurationFormat builds a NumberFormat for each unit it may write, from one
load of the locale's number data (`numberSources`), which took construction
from 4 ms to 0.4 ms.

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

### DateTimeFormat, as Node chooses its patterns

The corpus covers DateTimeFormat in depth for a few locales. A sweep of every
locale Node supports -- 23 option sets, two zones, two seasons, 33,332 cases
in `testdata/datetime_node.js` -- found go-intl at 96.6%, and the misses were
in how a pattern is chosen, not in the data.

go-intl's skeleton matcher was a reading of UTS #35. Node runs a particular
implementation of it, so the matcher is now that implementation: ICU's
`DateTimePatternGenerator` ported from `dtptngen.cpp` (`dtpg.go`) -- its
distance weights, tie-breaking, field adjustment and appending -- driven the
way V8 drives it (`skeleton.go`): the skeleton spelled in V8's field order
with the hour letter of the resolved cycle, the hour cycle resolved from
`hour12`, `hourCycle`, `-u-hc` and CLDR's time data, and V8's rewriting of
the hour letters afterwards. Style patterns are built as ICU's date
formatter builds them, and regenerated when V8 finds the hour cycle
disagrees.

**33,330 of 33,332** match, and the corpus's 1,230 still do. Along the way,
three more things turned out to be ICU's rather than CLDR's as cldr-json
resolves it, and now come from ICU's sources: the atTime glue where a locale
overrides only the plain one (French in Mali), the offset format's
truncation and fixed digit widths (Hebrew, Makhuwa), and numbering overrides
on a style pattern (Hawaiian writes its short month in Roman numerals).

The two left then were Montenegrin Cyrillic Serbian's zone names, and they
now match too: see the zone names below.

Two findings for later:

- **The plain space before AM/PM is V8's, not ICU's.** V8 replaces U+202F
  with a space in every formatted date (`Replace202F`, reverting ICU 72).
  Done: the data keeps CLDR's character and the NodeICU profile replaces it,
  as a named divergence.
- **125 of the 766 locales go-intl has date data for are not ones Node
  supports at all** -- Afar, Occitan and others ICU does not ship. Node
  negotiates them away to its default locale. Matching that is the
  negotiation layer's job, from ICU's installed locales.

### Zone names, day periods and fractions

**Done.** DateTimeFormat takes `dayPeriod`, `fractionalSecondDigits` and all
six `timeZoneName` styles. `testdata/datetime_features_node.js` records
66,023 cases over every locale Node supports, and
`testdata/datetime_zones_node.js` 45,900 more: every zone Node knows and
offsets in each form ECMA-402 accepts, in every style, in nine locales. All
match but 36, a named gap.

An offset zone ("+05:30", "+0530", "-08") is reported as `±HH:MM`, minus
zero as plus, and written as its offset. V8 hands ICU a custom zone, and ICU
spells the custom zone of no offset "GMT", so "+00:00" alone is named: as
Etc/GMT, "Greenwich Mean Time".

Zone names follow ICU's TimeZoneFormat and TimeZoneGenericNames, not CLDR's
description of them:

- A specific name ("z") is the zone's own name for the season, else its
  metazone's, and never another kind of name: without a short standard name
  a zone is written as its offset.
- A generic name ("v") is the standard name when the zone keeps no summer
  time within 184 days ("India Standard Time"), is qualified by a place when
  the zone's offset differs from the metazone's reference zone in the
  reader's region, and failing a name is the zone's location: its region's
  name when it is the region's only or primary zone ("United Kingdom Time"),
  its city otherwise.
- An offset of zero is "GMT+0", not the locale's "GMT": ICU reads that word
  but does not write it.

The names come from ICU's zone tree rather than cldr-json, merged field by
field along ICU's chain, as ICU reads them. A locale ICU keeps no zone bundle
for falls back by ICU's resource rules, with ICU's own tables of parents and
default scripts, which are not the likely subtags: Montenegrin Cyrillic
Serbian goes to `sr_ME`, an alias of the Latin `sr_Latn_ME`, because ICU's
table has Serbian written in Cyrillic everywhere.

The day period ("B") is noon only when the time as written is exactly noon,
its minutes and seconds zero where the pattern writes them. ICU never writes
midnight.

**Known gap:** Ireland. The tz database gives it a negative daylight saving
in winter, and Go's zone data follows it, so go-intl calls January "Irish
Standard Time"; ICU builds from the rearguard form and calls it "Greenwich
Mean Time". The fix is to take offsets and seasons from ICU's own
`zoneinfo64` rather than Go's zone data, which is part of the time-zone work
below.

### Ranges

**Done.** `FormatRange` and `FormatRangeToParts` are ICU's DateIntervalFormat,
ported from dtitvfmt.cpp and dtitvinf.cpp and driven as V8 drives it: made
from the skeleton of the formatter's own pattern, in the locale with the
resolved hour cycle, and with a range whose ends differ in nothing shown
written once by the formatter itself. The parts' sources come from ICU's
spans, the extent of the fields written twice.
`testdata/datetime_range_node.js` records 92,304 ranges over every locale
Node supports, eighteen option sets and eight pairs of moments, strings and
parts both; all match.

Two things in it are ICU's implementation rather than its data. Of two
equally near interval skeletons ICU takes the one its hash table reaches
first, so the data keeps the order ICU stores them in and go-intl walks them
as ICU's uhash would. And ICU passes skeletons by pointer, so once the month's
pattern is found by extending the skeleton, the year's is looked for in the
extended one. The interval patterns come from ICU's sources, merged along
ICU's chain and following its calendar aliases, as DateIntervalInfo loads
them.

### Numbering systems

**Done.** NumberFormat, DateTimeFormat and RelativeTimeFormat take the
`numberingSystem` option and `-u-nu`, for all 78 of CLDR's numeric systems.
`NumberingSystems` lists them, as `Intl.supportedValuesOf` does. The resolved
locale keeps `-u-nu` only when it was honoured.

A system the locale uses brings the separators and patterns CLDR gives it
there. For any other, ICU looks each mark and pattern up field by field:
first what the locale says about that system, then the root's entry for it,
then the locale's Latin data. cldr-json has the first only for the systems a
locale lists, and has neither of the other two layers. Both come from ICU's
sources in the pinned data archive instead. So Persian with Arabic digits
writes "E" for the exponent where the root would write "اس", and English
with Devanagari digits keeps its own separators.

`testdata/numbering_node.js` records 38,318 cases: every system, by keyword
and by option, in 14 locales, across NumberFormat's styles and notations,
DateTimeFormat and RelativeTimeFormat. All of them match; the 320 that
were once a gap were the `fa` and `ps` dates, in the Persian calendar.

That comparison also turned up two date-time glue bugs the corpus never
reached; both are fixed (see the commit "Join dates to times with the glue
ICU uses").

### Calendars

All eighteen calendars Node supports are implemented: Gregorian, Buddhist,
Persian, Coptic, Ethiopic, Ethiopic Amete Alem, Indian, the five Islamic
ones (civil, tabular, astronomical, Saudi, Umm al-Qura), ROC, Hebrew,
Japanese, ISO 8601, Chinese and Dangi. Each is ICU 78's arithmetic, from
the ICU source named in its comments, and matches go-quickjs's arithmetic
wherever both were compared. `testdata/datetime_calendars_node.js` records
every calendar Node supports in forty locales, dates from 1900 to 2077, and
all 48,960 cases match. `testdata/calendar_days_node.js` checks every day
from 1600 to 2400 in each calendar whose months are not the Gregorian
ones, five million days, and all of them match, on amd64 and on 386.

The calendar is resolved as ECMA-402 resolves "ca": the option if it names
a calendar, else the locale's "-u-ca-" keyword if that does, which the
resolved locale then keeps, else the region's. A well-formed name that is
no calendar's is passed over and an ill-formed one is an error, as in Node.
What Node does and go-intl does not yet is canonicalize an alias,
"islamicc" to "islamic-civil": that needs CLDR's BCP 47 alias data, which
comes with locale canonicalization under "Locale negotiation" below.

Persian is ICU 78's arithmetic (persncal.cpp): the 33-year rule with ICU's
list of corrected years. It is not astronomical, whatever go-quickjs's
generator says; go-quickjs carries a table of month starts read from
Node's output, which go-intl does not.

The sweep also found that "Y", the week-based year, is ICU's: reckoned from
the calendar's extended year and the region's week conventions, which are
now carried (`data/weekdata.bin`, from CLDR's weekData). The Buddhist
calendar's extended year is the Gregorian one, so Burmese Buddhist dates
written with "Y" show 2024, not 2567. "u" (the extended year) and "r" (the
related Gregorian year) are written as ICU writes them too.

Two more are ICU's rather than any calendar's. V8 makes ICU's Gregorian
calendar proleptic, but only a calendar that is exactly ICU's
GregorianCalendar: the Buddhist, ROC and Japanese calendars are subclasses and keep Julian dates before October 1582. And a
calendar with one era counts its years back through zero before it, so the
Islamic year of AD 200 is -435.

ISO 8601 is the Gregorian arithmetic with the root's own patterns ("y MMMM
d, EEEE", "y-MM-dd") over the locale's Gregorian names, weeks from Monday
needing four days, and the Gregorian glue between date and time, which it
has none of. Its era names are ICU's accident: supplementalData has no era
rules for it, and DateFormatSymbols is left with no wide or abbreviated
names and only the first narrow one, so Node writes " 2024" for an era and
a year, and "B 6" before the common era in English.

The astronomical Islamic calendar, which ICU calls both "islamic" and
"islamic-rgsa", starts a month on the first day whose midnight, in UTC,
finds the moon past new by ICU's CalendarAstronomer (astro.cpp), which is
ported: Duffett-Smith's sun and moon, with each constant rounded as ICU's
macros round it and each product converted before it is added, so that no
platform fuses the two. ICU guesses the month from the moon's age at the
moment formatted rather than at midnight, and a guess a month short
stands; go-intl keeps that. Umm al-Qura is ICU's table of which months of
1300 to 1600 AH have thirty days and the corrections to its fit of each
year's start, read from islamcal.cpp (vendored in `internal/icusrc`) into
`data/ummalqura.bin`; outside the table it is the civil calendar. ICU's C
remainder makes every civil year before 0 a leap year, and go-intl's
tabular calendar now agrees.

The Chinese calendar and Dangi, the Korean, are ICU's ChineseCalendar:
months from new moon to new moon, the eleventh holding the winter
solstice, and a leap month wherever a year between solstices has thirteen
and a month holds no major solar term, all found with the ported
CalendarAstronomer in China's time, UTC+8, or in dangical.cpp's Korean
zone (UTC+8, UTC+7 in 1897, UTC+9 from 1912). ICU caches the solstices and
new years it finds; go-intl's formatters keep no caches, so a date costs
some 17 microseconds. The era is the sixty-year cycle, written as a
number; "U" writes the year's name in the cycle, "jia-chen", from CLDR's
cyclic name sets, and a leap month is its month in the locale's leap-month
pattern, "{0}bis" or "闰{0}", loaded as DateFormatSymbols loads them, with
ICU's fix-ups for the Korean calendar's inheritance. The parts are V8's:
"relatedYear" for "r", "yearName" for "U".

The algorithmic numbering systems date patterns name -- Roman numerals for
Hawaiian months, Hebrew numerals, the Japanese era year that calls its first
year 元, the Chinese calendar's days -- are ICU's rule-based number formats.
go-intl carries ICU's rules (`data/rbnf.bin`, 23 KB) and interprets them, as
nfrule.cpp and nfrs.cpp do, for whole numbers. The hand-written Roman
numerals it replaced agreed with it.

The date data is 53 MB with eighteen calendars, up from 9 MB with three: each
calendar keeps its own copy of names and patterns that often repeat the
Gregorian ones, and every locale repeats the Japanese calendar's 237 era
names in three widths. A calendar identical to an earlier one in the same
locale is stored as a reference to it, which the five Islamic calendars
are. That is for the data-size decision below: storing
only what a locale's parent does not say, or compressing across locales,
would take most of it back.

go-quickjs's arithmetic calendars agree with these; its tables of Node's
answers for the astronomical ones (Chinese, Dangi, the Islamic ones,
Persian, the Japanese eras) are not carried, since the algorithms they
recorded are ported.

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
| 3c. Rest of the surface | **done** - exact decimal input and `formatRange`, 6,970 and 4,320 cases against node |
| 4. PluralRules, ListFormat | **done** - 300/300 and 120/120; `selectRange` and notations against node, 176,300 cases |
| 5. DateTimeFormat | 1,230/1,230, and 237,557 cases against node, all but a named 36; `dayPeriod`, `fractionalSecondDigits`, every `timeZoneName`, offset zones and `formatRange` done; all 18 calendars |
| 6. RelativeTimeFormat | **done** - 1,260/1,260 |
| 6b. DisplayNames | **done** - 34/34 |
| 6c. DurationFormat | **done** - not in the corpus; 20,325 cases against node, all but named gaps that belong to locale negotiation |
| 6d. Legacy `toLocale*` | **done** - 90/90; `Required` and `Defaults` on the date options |
| **Normalizer** | **done** - Unicode 17.0.0, 779,392 cases against node |
| **Numbering systems** | **done** - 78 systems, 37,998 cases against node |
| 7. Collator | **done** - 1,805/1,805, and 2,901/2,903 orders against node |
| 8. Segmenter | not started |
| 9. Retire internal/icu | not started |

**Corpus coverage so far: 7,809 of 7,949 cases, every one of them exact.** Only the Segmenter's 140 are left.

| Service | Cases | Matching |
|---|---|---|
| NumberFormat | 2,970 | 2,970 |
| PluralRules | 300 | 300 |
| RelativeTimeFormat | 1,260 | 1,260 |
| ListFormat | 120 | 120 |
| DateTimeFormat | 1,230 | 1,230 |
| DisplayNames | 34 | 34 |
| Collator | 1,805 | 1,805 |
| legacy `toLocale*` | 90 | 90 |

Switched over in go-quickjs: *none yet, and none until rule 5 is satisfied.*

**Corpus parity is not eligibility.** An audit of go-quickjs's option reads
(2026-09-24) found that an earlier version of this line, "all six finished
services meet both gate conditions", was wrong. Now **Collator, ListFormat,
DisplayNames, RelativeTimeFormat, PluralRules and NumberFormat** implement
every option go-quickjs does. DateTimeFormat matches the corpus exactly and
still lacks the non-Gregorian calendars; the next section lists them.

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
| Segmenter | 140 cases | the service: grapheme, word and sentence breaks, and the dictionaries and LSTM models for scripts without spaces |

### Beyond the formatters

| Area | What go-quickjs calls | What it needs |
|---|---|---|
| Locale negotiation | `Has`, `Resolve`, `ResolveTag`, `TagAliases` | each service's available locales, for `supportedLocalesOf` and `localeMatcher`, as V8 builds them from ICU's (21 of CLDR's locales are not ICU's, and resolve by truncation); ICU's per-tree fallback where it differs from CLDR's (sr-Cyrl-ME units and currency names, ku-TR); CLDR's alias data for `getCanonicalLocales` |
| `Intl.supportedValuesOf` | `Calendars`, `Collations`, `Currencies`, `Units`, `Zones` | the lists, from CLDR's BCP 47 data; `NumberingSystems` is done |
| `Intl.Locale` info | `LocaleCollations`, `LocaleHourCycles`, `LocaleNumberingSystem`, `ScriptDirection`, `TerritoryInfo*`, `WeekInfoForLocale` | CLDR's week data, time data and script metadata |
| Time zones | `CanonicalZone`, `Zones`, `SystemZone`, `LoadTimeZone`, `LoadLocation`, `OffsetName`, `LegacyZoneNameAt`, the Windows zone map | a pinned tzdb with transitions for Temporal, and zone canonicalization; ICU's `zoneinfo64` is the candidate, which would also close the Ireland gap |
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
That was measured before the calendars: the data is now 94 MB, 50 MB of it
dates, and the table below has not been measured again.

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
