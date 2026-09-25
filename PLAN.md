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
locale. All match. V8 sums a fraction of a second in a double converted to
an int64 of nanoseconds, and 1e20 of them converts to INT64_MIN, a result
C++ leaves undefined and x86-64 fixes; the NodeICU profile does the same
(a named divergence, tested under both profiles), the standard one sums
exactly, as the proposal does. Other differences had been locale
negotiation's: ICU has no data at all for 21 of
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
  trailing consonant equal to the leading one, and Korean's `searchjl` gives
  the leading consonants prefix contexts and weights of their own; the export
  drops all of it, and ICU4X lives without it. Those weights are allocated by
  ICU's rule compiler, so a collation type whose compiled trie tailors any
  conjoining jamo is taken whole from ICU's compiled data (`icudt78l.dat` in
  the source release, read by `internal/icudat`): its `%%CollationBin`
  trie, rebuilt as the code point trie the collator reads and checked against
  ICU's at every code point, and the arrays it points into, which the export
  renumbers. Its settings, reordering and diacritics still come from the
  export. This replaced reading the search rules as runs of jamo.
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
every option set and every collation type it supports, `en-US-u-va-posix`
and `en-u-va-posix` among them: 2,941 cases, all exact.** `ko-u-co-searchjl`
had been a gap until its jamo came from ICU's compiled data, and
`en-US-POSIX` until a data locale could hold its variant (see "The POSIX
variant").

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
match.

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

Ireland had been a known gap: the tz database gives it a negative daylight
saving in winter, and Go's zone data followed it, so go-intl called January
"Irish Standard Time" where ICU, built from the rearguard form, says
"Greenwich Mean Time". Zones are now ICU's own (see Time zones below), and
all 36 of Dublin's cases match.

### Time zones

A zone is read from ICU's `zoneinfo64`, from the time zone update Node runs
(2026c), not from the host's or Go's tz data: `tzgen` writes one file per
name ICU knows, `data/tz/<name>.bin` in lowercase, with the name as ICU
spells it, its canonical name (`ZoneMeta::getCanonicalCLDRID`), and either
the zone it links to or its offsets as `OlsonTimeZone` keeps them: raw
offset and daylight saving apart, the transitions, and the final rule. 637
names, 1.3 MB. A name is valid when its file exists, in any case, as
ECMA-402 and V8 now match names; Etc/Unknown and Factory, whose canonical
zone is Etc/Unknown, have none. resolvedOptions reports the canonical name,
"UTC" for Etc/UTC and Etc/GMT.

`timezone.go` ports what ICU computes from them: `OlsonTimeZone::getOffset`,
`getOffsetFromLocal` with ICU's options for skipped and repeated local
times, and `getNextTransition`/`getPreviousTransition`, which skip
transitions that change nothing except the one into the final rule;
`SimpleTimeZone`'s rule evaluation (`compareToRule`) and its
`AnnualTimeZoneRule` transitions. The formatter reckons local time with the
offset, and zone names ask the zone, not Go, whether it is daylight time,
whether summer time is within 184 days (TZGNCore), and what the reference
zone's offset is at the same wall time.

`testdata/timezone_node.js` records what Intl.DateTimeFormat resolves for
every ICU name in three spellings and odd inputs, 1,889 in all, and every
zone's offsets from 1800 to 2100, transition by transition, as Node's
Temporal reports them: all match. Temporal refuses some names
Intl.DateTimeFormat takes (Java's three-letter ones, SystemV's, Factory);
the `temporal` package follows Temporal's own list (see 8c).

`TimeZone` is the public face of it, for go-quickjs and for the `date` and
`temporal` stages: `LoadTimeZone` by name or offset, `ID` as ICU spells the
name, `Canonical` as resolvedOptions reports it, `Offset` with the raw
offset and saving apart, `OffsetFromLocal` with `Former` or `Latter` for
a skipped and a repeated time (V8's Date takes `Former` for both, which the
tests hold to Node's), and `NextTransition` and `PreviousTransition`.

`HostTimeZone` is ICU's `TimeZone::detectHostTimeZone`, which is Node's
default zone, and `DefaultTimeZone` V8's name for it, "UTC" where ICU has
none. On Windows (`hostzone_windows.go`) it ports `uprv_detectWindowsTimeZone`:
the dynamic zone's key, looked up in CLDR's Windows mapping for the user's
region (`data/windowszones.bin`, from the 2026c update, by `tzgen`), else
001; `Etc/GMT` at the offset where daylight saving is turned off; and the
registry search a remote session needs. TZ is not read there, as ICU does
not read it. Elsewhere (`hostzone_other.go`) it ports `uprv_tzname`: TZ
where ICU's `isValidOlsonID` takes it, then the zone `/etc/localtime` links
to, then the zone file it is a copy of. ICU's last resort there, guessing
from the C library's abbreviations, is not ported: Go has no such names, so
that host is Etc/Unknown where ICU might name a zone. Then, as ICU does, a
name ICU does not spell exactly so, or a three- or four-letter one at
another raw offset, is a zone of the host's offset with no canonical name.

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

**Done.** `Segmenter` is Intl.Segmenter as V8 builds it on ICU's break
iterators, ported: `RuleBasedBreakIterator`'s state machine
(`handleNext`, with its look-ahead rules and rule statuses), its break
cache and dictionary cache (a run of dictionary characters between two
rule boundaries handed to the engine for its first character, and the
engine's boundaries, which may run past the rule boundary, taken with the
closing rule's status), and the dictionary engines of `dictbe.cpp`: Thai,
Lao, Burmese and Khmer, one look-ahead algorithm with each script's sets
and Thai's suffixes, and Chinese and Japanese, the cheapest segmentation by
the dictionary's costs, with katakana runs, NFKC normalization and its
position map. The tries are ICU's `UCharsTrie` and `BytesTrie` and the
character classes its `UCPTrie`, read as they lie.

The rules and dictionaries are ICU's own compiled ones, from the
`icudt78l.dat` its source release ships and Node carries: `segmentgen`
copies out `char`, `word`, `word_POSIX`, `sent` and `sent_el` and the five
dictionaries, the way `zoneinfo64` and the collation export are taken, and
reads which rules each bundle of the brkitr tree names (resolved as ICU
opens it, so "el" breaks sentences at semicolons and "en-US-u-va-posix"
words at colons) and the engines' Unicode sets and the Script property
from UCD 17. Node uses no LSTM models: its ICU data has none, so ICU falls
back to the dictionaries for Thai and Burmese too.

ICU's factory keeps every engine it makes for the life of the process, and
an engine made for any character claims every character of its set. go-intl
reckons as a process that has made them all; a process that has not yet
broken any Chinese or Japanese text finds no engine for a run that starts
with U+30FC, U+FF70, U+FF9E or U+FF9F, which are of the Common script, and
breaks it differently.

All 140 corpus cases match, and `testdata/segmenter_node.js` records 9,555
more: sentences in two dozen scripts and 3,000 random strings from pools of
combining marks, emoji sequences, Hangul jamo, Indic conjuncts, the
dictionary scripts, halfwidth and fullwidth forms and lone surrogates, in
every granularity and in the locales whose rules differ, with
`containing` checked at every offset. All match.

### 8b. `date`: JavaScript's Date

A supplementary package, `github.com/go-quickjs/go-intl/date`, with Date's
semantics apart from any engine: time values and ECMA-262's MakeDay,
MakeTime, LocalTime and UTC over ICU's zones as V8 reads them
(`getOffsetFromLocal` with kFormer for both skipped and repeated times),
`Date.parse` as V8's date parser takes strings, and the strings Date
writes. `toString`, `toDateString`, `toTimeString` and `toUTCString` must
match Node exactly for every locale and zone. The zone's name in brackets
is V8's `ICUTimezoneCache::LocalTimezone`: the instant mapped to an
equivalent year when outside 1970 to 2038, only whether it is daylight
time kept, and `TimeZone::getDisplayName(daylight, LONG, default locale)`,
which takes the metazone at the current time, not at the instant, and
falls back to the localized GMT format of the zone's current raw offset
and saving.

Its constructor takes a locale and a time zone, both optional: the locale
the bracketed names are written in, and the zone local time is reckoned in.
Left out, they are the host's, as Node's are: the zone `HostTimeZone`
finds, and the host's default locale. It takes a clock too, for the
current time the names are chosen at, the system's by default. Expectations
come from Node run under each zone (`process.env.TZ`); a default locale
other than the host's needs Node on Linux, where `LANG` sets it.

V8 reads local time through `DateCache`, which keeps segments of constant
offset and assumes a zone changes at most once in 19 days. Where a zone
changed twice in less, it answers with an offset the zone did not have at
the instant asked about, and what it answers depends on what it was asked
before: in Africa/El_Aaiun, Node calls 1976-04-14T01:00Z UTC+1 after being
asked about the millisecond before it, and UTC when asked first. So the
`Environment` carries the cache, ported, behind a mutex, and is the one
stateful thing in go-intl: it is safe for concurrent use, but its answers
are history-dependent as V8's are. A Date object is a `Date` made through
the `Environment`, because V8's `JSDate` asks the cache whenever its value
is set and answers its year-to-second getters from what it read then; an
engine that makes its Date objects here asks the cache what Node's does.
`testdata/date_node.js` records `toString`, `toDateString`, `toTimeString`
and `getTimezoneOffset` about the first 40 transitions from 1965 of every
zone, and at the ends of time; dates from local fields either side of and
inside each transition; `Date.parse` of 133 strings in four zones; and
`toUTCString` and `toISOString`, in the order the replay asks them.

### 8c. `temporal`: Temporal

A supplementary package, `github.com/go-quickjs/go-intl/temporal`, with
Temporal's abstract operations apart from any engine: ISO date and time
arithmetic, durations and rounding, parsing, and zoned time over ICU's zones
(possible instants, disambiguation, transitions), with the non-ISO
calendars' arithmetic (the calendar arithmetic row above). go-quickjs's VM
keeps the objects and calls in. Temporal's own choice of zones, narrower
than Intl.DateTimeFormat's, is followed.

**Node's Temporal is not ICU4C's.** Node 26.10.0 builds Temporal from Rust
crates (`deps/crates/Cargo.lock`): `temporal_rs` 0.2.3 over ICU4X's
`icu_calendar` 2.2.1, `calendrical_calculations` 0.2.4 and
`icu_calendar_data` 2.2.0, with zones from `zoneinfo64` 0.3.0, which reads
the same ICU zone data go-intl does. So Temporal's calendars follow ICU4X,
where Intl.DateTimeFormat's follow ICU4C, and the two disagree: over every
day from 1000 to 3000, the Chinese and Dangi calendars put the day in a
different month or on a different date in about 1,800 years, and in a few
years (1954, 1999, 2012, 2027, 2057...) even between 1900 and 2102, where
ICU4X reads tables (Qing 1900-1911, then China and Korea to 2102) and
ICU4C computes the astronomy; outside them ICU4X has a mean-motion
approximation. Buddhist, Japanese and ROC differ before 1582, where ICU4C
changes to the Julian calendar and Temporal stays proleptic. Temporal takes
neither "islamic" nor "islamic-rgsa". The other calendars agree day for day.

So the `temporal` package carries ICU4X's reckoning, ported from those
crates at those versions, and `intl` keeps ICU4C's for formatting, as V8
does: V8 formats a Temporal date by handing ICU4C its ISO date. The port
starts with the calendars: a date's fields from its ISO date and back, with
Temporal's overflow; then adding to a date and the difference between two;
then the rest of Temporal.

**The calendars, ISO date to fields: done.** `temporal.Calendar` reckons
all sixteen of Temporal's calendars as `icu_calendar` 2.2.1 does, each as
ICU4X's `DateFieldsResolver` over a year laid out as its first day and its
months: the Gregorian ones with their eras (the Japanese from Meiji 6, CE
and BCE before), Coptic and the two Ethiopian eras, Indian, the "fast"
Persian with its 78 corrections, the tabular Islamic from either epoch,
Hebrew, and Umm al-Qura, Chinese and Korean from ICU4X's tables
(`data/temporalcalendars.bin`, which `temporalgen` copies out of the
checksummed crate) and, outside them, its mean-motion approximation, ported
in its integer arithmetic, with the six-bit new-year offset it packs.
`testdata/temporal_calendars_node.js` records every year of every calendar
from ISO -3000 to 3000 and at both ends of Temporal's range, month by
month, and the Japanese era of every day from 1868 to 2030: all 96,659
years match.

**Fields to dates, and date arithmetic: done.** `DateFromFields`,
`YearMonthFromFields`, `MonthDayFromFields`, `DateAdd` and `DateUntil` are
temporal_rs's calendar operations: the ISO calendar by the specification's
algorithms, the others through ICU4X's `ArithmeticDate` (`from_fields` with
its missing-field strategies and reference years, `added`, and `until`
with its surpass checker), and errors as temporal_rs throws them,
`ErrType` for missing fields and `ErrRange` for the rest.
`testdata/temporal_fields_node.js` records 311,526 cases across the
sixteen calendars, fields by ordinal month and by code, leap months, eras
and their aliases, both overflows, adding durations of each unit and
differencing with each largest unit: all match. ICU4X balances a month
count year by year, so `until` in months across millennia of Chinese years
is slow, as it is in ICU4X.

**The rest of Temporal: done.** `PlainDate`, `PlainTime`, `PlainDateTime`,
`PlainYearMonth`, `PlainMonthDay`, `Instant`, `ZonedDateTime` and
`Duration` are temporal_rs 0.2.3's, ported: construction, fields, `with`,
adding, differencing with rounding and balancing (the nudge windows and
bubbling of `Duration.round` and `total`, relative to a date or a zoned
date-time), rounding, and the strings Temporal writes. temporal_rs keeps
epoch nanoseconds and time durations in i128; the port has an `int128`
with Rust's semantics (wrapping arithmetic, `div_euclid`, `as f64` to
nearest-even, saturating casts back), checked against `math/big`. Node
builds `temporal_capi` with `float64_representable_durations`, so a
duration's microseconds and nanoseconds are held as float64, as there.
Strings are parsed by a port of the `ixdtf` 0.6.4 parser, with its error
messages, over the bytes V8 hands it: a one-byte JavaScript string as
Latin-1, a two-byte one as UTF-8, a lone surrogate refused.

Zones are what temporal_rs takes, which is not what Intl.DateTimeFormat
takes: names from `timezone_provider` 0.2.3's normalizer, built from tz
2025c (598 names, 152 of them links, spelled as the table spells them in
any case asked, compared by primary name), which `tzidgen` copies out of
the checksummed crate into `data/temporalzones.bin`; and offsets from
ICU's zoneinfo64 read as the `zoneinfo64` 0.3.0 crate reads it, its own
final-rule evaluation included, over the same tz 2026c data `intl` uses.
`Data` loads calendars and zones once, eagerly, and is immutable after.

What V8 does before it calls temporal_rs is V8's, not temporal_rs's, and
the package leaves it to the engine: reading property bags in alphabetical
order, `ToIntegerWithTruncation`, clamping a month or day to an int8 under
`constrain`, the options and their order, and the messages it throws
itself. The replay holds a port of that glue (`node_glue_test.go`,
`node_ops_test.go`) so the expectations are Node's end to end, and
go-quickjs's VM is to do what it does. `Temporal.Now` needs a clock and is
the engine's; `toLocaleString` is Intl.DateTimeFormat's, which each type's
`EpochNanosecondsForUTC` feeds.

`testdata/temporal_node.js` records 368,399 calls: 1,166 strings parsed as
all eight types; every `Duration` operation over 37 durations and 13
`relativeTo`s; every method and getter of the other seven types, the dates
in all sixteen calendars; zoned date-times about transitions in 20 zones
with every disambiguation and offset option; and, for all 750 zone names,
three spellings and twelve transitions each way. All match. ICU4X's
`until` counts months one at a time, so differences across the whole of
Temporal's range are recorded for ISO only: in the Chinese calendar one
takes Node minutes.

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
| 5. DateTimeFormat | 1,230/1,230, and 286,519 cases against node, all match; `dayPeriod`, `fractionalSecondDigits`, every `timeZoneName`, offset zones and `formatRange` done; all 18 calendars |
| 6. RelativeTimeFormat | **done** - 1,260/1,260 |
| 6b. DisplayNames | **done** - 34/34 |
| 6c. DurationFormat | **done** - not in the corpus; 21,202 cases against node, all match; V8's int64 overflow is a named NodeICU divergence |
| 6d. Legacy `toLocale*` | **done** - 90/90; `Required` and `Defaults` on the date options |
| **Normalizer** | **done** - Unicode 17.0.0, 779,392 cases against node |
| **Numbering systems** | **done** - 78 systems, 37,998 cases against node |
| 7. Collator | **done** - 1,805/1,805, and 2,941/2,941 orders against node, `searchjl` and POSIX included |
| **Time zones** | **done** - ICU's zoneinfo64 (tz 2026c): 1,889 names and every zone's transitions 1800-2100 match Node; Dublin's gap closed; `TimeZone`, and the host's zone as ICU detects it |
| 8. Segmenter | **done** - 140/140, and 9,555 cases against node |
| 8b. `date` package | **done** - 134,418 cases against node, every zone Node knows, all match |
| 8c. `temporal` package | **done** - all 16 calendars, 96,659 years; fields, adding and differencing, 311,526 cases; every type's methods, parsing and all 750 zone names, 368,399 calls; against node, all match |
| **Data size** | files written once, 105 MB to 57.1 MB; every set kept per locale shared through a pool, 18.3 MB |
| 9. Retire internal/icu | not started |

**Corpus coverage: 7,949 of 7,949 cases, every one of them exact.**

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
| Segmenter | 140 | 140 |

Switched over in go-quickjs: *none yet, and none until rule 5 is satisfied.*

**Corpus parity is not eligibility.** An audit of go-quickjs's option reads
(2026-09-24) found that an earlier version of this line, "all six finished
services meet both gate conditions", was wrong. Now **Collator, ListFormat,
DisplayNames, RelativeTimeFormat, PluralRules and NumberFormat** implement
every option go-quickjs does, and so does DateTimeFormat, all eighteen
calendars included.

## What is left

The first version of this table counted corpus cases, and so listed only what
the corpus exercises. The real measure is everything go-quickjs gets from
`internal/icu` today, because that is what a switch-over has to replace. This
inventory was taken from go-quickjs's own code: the option names its VM reads
for each service, and the `internal/icu` entry points it calls - about seventy.
Much of `internal/icu` serves things that are not Intl formatters at all.

### Formatter surfaces

What go-quickjs's VM accepts and go-intl does not yet:

None: the Segmenter, the last, is done.

### Beyond the formatters

| Area | What go-quickjs calls | What it needs |
|---|---|---|
| Time zones | `CanonicalZone`, `Zones`, `SystemZone`, `LoadTimeZone`, `LoadLocation`, `OffsetName`, `LegacyZoneNameAt`, the Windows zone map | done (see Time zones): `TimeZone`, `HostTimeZone`, `DefaultTimeZone`, `TimeZones`; Temporal's possible instants belong to the `temporal` stage, and `LegacyZoneNameAt` to `date` |
| Calendar arithmetic | `Date`, `DateIn`, `DateInfo`, `ResolveDate`, `MonthsInYear`, `MonthsBetweenYears` | Temporal's non-ISO calendars, as ICU4X's `icu_calendar` 2.2.1 reckons them (see 8c): part of the `temporal` package |

The formatter surfaces come first: they are go-intl's own API, and each item
is small beside the areas below them. Time zones and calendar arithmetic are
the largest pieces left, and they are Temporal's as much as Intl's.

Anything without corpus cases needs what DisplayNames and the Collator
needed: expectations taken from node, recorded in `testdata`, where the corpus
does not reach.

### Canonicalization

`Canonicalizer` gives a tag the form `Intl.getCanonicalLocales` gives it,
as V8 and ICU 78.3 give it: V8's check of the language identifier, then the
legacy and redundant tags ICU's parser rewrites first ("zh-hakka" to
"hak", "sgn-no" to "nsl", from uloc_tag.cpp, vendored), then ICU's strict
parse, then ICU's AliasReplacer over CLDR's language, region, script,
variant and subdivision aliases, and every spelling of a Unicode extension
type written as its BCP 47 id (`data/aliases.bin`, from ICU's metadata and
keyTypeData, by `aliasgen`). A replacement fills only the fields a tag
leaves empty, so "cnr-BA" is "sr-BA"; a region that split becomes the one
the language most likely means, "hy-SU" "hy-AM"; the extensions are
written in singleton order. `testdata/canonical_node.js` records every
alias ICU knows in a few surroundings, every extension type and test262's
tags: all 11,139 match.

