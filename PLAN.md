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
3. A service switches over in go-quickjs only when its slice of the corpus is
   **at or above** what `internal/icu` scores, and all four gates hold.
4. If a switch regresses anything, it does not land. The old path stays.

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
has no unverified pins left.

### 1. Locale

`Language`, `Script`, `Region`, `Variant` as fixed-size comparable types.
Parser, canonicalization, likely-subtags, and the fallback chain. No data
dependency beyond likely-subtags, so it can land standalone. ICU4X's
`locale_fallback` is ~1,000 lines and worth following closely.

*Gate:* a locale corpus - parse, canonicalize, fallback - passes.

### 2. Provider seam and datagen skeleton

The `Source` interface plus the embedded implementation. The datagen command
that reads `cldr-json` and writes the Model encoding. This is the highest-
leverage stage in the project: it is what makes the library reusable.

*Gate:* round-trip test - datagen writes, `Source` reads, the bytes agree.

### 3. NumberFormat

The first full vertical slice: datagen for `cldr-numbers-full`, the algorithm,
`FormatToParts`, and both `Standard` and `NodeICU` profiles. `cldr-numbers-full`
is clean self-contained JSON, which is why this service goes first.

*Gate:* the number slice of the golden corpus at or above `internal/icu`.
**This stage is the architecture proof.** If the layering does not hold here it
will not hold anywhere, and it is cheap to change now.

### 4. PluralRules and ListFormat

Small, data-clean, and the two services ICU4X's own ECMA-402 layer bothered to
bind. Confirms the shape repeats.

*Gate:* their corpus slices; first go-quickjs switch-over for all three services
so far.

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
| 0. Bootstrap | not started |
| 1. Locale | not started |
| 2. Provider and datagen | not started |
| 3. NumberFormat | not started |
| 4. PluralRules, ListFormat | not started |
| 5. DateTimeFormat | not started |
| 6. RelativeTime, DisplayNames, Duration | not started |
| 7. Collator | not started |
| 8. Segmenter | not started |
| 9. Retire internal/icu | not started |

Switched over in go-quickjs: *none yet.*

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
