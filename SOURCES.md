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

Generating from a newer CLDR than the oracle was built with makes some corpus
differences genuine upstream drift rather than bugs, which is time spent
chasing nothing. Both move together, deliberately, or neither moves.

Note this differs from ICU4X's own pins (CLDR `49.0.0-ALPHA2`, icuexport
`icu4x/2026-08-31/79.x`). ICU4X is an architectural reference, not a version
authority.

## Pins

| Source | Pin | Feeds | Confirmed |
|---|---|---|---|
| ICU | 78.3 | the anchor; the golden corpus oracle | yes, from node |
| CLDR (`cldr-json`) | 48.0 | numbers, dates, units, names, plurals, zones | yes, from node |
| Unicode (UCD) | 17.0 | properties, normalization | yes, from node |
| IANA tzdb | 2026b | zone rules | yes, from node — but see below |
| `icuexportdata` | ICU 78.x export tag | UCA, collation tailorings, normalizer | **not yet** |
| Break dictionaries | ICU `release-78.3` | Chinese, Japanese, Thai, Khmer, Lao, Burmese | yes |
| LSTM models | `v0.1.0` | Thai, Khmer, Lao, Burmese word breaking | not version-tied to ICU |

URLs follow the patterns ICU4X uses:

- `https://github.com/unicode-org/cldr-json/releases/download/{tag}/cldr-{tag}-json-full.zip`
- `https://github.com/unicode-org/icu/releases` — the `icu4x/{date}/{major}.x` artifacts
- `https://github.com/unicode-org/lstm_word_segmentation/releases`

## Open

**The `icuexportdata` tag for ICU 78.x is unverified.** ICU4X pins a 79.x
export; whether a matching 78.x artifact is published needs checking against the
releases page. If none exists, the choice is between generating collation data
from CLDR's collation XML plus `allkeys_CLDR.txt` directly, or accepting a 79.x
export against a 78.3 oracle and recording which differences that explains.
Settle this in stage 0, before stage 7 depends on it.

**tzdb is 2026b in ICU 78.3, but go-quickjs bundles 2026c.** Its
`internal/icu/timezones.go` calls the bundle "the same version used by the
Node/ICU release", which is no longer accurate; `TestBundledTimeZoneMatchesICURelease`
does not actually check the version, only that every zone loads and one
Vancouver rule holds. Nothing currently fails because of it. go-intl should pin
deliberately rather than inherit the drift.

## Known defects in the data go-intl inherits

**Normalization is Unicode 13.0.0 while everything else is 17.0.** go-quickjs's
`internal/normalize/tables.go` says it was generated "from the Unicode 13.0.0
database, as shipped with the system's Perl" — an accidental input, four
versions behind the `\p{...}` property tables in the same binary, which are at
17.0.0.

This is not cosmetic for go-intl: **UCA requires NFD**, so the Collator sits
directly on this table. Every character added or recomposed between Unicode 13
and 17 is a latent sorting divergence that would surface late, inside the
hardest stage. Replace the normalizer's data source before stage 7 starts.

**Vendored `windowsZones.json` was taken from `cldr-json` `main`, not a tagged
release.** Its content self-reports CLDR 48, which is correct for the anchor,
but the provenance is unpinned. Re-fetch it from the CLDR 48.0 tag when that
generator is brought over.