One divergence, in the compatibility profile: V8 answers two lowercase
letters alone without consulting ICU, so "bh" stays "bh" where the standard
makes it "bho".

### The POSIX variant

ICU reads "-u-va-posix" as the locale's variant POSIX, not as a keyword, and
keeps data under one variant alone: `en_US_POSIX`, whose collation sorts in
ASCII order, whose numbers are written without grouping ("0.######") and
infinity as "INF", and whose words break at colons. So a `DataLocale` holds
that one variant (`Locale.Data` sets it from the keyword or the bare
variant), the fallback chains drop it first (`en-US-posix`, `en-US`, `en`,
the root; `en-posix` to `en`), `collgen` writes the POSIX tailoring the
export has, and `numbergen` writes `en-US-posix` as en-US with
`en_US_POSIX`'s own patterns and marks from ICU's sources, cldr-json having
no such locale. The segmenter's special case gave way to the chain.

V8's ResolveLocale rebuilds the extension from the keywords a service uses,
but ICU's variant is not a keyword, so the resolved locale keeps
"-u-va-posix" in every service: `en-US-u-va-posix` for NumberFormat,
`en-u-va-posix` for PluralRules, which has no en-US. `withKeywords` keeps it
whatever else is dropped. That pass also found services keeping keywords V8
drops: PluralRules, ListFormat and DisplayNames keep none now, and
DateTimeFormat only "ca", "hc" and "nu" -- a "-u-rg-" that DateTimeFormat
had been honouring, and that Node's does not. LocaleInfo's hour cycles,
which do honour it, now come from the region's time data rather than from a
DateTimeFormat. `testdata/variant_node.js` records all nine services with
eleven tags: all 242 match.

