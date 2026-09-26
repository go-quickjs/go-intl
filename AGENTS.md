# AGENTS.md

This file applies to the entire repository.

## Project overview

`go-intl` is a pure-Go implementation of ECMA-402 internationalization. It is
consumed by the go-quickjs JavaScript engine and is meant to be usable by any
Go program on its own.

Read [DESIGN.md](DESIGN.md) for the architecture and why it is shaped this
way; it is authoritative. This file is how to work.

Module path: `github.com/go-quickjs/go-intl`. Targets Go 1.24, matching
go-quickjs, so consuming it never forces a toolchain upgrade.

## Working rules

- Keep it portable: no cgo, no WebAssembly, no C compiler, no platform-specific
  runtime dependencies. Platform differences go behind build tags, and every
  build tag needs its counterpart.
- **Datagen emits inputs to algorithms, never answers.** If a generated table
  holds a string a formatter could have produced, stop and fix the layering.
  This is the project's one inviolable rule.
- A formatter is immutable after construction and safe for concurrent use. No
  package-level mutable state, no lazy global caches, no warm-up entry points.
- Constructors return `error` where ECMA-402 throws. Not `(T, bool)`.
- Every Node/ICU4C divergence is a named entry in the compatibility profile with
  a test for both `Standard` and `NodeICU`. Never an unexplained special case.
- Add a regression test for every bug. Prefer an exact expected value over a
  smoke test.
- Run `gofmt` on every changed Go file.
- Do not weaken a test or a gate to make a suite green. Establish the behavior
  in UTS #35, ECMA-402, or the pinned ICU, and say which.

## Generated files

Do not edit generated files by hand. Everything under `data/`, and
`data.pack`, is written by a generator under `internal/<name>gen/`.

Regenerate all of it in one step, from the repository root:

```sh
go run ./internal/regen
```

`regen` downloads the pinned sources into a cache (`-cache`), checks their
checksums, runs every generator in parallel (`-j`), zonegen after dategen,
repacks, and reports whether the result is byte for byte what git holds. A
change to a generator is done when `regen` reproduces the committed data, or
the difference is the change. A new upstream input goes into its source
list as well as SOURCES.md.

To run one generator on its own, its doc comment gives its arguments; run it
from the repository root. A generator does its own replacing, and builds
every table before writing any of them, so a failure partway leaves a matched
set on disk rather than one new file beside one old one.

The package embeds `data.pack`, the data directory packed into one file so
it is read in place. After running a generator on its own, repack; a test
fails until you do:

```sh
go run ./internal/packgen
```

Generator output is reproducible: running one twice gives byte-identical files.
A generator that sorts a map without fixing the order is broken even when its
tests pass.

Upstream versions are pinned. Changing one is its own commit, with the gate
re-measured, never folded into a behavior change.

## Verification

go-intl's own tests:

```sh
go test ./...
```

The gates that govern whether work may land are measured in go-quickjs, with
go-intl required at the commit under test. Each is a floor: nothing lands that
lowers one.

| Gate | Floor |
|---|---|
| test262, all of it | 93,010 pass, 0 fail, 5,550 skip (98,560 variants, test262 `045bf6f9`) |
| The golden corpus through the engine | 7,948 of 7,949, the one named |
| `go test ./...` in go-quickjs | green |

```sh
cd ../go-quickjs
TEST262_DIR=/path/to/test262 go test ./conformance -run TestConformance \
  -conformance.max-failures=0 -timeout=90m
go test ./...
```

For a change to dates, zones or Temporal, compare with Node across every
locale and zone as well (about ten minutes, in parallel):

```sh
QUICKJS_COMPARE_NODE_TEMPORAL=1 go test . \
  -run '^TestTemporalMatchesNodeAcrossLocalesAndTimeZones$' -count=1 -timeout=60m
```

Re-measure before changing anything, and on both Linux and Windows - the
platforms disagree, and a bug has already hidden on one of them.

## Commits

- Explain the behavior and why it changed, not the diff.
- A new divergence updates the count in DESIGN.md and README.md with it.
- Branch rather than committing to the default branch.
- Do not push unless asked.
