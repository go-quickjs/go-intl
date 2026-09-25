# go-intl

A pure-Go implementation of ECMA-402, the JavaScript internationalization API:
number, date, list, plural, relative-time and display-name formatting,
collation and normalization, for every CLDR locale. It is built to give the
same answers as ICU 78.3 as Node ships it, and to be usable by any Go program,
not only a JavaScript engine.

It is the base the [go-quickjs](https://github.com/go-quickjs/go-quickjs)
engine will build `Intl` on. There is no cgo, no WebAssembly and no C
compiler: it builds wherever Go does.

> **Status: pre-release.** The API may still change. go-quickjs does not use it
> yet. See [PLAN.md](PLAN.md) for what is done and what is left.

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

| Service | State |
|---|---|
| `NumberFormat` | Complete: every style, notation, rounding option and numbering system, exact decimal input (`ParseDecimal`, `FormatDecimal`), ranges |
| `DateTimeFormat` | Complete for the Gregorian, Buddhist and Persian calendars: styles, fields, hour cycles, day periods, fractional seconds, every `timeZoneName` style, offset time zones, ranges. The other 14 calendars are to come |
| `PluralRules` | Complete, with `SelectRange` |
| `Collator` | Complete but for Korean `searchjl`: every other tailoring, sensitivity, numeric ordering, case order |
| `ListFormat`, `RelativeTimeFormat`, `DisplayNames` | Complete |
| `Normalizer` | NFC, NFD, NFKC, NFKD, at Unicode 17.0.0 |
| `Segmenter`, `DurationFormat` | Not started |

Every service has `ToParts` where ECMA-402 does, and a `ResolvedOptions`.

## How it is held to ICU

The answers are checked, not assumed:

- **A golden corpus** of 7,949 cases taken from Node, run by `go test`.
- **Sweeps against Node** over every locale Node supports, recorded under
  [`testdata`](testdata) by the scripts beside them: about 515,000 cases across
  dates, date ranges, calendars, zone names, numbering systems, number
  ranges, exact decimals, plural ranges and selection, and collation orders.
  Every one matches except the named gaps: the calendars not yet
  implemented, Dublin's zone names (Go's time-zone data differs from ICU's
  for Ireland) and Korean `searchjl` collation. A gap that
  starts passing fails its test, so the entry gets removed.

Where Node and ECMA-402 disagree, the difference is a named entry in
[`compat.go`](compat.go). The zero value of `Compat` is the standard, and
`NodeICU` gives Node's behavior. There are two so far: V8 writes a plain
space before AM and PM where ICU writes U+202F, and V8 adds a collation
chosen by option to the resolved locale.

Where ICU's implementation decides an answer, the implementation is ported.
The pattern generator, the interval formatter, the number-range formatter
and the time-zone name logic are ICU's algorithms, and their comments name the
ICU functions they follow.

## Data

The tables are generated from CLDR 48.2.0 and the ICU 78.3 release by
generators in [`internal`](internal). They hold CLDR's inputs, never ICU's
answers: patterns, names and rules, not formatted strings. That is the
project's one inviolable rule. [SOURCES.md](SOURCES.md) pins every input by
checksum.

The data is embedded with `go:embed`, about 52 MB, not yet compressed. Every
constructor has a `...From` variant that takes a `Source`, so a program can
supply data from a directory or carry only the locales it needs:

```go
src := intl.NewFS(os.DirFS("/path/to/data"))
nf, err := intl.NewNumberFormatFrom(src, loc, opts)
```

## Design

- A formatter is immutable after construction and safe for concurrent use.
  There is no package-level mutable state and no global cache.
- Constructors return an `error` where ECMA-402 throws.
- Options are structs whose zero value is ECMA-402's default.

[DESIGN.md](DESIGN.md) has the architecture and why it is shaped this way.
[AGENTS.md](AGENTS.md) has the working rules for changing it.

## Development

```sh
go test ./...
```

Go 1.24 or later. Regenerating the data needs the pinned upstream archives
named in [SOURCES.md](SOURCES.md). Each generator's doc comment gives its
command.

## License

[Unicode License v3](LICENSE), the license ICU and CLDR are released under,
whose data and algorithms this adapts.
