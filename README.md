# go-intl

A pure-Go implementation of ECMA-402, the JavaScript internationalization API:
number, date, list, plural, relative-time, duration and display-name
formatting, collation, segmentation, normalization, locale negotiation and
canonicalization, for every locale ICU has data for. It is built to give the
same answers as ICU 78.3 as Node 26 ships it, and to be usable by any Go
program, not only a JavaScript engine.

It is the base of `Intl`, `Date` and `Temporal` in the
[go-quickjs](https://github.com/go-quickjs/go-quickjs) engine. There is no
cgo, no WebAssembly and no C compiler: it builds wherever Go does.

> **Status: v0.** Every service is done and held to Node. Releases are v0.x,
> and the API may still change between minor versions.

```sh
go get github.com/go-quickjs/go-intl
```

```go
import intl "github.com/go-quickjs/go-intl"

loc, _ := intl.ParseLocale("de-DE")

nf, _ := intl.NewNumberFormat(loc, intl.NumberFormatOptions{
	Style: intl.StyleCurrency, Currency: "EUR",
})
nf.Format(1234.5)       // "1.234,50 €"
nf.FormatRange(5, 10)   // "5,00–10,00 €"

df, _ := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
	TimeZone: "Europe/Berlin", DateStyle: intl.LengthLong, TimeStyle: intl.LengthShort,
})
df.Format(time.Date(2024, 1, 5, 14, 4, 5, 0, time.UTC)) // "5. Januar 2024 um 15:04"

pr, _ := intl.NewPluralRules(loc, intl.PluralRulesOptions{})
pr.Select(1) // "one"

col, _ := intl.NewCollator(loc, intl.CollatorOptions{})
col.Compare("Äpfel", "Birnen") // -1
```

This example is [`example_test.go`](example_test.go), so it is compiled and
checked with the tests. The spaces before "€" are CLDR's no-break spaces.

## Services

| Service | What it covers |
|---|---|
| `NumberFormat` | Every style, unit, notation, rounding option and numbering system, exact decimal input (`ParseDecimal`, `FormatDecimal`), ranges |
| `DateTimeFormat` | All eighteen calendars Node supports -- Gregorian, Buddhist, Persian, Coptic, Ethiopic (both eras), Indian, the five Islamic ones, ROC, Hebrew, Japanese, ISO 8601, Chinese, Dangi -- with styles, fields, hour cycles, day periods, fractional seconds, every `timeZoneName` style, offset zones, ranges, and Temporal values (`ForTemporal`) |
| `PluralRules` | Cardinal and ordinal, with the digit options, and `SelectRange` |
| `Collator` | Every tailoring ICU has, Korean `searchjl` and POSIX among them, sensitivity, numeric ordering, case order |
| `Segmenter` | Graphemes, words and sentences, with ICU's rules and the dictionaries for Thai, Lao, Khmer, Burmese, Chinese and Japanese |
| `ListFormat`, `RelativeTimeFormat`, `DisplayNames`, `DurationFormat` | Every option |
| `Canonicalizer`, `LocaleMatcher` | `Intl.getCanonicalLocales`, and each service's available locales and locale negotiation, as V8 builds them from ICU's |
| `LocaleInfo` | What `Intl.Locale` reports beyond its subtags: likely subtags, calendars, collations, hour cycles, numbering system, time zones, text direction, week |
| `TimeZone` | ICU's zoneinfo64 (tz 2026c), the host's zone as ICU detects it, and the zone names formatting writes |
| `Normalizer` | NFC, NFD, NFKC, NFKD, at Unicode 17.0.0 |
| `Calendars`, `Currencies`, ... | The lists `Intl.supportedValuesOf` returns |

Every formatter has `ToParts` where ECMA-402 does, and a `ResolvedOptions`.

Two packages beside it hold what a JavaScript engine needs apart from `Intl`:

- [`date`](date) is JavaScript's `Date`: ECMA-262's time arithmetic, local
  time as V8 reads it from ICU's zones, `Date.parse`, and the strings `Date`
  writes.
- [`temporal`](temporal) is Temporal's arithmetic, ported from the Rust crates
  Node builds Temporal from (`temporal_rs` 0.2.3 over ICU4X's `icu_calendar`
  2.2.1): the calendars, dates and times, durations with rounding, zoned time
  and parsing. An engine keeps the objects and calls in.

## How it is held to Node

The answers are checked, not assumed:

- **A golden corpus** of 7,949 formatting calls taken from Node, run by
  `go test`: all 7,949 match exactly.
- **Recordings of Node** over every locale it supports, made by the scripts in
  [`testdata`](testdata) and replayed by `go test`: more than two million
  cases across dates and ranges in every calendar, zone names, Temporal
  values, numbers, number ranges and exact decimals, plural ranges,
  collation orders, segmentation, normalization, numbering systems,
  durations, canonicalization, negotiation, `Date` in every zone Node knows,
  and Temporal's calendars and operations. All of them match.

Where Node and the standard disagree, the difference is a named divergence in
[`compat.go`](compat.go), tested on both sides. `Compat` chooses them one by
one; its zero value, `Standard`, is ECMA-402 in every one, and `NodeICU` is
Node in every one. There are twenty-two, among them the plain space V8 writes
where ICU writes U+202F, V8's twelve-hour clock for Japanese, the era V8
counts as a field asked for when it writes a Temporal value, and two roundings
the standard fixed after the temporal_rs Node runs.

Where ICU's implementation decides an answer, the implementation is ported:
the pattern generator, the interval and number-range formatters, the zone
name logic, the collation and break iterators are ICU's algorithms, and their
comments name the ICU functions they follow.

## Data

The tables are generated from CLDR 48.2.0, ICU 78.3's sources and data, tz
2026c and Unicode 17.0.0, by generators in [`internal`](internal). They hold
the inputs to the algorithms, never ICU's answers: patterns, names and rules,
not formatted strings. That is the project's one inviolable rule.
[SOURCES.md](SOURCES.md) pins every input by checksum.

The data is embedded, 18.7 MB, with what locales share kept once. It is packed
into one file and read where it lies: a formatter looks up what it needs
rather than decoding tables, so building one is cheap -- in English a
`NumberFormat` in about 8 µs, a `DateTimeFormat` in about 70 µs, a `Collator`
in about 6 µs. There is no cache to warm up, because there is nothing to
cache.

Every constructor has a `...From` variant that takes a `Source`, so a program
can supply the data from a directory, or carry only the locales it needs:

```go
src := intl.NewFS(os.DirFS("/path/to/data"))
nf, err := intl.NewNumberFormatFrom(src, loc, opts)
```

## Design

- A formatter is immutable after construction and safe for concurrent use.
  There is no package-level mutable state and no global cache.
- Constructors return an `error` where ECMA-402 throws.
- Options are structs whose zero value is ECMA-402's default, and every
  option's value prints as JavaScript spells it: `intl.Width2Digit` is
  "2-digit", `intl.HalfExpand` "halfExpand", an option not given
  "undefined".

[DESIGN.md](DESIGN.md) has the architecture and why it is shaped this way.
[AGENTS.md](AGENTS.md) has the working rules for changing it.

## Development

```sh
go test ./...
```

Go 1.24 or later. The data is regenerated in one step, from nothing but the
pinned upstream sources:

```sh
go run ./internal/regen
```

It downloads each source [SOURCES.md](SOURCES.md) pins into a cache, checks
its sha256, runs every generator and repacks the embedded data; on a clean
checkout it reproduces `data/` and `data.pack` byte for byte, and says so.
The first run fetches about 85 MB, which the cache keeps unpacked in about
670 MB, and takes about a minute; later runs reuse the cache and take seconds.

## License

[Unicode License v3](LICENSE), the license ICU and CLDR are released under,
whose data and algorithms this adapts.