Two NumberFormat bugs it turned up: a pattern without grouping is grouped by
threes under `useGrouping: "always"`, as ICU's Grouper does, and the unit
"percent", short or narrow and not compact, is written with the percent
pattern, unscaled, its sign typed as the unit ("0,5 %" in German, "%5" in
Turkish), as ICU does. `useGrouping: "min2"` was missing and is added.
`number_decimal_node.js` now records both: 10,824 cases, all match.

### Negotiation

`LocaleMatcher` is ECMA-402's ResolveLocale and SupportedLocales for one
service, over that service's available locales as V8 builds them from
ICU's (`data/available.bin`, by `availgen`): ICU's installed locales, as
its build indexes them, kept where the bundle or its language's holds what
the service reads ("NumberElements", "calendar", "listPattern"), each also
without its script; the Collator's from the collation tree, PluralRules'
from plurals.txt. 21 of CLDR's locales have no ICU data and so are not
available: "az-Arab" resolves as "az", "ht" as the default.
`testdata/available_node.js` checks every name ICU has a bundle for, cut
short and without its script, in nine services: all 9,747 agree.

"Best fit" matches by lookup, as V8's does: its LocaleMatcher-based best
fit is behind a flag Node leaves off, and across every available locale,
alone and in pairs, Node's two matchers answer alike. With negotiation the
DurationFormat sweep's 21 locale gaps close.

