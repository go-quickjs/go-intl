# AGENTS.md

This file applies to the entire repository.

## Project overview

`go-intl` is a pure-Go implementation of ECMA-402 internationalization. It is
consumed by the go-quickjs JavaScript engine and is meant to be usable by any
Go program on its own.

Read [DESIGN.md](DESIGN.md) for the architecture and [PLAN.md](PLAN.md) for the
staging and current state. Both are authoritative; this file is how to work.

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

Do not edit generated files by hand. Each has a generator under
`internal/<name>gen/`, and each generated file's header names the upstream
release it came from.

Run generators from the repository root, writing to a temporary file first so a
failed run cannot truncate the tracked one:

```sh
generated=$(mktemp /tmp/go-intl.XXXXXX.go)
go run ./internal/<name>gen > "$generated" && mv "$generated" <target>.go
gofmt -w <target>.go
```

A generator that writes binary tables does its own replacing, and builds every
table before writing any of them, so a failure partway leaves a matched set on
disk rather than one new file beside one old one:

```sh
go run ./internal/localegen     # writes data/likelysubtags.bin, data/parentlocales.bin
```

The package embeds `data.pack`, the data directory packed into one file so
it is read in place. After any generator, repack; a test fails until you do:

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

The gates that govern whether work may land are measured in go-quickjs. See
PLAN.md for the current floors and the exact commands. Re-measure before
changing anything, and on both Linux and Windows - the platforms disagree, and
a bug has already hidden on one of them.

## Commits

- Explain the behavior and why it changed, not the diff.
- Update PLAN.md's Status table in the same commit as the work it describes.
- Branch rather than committing to the default branch.
- Do not push unless asked.
