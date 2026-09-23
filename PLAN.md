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

go-intl targets **ICU 78.3** - CLDR 48.0, Unicode 17.0 - because that is what
produced the golden corpus. Generating from a newer CLDR than the oracle was
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
- `style: "unit"` with the unit identifiers and their patterns, which needs
  `cldr-units-full`.
- `currencyDisplay: "name"`, which needs plural rules and the currency display
  names.

- ~~the digit options `PluralRules` is also given~~ **done**; `selectRange` is
  still missing.

The rounding was rewritten to do its work on the digits rather than on the
float. Scaling a float by a power of ten to round it introduces error of its
own, and that error lands exactly on the ties the mode exists to decide.

Every expectation in the rounding tests was checked against node rather than
against a reading of the specification, and all of them agreed first time:
nine rounding modes, significant digits, both priorities, `trailingZeroDisplay`,
`roundingIncrement` and the accounting sign.

*Gate:* every option `internal/icu` accepts is accepted here, and the corpus
stays at 2,970/2,970. **Corpus held; the surface is not complete yet.**

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

**Prerequisite, and it is not optional:** replace the normalizer's data source
first. go-quickjs's tables are Unicode 13.0.0, taken from whatever UCD the
system's Perl shipped, while the anchor is 17.0. UCA runs on NFD, so the
Collator sits directly on that table and every character changed between those
versions is a sorting divergence waiting to surface in the hardest stage. See
SOURCES.md.

*Gate:* collation order across the corpus's locales, including the CJK
tailorings `internal/icu` carries.

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
| 3c. Rest of the surface | rounding, notation, plural digits done; unit and currency names left |
| 4. PluralRules, ListFormat | **done** - 300/300 and 120/120 |
| 5. DateTimeFormat | not started |
| 6. RelativeTime, DisplayNames, Duration | not started |
| 7. Collator | not started |
| 8. Segmenter | not started |
| 9. Retire internal/icu | not started |

**Corpus coverage so far: 3,390 of 7,949 cases, all at 100%.**

| Service | Cases | Matching |
|---|---|---|
| NumberFormat | 2,970 | 2,970 |
| PluralRules | 300 | 300 |
| ListFormat | 120 | 120 |

Switched over in go-quickjs: *none yet.* Three services are now at parity, so
the first switch-over is due.

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