Data is read along the chain ICU reads, per tree of ICU's data:
`Fallbacker.ChainIn` resolves a locale as `ures_open` does, in ICU's own
index of the tree (`data/icutree-<tree>.bin`: its bundles, aliases and
parents) and, for a bundle that does not exist, ICU's default scripts and
parent locales (`data/icufallback.bin`), all written by `availgen`. ICU
opens an alias bundle as the one it names and otherwise drops a default
script, where CLDR's chain truncates: "zh-TW" had been written in
simplified Chinese from "zh", and is now "zh-Hant-TW"'s traditional; "sr-ME"
is Serbian in Latin and "uz-AF" Uzbek in Arabic; "sr-Cyrl-ME" keeps its
Cyrillic dates and names but writes Latin units and currency names, ICU's
unit and currency trees having no bundle of its own; and "az-Arab", which
ICU has no data for, is the root's, as it is for a service that is handed
it without negotiation. The collator reads the collation tree the same way.
Zone names are the exception: zonegen already resolves ICU's zone and
region trees for each locale it writes, so only a locale without a file is
resolved at run time. The index is read by the resolution that needs it,
one tree's file of about 10 KB, so building a formatter costs no more than
it did.

The root's data had never been read: a root data locale opened the
marker's shared file, `dates.bin`, which a per-locale marker has none of,
so a chain that ran out of locales failed rather than ending at the root.
It now opens `<marker>/und.bin`, and "und", "und-TW" and the like format.

