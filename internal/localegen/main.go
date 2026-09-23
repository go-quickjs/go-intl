// Command localegen writes the locale tables the intl package carries.
//
// Two of them, both from CLDR's supplemental data, vendored beside this
// command:
//
//   - the likely subtags, which say what an identifier that leaves out a
//     script or a region most probably meant, so that "zh-TW" and
//     "zh-Hant-TW" are looked for in one place;
//   - the parent locales, which redirect a fallback that truncation would send
//     somewhere wrong. "zh-Hant" must not fall back through "zh": traditional
//     Chinese inheriting from simplified is worse than inheriting from the
//     root.
//
// Usage, from the repository root:
//
//	go run ./internal/localegen
//
// It writes data/likelysubtags.bin and data/parentlocales.bin, replacing them
// only once both have been built, so a failed run leaves the tracked files
// alone.
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

	intl "github.com/go-quickjs/go-intl"
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

	// Both are built before either is written, so a failure partway leaves a
	// matched pair on disk rather than one new table and one old one.
	if err := write("likelysubtags.bin", likelyTable); err != nil {
		return err
	}
	if err := write("parentlocales.bin", parentTable); err != nil {
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

	out := make([]byte, 0, len(records)*2*intl.DataLocaleSize)
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

func write(name string, data []byte) error {
	target := filepath.Join("data", name)
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}
