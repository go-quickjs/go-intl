// Package datawrite writes the data sets kept per locale, for every
// generator that writes one.
//
// CLDR's locales are written resolved, each with everything it inherits,
// so many are byte for byte another's: en-AG writes what en-001 writes.
// Such data is written once. A directory holds <tag>.bin for each locale
// whose data is its own, and same.bin, a table of each other locale and
// the one whose file it shares, which the Source follows. Nothing reading
// the data sees the difference.
package datawrite

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	intl "github.com/go-quickjs/go-intl"
)

// SameFile is the table of locales whose data is another's.
const SameFile = "same.bin"

// Locales replaces a data set kept per locale with files, which are keyed
// by the tag each is written under ("und" for the root). A file that is
// another's byte for byte is not written; it is named in same.bin instead,
// against the locale that holds it: the root where the root is one of
// them, else the shortest tag, then the first in order. A locale with a
// variant, which same.bin cannot hold, is always written.
//
// A locale with a variant a data locale does not hold -- CLDR's be-tarask --
// is left out: nothing could read it. ICU has no data under it, and Node
// writes it as the locale without the variant, which the fallback does.
//
// Everything is built before anything is replaced, and each file goes
// through a temporary one, so a failure leaves the old set.
func Locales(dir string, files map[string][]byte) error {
	tags := make([]string, 0, len(files))
	for tag := range files {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	locales := map[string]intl.DataLocale{}
	kept := tags[:0]
	for _, tag := range tags {
		l, err := intl.ParseLocale(tag)
		if err != nil {
			return fmt.Errorf("%s: %w", tag, err)
		}
		d := l.Data()
		if len(l.Variants) > 0 && d.Variant.IsZero() {
			continue
		}
		if d.String() != l.String() && !(tag == "und" && d.IsRoot()) {
			return fmt.Errorf("%s is more than a data locale", tag)
		}
		locales[tag] = d
		kept = append(kept, tag)
	}
	tags = kept

	groups := map[[32]byte][]string{}
	for _, tag := range tags {
		if !locales[tag].Variant.IsZero() {
			continue
		}
		h := sha256.Sum256(files[tag])
		groups[h] = append(groups[h], tag)
	}
	canonical := map[string]string{}
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			a, b := group[i], group[j]
			switch {
			case a == "und" || b == "und":
				return a == "und"
			case len(a) != len(b):
				return len(a) < len(b)
			}
			return a < b
		})
		for _, tag := range group[1:] {
			canonical[tag] = group[0]
		}
	}

	type pair struct{ key, value []byte }
	var pairs []pair
	for tag, to := range canonical {
		key, _ := locales[tag].MarshalBinary()
		value, _ := locales[to].MarshalBinary()
		pairs = append(pairs, pair{key, value})
	}
	sort.Slice(pairs, func(i, j int) bool { return string(pairs[i].key) < string(pairs[j].key) })
	var same []byte
	for i, p := range pairs {
		if i > 0 && string(p.key) == string(pairs[i-1].key) {
			return fmt.Errorf("two locales share the written form %x", p.key)
		}
		same = append(same, p.key...)
		same = append(same, p.value...)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "*.bin"))
	for _, name := range old {
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	for _, tag := range tags {
		if _, shared := canonical[tag]; shared {
			continue
		}
		if err := write(filepath.Join(dir, tag+".bin"), files[tag]); err != nil {
			return err
		}
	}
	if len(same) > 0 {
		if err := write(filepath.Join(dir, SameFile), same); err != nil {
			return err
		}
	}
	return nil
}

// Tags lists the locales a directory written by Locales has data for, its
// own or another's, as the tags they were written under.
func Tags(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.bin"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		if filepath.Base(p) == SameFile {
			continue
		}
		out = append(out, strings.TrimSuffix(filepath.Base(p), ".bin"))
	}
	b, err := os.ReadFile(filepath.Join(dir, SameFile))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	const pair = 2 * intl.DataLocaleSize
	if len(b)%pair != 0 {
		return nil, fmt.Errorf("%s is %d bytes, not whole records", SameFile, len(b))
	}
	for i := 0; i < len(b); i += pair {
		var d intl.DataLocale
		if err := d.UnmarshalBinary(b[i : i+intl.DataLocaleSize]); err != nil {
			return nil, err
		}
		tag := d.String()
		if d.IsRoot() {
			tag = "und"
		}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out, nil
}

func write(target string, data []byte) error {
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, target)
}