### Locale info

`LocaleInfo` answers what `Intl.Locale` says beyond a locale's subtags, as
V8 answers it from ICU (js-locale.cc): `Maximize` and `Minimize`; the
calendars the region reckons in, most preferred first (calendarprefs.bin
now keeps the whole list); the collations along the collation tree's
chain, in BCP 47's spelling; the pattern generator's default hour cycle;
the default numbering system; the canonical zones of the region subtag,
SystemV's among them for 001, as ICU counts a zone canonical when it is
neither an alias nor a tz link; the direction of the locale's, or its
likely, script, from ICU's script properties (uscript_props.cpp, vendored)
and uloc_isRightToLeft's shortcut for common languages; and the first day
of the week and the weekend. Each takes its keyword ("-u-ca-", "-u-co-",
"-u-hc-", "-u-nu-", "-u-fw-") where one is given, and the region for
supplemental data as ICU finds it: "-u-rg-", the region, "-u-sd-", the
likely region. `Minimize` had tried the request's own region and script
rather than the maximized ones; it now gives "zh-TW" for "zh-Hant", as ICU
does. `testdata/localeinfo_node.js` records 690 locales: all nine answers
match for every one.

### Supported values

`Calendars`, `Collations`, `Currencies`, `NumberingSystems`, `TimeZones`
and `SanctionedUnits` are the lists `Intl.supportedValuesOf` answers with.
The collations, currencies and time zones are V8's, built from ICU's
(`data/values.bin`, by `availgen`): every collation the collation tree
names, in BCP 47 spelling, but "standard" and "search"; ICU's common,
current ISO currencies, a list compiled into ucurr.cpp (vendored), with an
English name, V8's four additions and without VEF; and ICU's canonical
zones in a region. `testdata/values_node.js` records all six: identical,
order included.

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

## Data size

**Decided: remove the redundancy, not compress it.** Before go-quickjs is
wired to go-intl the data has to come down: `go:embed` puts all of it into
any program that imports the package, and go-quickjs's `internal/icu` carries
about 14 MB. The data had grown to 105 MB, 55.6 MB of it dates. Compressing
each file would give about 14 MB, but every formatter built would inflate its
locale again, with no cache allowed; so the maintainer chose to take the
redundancy out of the data instead, keeping every table read where it lies.

Measured on 2026-09-25, the redundancy is of two kinds:

