// Command localegen writes the locale tables the intl package carries.
//
// Three of them, two from CLDR's supplemental data, vendored beside this
// command:
//
//   - the likely subtags, which say what an identifier that leaves out a
//     script or a region most probably meant, so that "zh-TW" and
//     "zh-Hant-TW" are looked for in one place;
//   - the parent locales, which redirect a fallback that truncation would send
//     somewhere wrong. "zh-Hant" must not fall back through "zh": traditional
//     Chinese inheriting from simplified is worse than inheriting from the
//     root;
//   - the regions a "-u-rg-" or "-u-sd-" keyword may name, which ICU
//     compiles into loclikely.cpp rather than reading from CLDR, read
//     through internal/icusrc.
//
// Usage, from the repository root:
//
//	go run ./internal/localegen
//
// It writes data/likelysubtags.bin, data/parentlocales.bin and
// data/validregions.bin, replacing them only once all have been built, so a
// failed run leaves the tracked files alone.
//
// Each table is records of two data locales, sorted by the first, which the
// intl package binary-searches where it lies. The layout is that package's
// own: this command marshals through it rather than spelling the bytes out, so
// the writer and the reader cannot drift apart.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/layout"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

//go:embed likelySubtags.json
var likelySubtagsJSON []byte

//go:embed parentLocales.json
var parentLocalesJSON []byte

type likelyResource struct {
	Supplemental struct {
		Version struct {
			CLDR string `json:"_cldrVersion"`
		} `json:"version"`
		LikelySubtags map[string]string `json:"likelySubtags"`
	} `json:"supplemental"`
}

type parentResource struct {
	Supplemental struct {
		Version struct {
			CLDR string `json:"_cldrVersion"`
		} `json:"version"`
		ParentLocales struct {
			// CLDR also keeps parents that apply only to collation, to
			// segmentation and to a few other things. Only the general ones
			// are taken here; the rest belong with the services that use them.
			ParentLocale map[string]string `json:"parentLocale"`
		} `json:"parentLocales"`
	} `json:"supplemental"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "localegen:", err)
		os.Exit(1)
	}
}

func run() error {
	var likely likelyResource
	if err := json.Unmarshal(likelySubtagsJSON, &likely); err != nil {
		return fmt.Errorf("reading likelySubtags.json: %w", err)
	}
	var parents parentResource
	if err := json.Unmarshal(parentLocalesJSON, &parents); err != nil {
		return fmt.Errorf("reading parentLocales.json: %w", err)
	}
	if a, b := likely.Supplemental.Version.CLDR, parents.Supplemental.Version.CLDR; a != b {
		return fmt.Errorf("the two inputs are CLDR %s and CLDR %s", a, b)
	}

	likelyTable, err := build(likely.Supplemental.LikelySubtags)
	if err != nil {
		return fmt.Errorf("the likely subtags: %w", err)
	}
	parentTable, err := build(parents.Supplemental.ParentLocales.ParentLocale)
	if err != nil {
		return fmt.Errorf("the parent locales: %w", err)
	}
	regions, err := icusrc.ValidRegions()
	if err != nil {
		return err
	}
	// A line each, sorted, which the reader binary-searches.
	regionTable := []byte(strings.Join(regions, "\n") + "\n")

	// All are built before any is written, and replaced together, so a
	// failure leaves a matched set on disk rather than one new table beside
	// an old one.
	set := writeset.New()
	set.File(filepath.Join("data", "likelysubtags.bin"), likelyTable)
	set.File(filepath.Join("data", "parentlocales.bin"), parentTable)
	set.File(filepath.Join("data", "validregions.bin"), regionTable)
	if err := set.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr,
		"localegen: CLDR %s, %d likely subtags, %d parent locales\n",
		likely.Supplemental.Version.CLDR,
		len(likely.Supplemental.LikelySubtags),
		len(parents.Supplemental.ParentLocales.ParentLocale))
	return nil
}

// build turns a mapping of tags into the sorted records the reader searches.
func build(in map[string]string) ([]byte, error) {
	type record struct{ key, value intl.DataLocale }
	records := make([]record, 0, len(in))
	for from, to := range in {
		key, err := dataLocale(from)
		if err != nil {
			return nil, err
		}
		value, err := dataLocale(to)
		if err != nil {
			return nil, err
		}
		records = append(records, record{key, value})
	}

	// Sorted by the written form of the key, since that is the order the
	// binary search walks.
	keyed := make(map[intl.DataLocale][]byte, len(records))
	for _, r := range records {
		b, err := r.key.MarshalBinary()
		if err != nil {
			return nil, err
		}
		keyed[r.key] = b
	}
	sort.Slice(records, func(i, j int) bool {
		return string(keyed[records[i].key]) < string(keyed[records[j].key])
	})

	out := make([]byte, 1, 1+len(records)*2*intl.DataLocaleSize)
	out[0] = layout.Pairs
	var last []byte
	for _, r := range records {
		key := keyed[r.key]
		if last != nil && string(key) == string(last) {
			return nil, fmt.Errorf("%s is given twice", r.key)
		}
		last = key
		var err error
		if out, err = r.key.AppendBinary(out); err != nil {
			return nil, err
		}
		if out, err = r.value.AppendBinary(out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// dataLocale reads a tag as CLDR writes it. CLDR uses "root" and "und" for the
// same thing and the parser settles that, but a tag carrying anything beyond a
// language, a script and a region has no place in either table.
func dataLocale(tag string) (intl.DataLocale, error) {
	l, err := intl.ParseLocale(tag)
	if err != nil {
		return intl.DataLocale{}, err
	}
	if len(l.Variants) > 0 || len(l.Keywords) > 0 || len(l.Extensions) > 0 {
		return intl.DataLocale{}, fmt.Errorf("%q is more than a data locale", tag)
	}
	return l.Data(), nil
}