- **Whole files that are another's.** CLDR's locales are written resolved,
  so en-AG writes what en-001 writes: 103.9 MB of files hold 56.1 MB that is
  distinct.
- **The same text over and over inside them.** The dates files hold 21.9 MB
  of strings of which 0.24 MB are distinct, and of 7,506 calendars' lists
  only 640 skeleton lists, 444 interval lists and 2,001 name lists differ.
  Dates would be about 1.5 MB with each list and string stored once.

**Files written once: done.** `internal/datawrite` writes every data set kept
per locale, for the nine generators that write one: a locale whose data is
another's byte for byte has no file, and the set's `same.bin`, a table of data
locales read in place, names the locale that has it; the Source follows it.
Every locale reads what it read before, byte for byte (checked against the
data as it was), and nothing reading the data changed. CLDR's be-tarask,
ca-ES-valencia and el-polyton, which a data locale cannot address and ICU has
no data for, are no longer written. The collation root moved from
`collation.bin` to `collation/und.bin`, where every other set keeps its root.
105 MB became 57.1 MB.

**Lists and strings shared: dates done.** `blob` has a pool: a generator
gives every table of a set one, and a part written through `Shared` or
`SharedString` is kept in it once, each table holding its number; the pool
is laid out to be read where it lies, a part found by number without reading
any other. Dates write every string, each list and each calendar through it,
into `datesshared.bin`: 18.8 MB became 1.2 MB of pool and 57 KB of locale
files, and the data 28.3 MB. A locale's calendars are read from the pool
only when asked for, one of eighteen, and the embedded source serves the pool
in place (a string embedded beside the file system, whose memory Source's
readers never change) rather than copying it on every Open. A date formatter
now builds in about 1.5 ms and allocates 1.35 MB, from 2.0 ms and 1.74 MB.

Numbers likewise, into `numbersshared.bin`: each system, each currency and
each list is a shared part, and a currency is read from the pool only when
one is asked for. 8.6 MB became 3.1 MB, of which 1.5 MB is text no other
locale shares, and the data 22.7 MB.

Zone names likewise, into `zonenamesshared.bin`: 6.4 MB became 3.9 MB, of
which 2.5 MB is the translated zone and city names themselves, which no two
locales share; the data 20.3 MB.

Display names (4.2 to 3.4 MB, nearly all of it names no two languages
share), units (1.3 to 0.5 MB) and relative time (0.75 to 0.3 MB) likewise;
the data 18.3 MB. Every set kept per locale now reads through `openShared`.

What a program pays is the binary: one that formats a number in English was
60.9 MB before any of this and is 21.8 MB after (Windows, amd64). The linker
already kept identical embedded files once, so writing files once shrank the
repository more than the binary; the pools are what shrank the binary.

**Loading time, which matters more than size.** A host builds a formatter
on every toLocaleString and nothing may be cached, so what building one
costs is paid over and over. Measured (`load_bench_test.go`), most of it was
`embed.FS` copying files out: the likely subtags and parent locales, reread
for every fallback chain, were 93% of what a ListFormat allocated. So the
data directory is now also packed into one file, `data.pack`
(`internal/packgen`, `internal/datapack`: a sorted index and the files),
embedded as a string and served in place by binary search, and the embedded
file system and the per-pool strings are gone; `data/` stays what the
generators write and `NewFS` serves, and a test holds the pack to it. In
English, a Collator went from 344 to 33 µs, a ListFormat from 151 to 16 µs,
a NumberFormat from 148 to 73 µs, a DateTimeFormat from 1.07 to 0.32 ms and
a Segmenter from 1.82 to 0.57 ms, each allocating a fraction of what it did.

Then the tables every build read in full became indexes (`blob.Index`:
sorted keys and values, found by binary search where they lie): ICU's tree
indexes and the tables its fallback reads, the normalization tables, the
break engines' Unicode sets and scripts, the hour-cycle preferences, the
zones' shared metadata, the locale aliases and the services' available
locales. A locale's names of metazones, zones and
regions are tables in their pool (`blob.Table`): records of pool numbers,
the key's first, all of one width, so a name is found by binary search
without an index of offsets, for 133 KB more pool than the lists read whole;
the display names, the units and the currencies' names are tables the same
way, for 58, 41 and 22 KB more, and a relative-time field is read from its
pool when a format asks for it.
Building no longer decodes any of them into maps; it looks up what it needs.
And a string read from the data is the data's own memory rather than a copy
(a Source's bytes never change), which halved what most builds allocate.

In English now, against where this started:

| Built | Then | Now |
|---|---|---|
| Collator | 344 µs | 26 µs |
| ListFormat | 151 µs | 2.5 µs |
| PluralRules | | 2.7 µs |
| NumberFormat | 148 µs | 8 µs |
| Segmenter | 1.82 ms | 21 µs |
| RelativeTimeFormat | 303 µs | 14 µs |
| DurationFormat | 575 µs | 23 µs |
| DisplayNames | 224 µs | 2.8 µs |
| DateTimeFormat, fields | 1.07 ms | 66 µs |
| DateTimeFormat, full styles and a zone name | 1.19 ms | 130 µs |
| Canonicalizer | 392 µs | 1 µs |
| LocaleMatcher | 201 µs | 0.36 µs |

What a DateTimeFormat still spends is spread out: reading its calendar,
building the pattern generator from the locale's skeletons, and the range
patterns, each as ICU builds them.

What is left is mostly text no two locales share -- translated names of
languages, regions, zones, cities and currencies, about 9 MB -- and ICU's
segmentation dictionaries, 3.1 MB, which no sharing reduces. Inlining the
parts only one table uses, which pay an offset and a number for nothing,
would save an estimated 1.5 to 2 MB more.

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
| 5. DateTimeFormat | 1,230/1,230, and 286,519 cases against node, all match; `dayPeriod`, `fractionalSecondDigits`, every `timeZoneName`, offset zones and `formatRange` done; all 18 calendars |
| 6. RelativeTimeFormat | **done** - 1,260/1,260 |
| 6b. DisplayNames | **done** - 34/34 |
| 6c. DurationFormat | **done** - not in the corpus; 21,202 cases against node, all match; V8's int64 overflow is a named NodeICU divergence |
| 6d. Legacy `toLocale*` | **done** - 90/90; `Required` and `Defaults` on the date options |
| **Normalizer** | **done** - Unicode 17.0.0, 779,392 cases against node |
| **Numbering systems** | **done** - 78 systems, 37,998 cases against node |
| 7. Collator | **done** - 1,805/1,805, and 2,941/2,941 orders against node, `searchjl` and POSIX included |
| **Time zones** | **done** - ICU's zoneinfo64 (tz 2026c): 1,889 names and every zone's transitions 1800-2100 match Node; Dublin's gap closed; `TimeZone`, and the host's zone as ICU detects it |
| 8. Segmenter | **done** - 140/140, and 9,555 cases against node |
| 8b. `date` package | **done** - 134,418 cases against node, every zone Node knows, all match |
| 8c. `temporal` package | **done** - all 16 calendars, 96,659 years; fields, adding and differencing, 311,526 cases; every type's methods, parsing and all 750 zone names, 368,399 calls; against node, all match |
| **Data size** | files written once, 105 MB to 57.1 MB; every set kept per locale shared through a pool, 18.3 MB |
| 9. Retire internal/icu | not started |

**Corpus coverage: 7,949 of 7,949 cases, every one of them exact.**

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
| Segmenter | 140 | 140 |

Switched over in go-quickjs: *none yet, and none until rule 5 is satisfied.*

**Corpus parity is not eligibility.** An audit of go-quickjs's option reads
(2026-09-24) found that an earlier version of this line, "all six finished
services meet both gate conditions", was wrong. Now **Collator, ListFormat,
DisplayNames, RelativeTimeFormat, PluralRules and NumberFormat** implement
every option go-quickjs does, and so does DateTimeFormat, all eighteen
calendars included.

## What is left

The first version of this table counted corpus cases, and so listed only what
the corpus exercises. The real measure is everything go-quickjs gets from
`internal/icu` today, because that is what a switch-over has to replace. This
inventory was taken from go-quickjs's own code: the option names its VM reads
for each service, and the `internal/icu` entry points it calls - about seventy.
Much of `internal/icu` serves things that are not Intl formatters at all.

### Formatter surfaces

What go-quickjs's VM accepts and go-intl does not yet:

None: the Segmenter, the last, is done.

### Beyond the formatters

| Area | What go-quickjs calls | What it needs |
|---|---|---|
| Time zones | `CanonicalZone`, `Zones`, `SystemZone`, `LoadTimeZone`, `LoadLocation`, `OffsetName`, `LegacyZoneNameAt`, the Windows zone map | done (see Time zones): `TimeZone`, `HostTimeZone`, `DefaultTimeZone`, `TimeZones`; Temporal's possible instants belong to the `temporal` stage, and `LegacyZoneNameAt` to `date` |
| Calendar arithmetic | `Date`, `DateIn`, `DateInfo`, `ResolveDate`, `MonthsInYear`, `MonthsBetweenYears` | Temporal's non-ISO calendars, as ICU4X's `icu_calendar` 2.2.1 reckons them (see 8c): part of the `temporal` package |

The formatter surfaces come first: they are go-intl's own API, and each item
is small beside the areas below them. Time zones and calendar arithmetic are
the largest pieces left, and they are Temporal's as much as Intl's.

Anything without corpus cases needs what DisplayNames and the Collator
needed: expectations taken from node, recorded in `testdata`, where the corpus
does not reach.

### Canonicalization

`Canonicalizer` gives a tag the form `Intl.getCanonicalLocales` gives it,
as V8 and ICU 78.3 give it: V8's check of the language identifier, then the
legacy and redundant tags ICU's parser rewrites first ("zh-hakka" to
"hak", "sgn-no" to "nsl", from uloc_tag.cpp, vendored), then ICU's strict
parse, then ICU's AliasReplacer over CLDR's language, region, script,
variant and subdivision aliases, and every spelling of a Unicode extension
type written as its BCP 47 id (`data/aliases.bin`, from ICU's metadata and
keyTypeData, by `aliasgen`). A replacement fills only the fields a tag
leaves empty, so "cnr-BA" is "sr-BA"; a region that split becomes the one
the language most likely means, "hy-SU" "hy-AM"; the extensions are
written in singleton order. `testdata/canonical_node.js` records every
alias ICU knows in a few surroundings, every extension type and test262's
tags: all 11,139 match.

One divergence, in the compatibility profile: V8 answers two lowercase
letters alone without consulting ICU, so "bh" stays "bh" where the standard
makes it "bho".

### The POSIX variant

ICU reads "-u-va-posix" as the locale's variant POSIX, not as a keyword, and
keeps data under one variant alone: `en_US_POSIX`, whose collation sorts in
ASCII order, whose numbers are written without grouping ("0.######") and
infinity as "INF", and whose words break at colons. So a `DataLocale` holds
that one variant (`Locale.Data` sets it from the keyword or the bare
variant), the fallback chains drop it first (`en-US-posix`, `en-US`, `en`,
the root; `en-posix` to `en`), `collgen` writes the POSIX tailoring the
export has, and `numbergen` writes `en-US-posix` as en-US with
`en_US_POSIX`'s own patterns and marks from ICU's sources, cldr-json having
no such locale. The segmenter's special case gave way to the chain.

V8's ResolveLocale rebuilds the extension from the keywords a service uses,
but ICU's variant is not a keyword, so the resolved locale keeps
"-u-va-posix" in every service: `en-US-u-va-posix` for NumberFormat,
`en-u-va-posix` for PluralRules, which has no en-US. `withKeywords` keeps it
whatever else is dropped. That pass also found services keeping keywords V8
drops: PluralRules, ListFormat and DisplayNames keep none now, and
DateTimeFormat only "ca", "hc" and "nu" -- a "-u-rg-" that DateTimeFormat
had been honouring, and that Node's does not. LocaleInfo's hour cycles,
which do honour it, now come from the region's time data rather than from a
DateTimeFormat. `testdata/variant_node.js` records all nine services with
eleven tags: all 242 match.

Two NumberFormat bugs it turned up: a pattern without grouping is grouped by
threes under `useGrouping: "always"`, as ICU's Grouper does, and the unit
"percent", short or narrow and not compact, is written with the percent
pattern, unscaled, its sign typed as the unit ("0,5 %" in German, "%5" in
Turkish), as ICU does. `useGrouping: "min2"` was missing and is added.
`number_decimal_node.js` now records both: 10,824 cases, all match.

### Negotiation

`LocaleMatcher` is ECMA-402's ResolveLocale and SupportedLocales for one
service, over that service's available locales as V8 builds them from
ICU's (`data/available.bin`, by `availgen`): ICU's installed locales, as
its build indexes them, kept where the bundle or its language's holds what
the service reads ("NumberElements", "calendar", "listPattern"), each also
without its script; the Collator's from the collation tree, PluralRules'
from plurals.txt. 21 of CLDR's locales have no ICU data and so are not
available: "az-Arab" resolves as "az", "ht" as the default.
`testdata/available_node.js` checks every name ICU has a bundle for, cut
short and without its script, in nine services: all 9,747 agree.

"Best fit" matches by lookup, as V8's does: its LocaleMatcher-based best
fit is behind a flag Node leaves off, and across every available locale,
alone and in pairs, Node's two matchers answer alike. With negotiation the
DurationFormat sweep's 21 locale gaps close.

Data is read along the chain ICU reads, per tree of ICU's data:
`Fallbacker.ChainIn` resolves a locale as `ures_open` does, in ICU's own
index of the tree (`data/icutree-<tree>.bin`: its bundles, aliases and
parents) and, for a bundle that does not exist, ICU's default scripts and
parent locales (`data/icufallback.bin`), all written by `availgen`. ICU
opens an alias bundle as the one it names and otherwise drops a default
script, where CLDR's chain truncates: "zh-TW" had been written in
simplified Chinese from "zh", and is now "zh-Hant-TW"'s traditional; "sr-ME"
is Serbian in Latin and "uz-AF" Uzbek in Arabic; "sr-Cyrl-ME" keeps its
Cyrillic dates and names but writes Latin units and currency names, ICU's
unit and currency trees having no bundle of its own; and "az-Arab", which
ICU has no data for, is the root's, as it is for a service that is handed
it without negotiation. The collator reads the collation tree the same way.
Zone names are the exception: zonegen already resolves ICU's zone and
region trees for each locale it writes, so only a locale without a file is
resolved at run time. The index is read by the resolution that needs it,
one tree's file of about 10 KB, so building a formatter costs no more than
it did.

The root's data had never been read: a root data locale opened the
marker's shared file, `dates.bin`, which a per-locale marker has none of,
so a chain that ran out of locales failed rather than ending at the root.
It now opens `<marker>/und.bin`, and "und", "und-TW" and the like format.

### Locale info

`LocaleInfo` answers what `Intl.Locale` says beyond a locale's subtags, as
V8 answers it from ICU (js-locale.cc): `Maximize` and `Minimize`; the
calendars the region reckons in, most preferred first (calendarprefs.bin
now keeps the whole list); the collations along the collation tree's
chain, in BCP 47's spelling; the pattern generator's default hour cycle;
the default numbering system; the canonical zones of the region subtag,
SystemV's among them for 001, as ICU counts a zone canonical when it is
neither an alias nor a tz link; the direction of the locale's, or its
likely, script, from ICU's script properties (uscript_props.cpp, vendored)
and uloc_isRightToLeft's shortcut for common languages; and the first day
of the week and the weekend. Each takes its keyword ("-u-ca-", "-u-co-",
"-u-hc-", "-u-nu-", "-u-fw-") where one is given, and the region for
supplemental data as ICU finds it: "-u-rg-", the region, "-u-sd-", the
likely region. `Minimize` had tried the request's own region and script
rather than the maximized ones; it now gives "zh-TW" for "zh-Hant", as ICU
does. `testdata/localeinfo_node.js` records 690 locales: all nine answers
match for every one.

### Supported values

`Calendars`, `Collations`, `Currencies`, `NumberingSystems`, `TimeZones`
and `SanctionedUnits` are the lists `Intl.supportedValuesOf` answers with.
The collations, currencies and time zones are V8's, built from ICU's
(`data/values.bin`, by `availgen`): every collation the collation tree
names, in BCP 47 spelling, but "standard" and "search"; ICU's common,
current ISO currencies, a list compiled into ucurr.cpp (vendored), with an
English name, V8's four additions and without VEF; and ICU's canonical
zones in a region. `testdata/values_node.js` records all six: identical,
order included.

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
