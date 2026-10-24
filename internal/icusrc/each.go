package icusrc

import (
	"bytes"
	"sort"
	"strings"
)

// EachLocale builds a data set kept per locale from a tree's bundles: build
// makes one locale's file from its resolved view, or nothing, and the
// result is keyed by the locale's tag, "und" for the root, as datawrite
// takes it.
//
// A locale is a bundle of the tree that is not an alias of another: an
// alias bundle (in, for id) is read as the bundle it names, and a tag is
// canonicalized before its data is looked up, so its own name is never
// asked for. An empty bundle (af_ZA), which is there so that ICU lists the
// locale as installed, is a locale like any other, whose file is its
// parent's and is named in same.bin. A bundle with a variant
// (ca_ES_VALENCIA) is kept only where its file differs from the one of
// the locale without the variant, which is what a variant locale without
// data of its own reads. One whose variant BCP 47 cannot spell -- ICU 74's
// ja_JP_TRADITIONAL, which only ICU's own locale names reach -- is never
// asked for, and is left out.
func EachLocale(c *Locales, fb Fallback, build func(*View) ([]byte, error)) (map[string][]byte, error) {
	return EachLocaleIn([]*Locales{c}, fb, func(v []*View) ([]byte, error) { return build(v[0]) })
}

// EachLocaleIn is EachLocale over several trees, for a data set made of
// what each holds -- display names, which ICU keeps in the lang tree, the
// region tree and the locales tree. A locale is a bundle of any of them
// that none has as an alias, and build has its view in each tree, in the
// order given.
func EachLocaleIn(trees []*Locales, fb Fallback, build func([]*View) ([]byte, error)) (map[string][]byte, error) {
	names := map[string]bool{}
	for _, c := range trees {
		for _, name := range c.Names() {
			n, err := c.Get(name)
			if err != nil {
				return nil, err
			}
			if n == nil {
				continue
			}
			if n.Get("%%ALIAS") != nil {
				names[name] = false
			} else if _, seen := names[name]; !seen {
				names[name] = true
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for name, keep := range names {
		if keep {
			sorted = append(sorted, name)
		}
	}
	sort.Strings(sorted)

	out := map[string][]byte{}
	for _, name := range sorted {
		file, err := buildOne(trees, fb, name, build)
		if err != nil {
			return nil, err
		}
		if file == nil {
			continue
		}
		if _, _, _, variant := splitName(name); variant {
			if !bcp47Variant(name[strings.LastIndex(name, "_")+1:]) {
				continue
			}
			without, err := buildOne(trees, fb, name[:strings.LastIndex(name, "_")], build)
			if err != nil {
				return nil, err
			}
			if bytes.Equal(file, without) {
				continue
			}
		}
		out[Tag(name)] = file
	}
	return out, nil
}

// bcp47Variant reports whether a variant is one a BCP 47 tag can name: five
// to eight letters or digits, or four beginning with a digit.
func bcp47Variant(v string) bool {
	for _, c := range v {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9') {
			return false
		}
	}
	return 5 <= len(v) && len(v) <= 8 || len(v) == 4 && '0' <= v[0] && v[0] <= '9'
}

func buildOne(trees []*Locales, fb Fallback, name string, build func([]*View) ([]byte, error)) ([]byte, error) {
	views := make([]*View, len(trees))
	for i, c := range trees {
		v, err := c.View(name, fb)
		if err != nil {
			return nil, err
		}
		views[i] = v
	}
	return build(views)
}

// Tag is the BCP 47 tag of an ICU locale name: "und" for the root, hyphens
// for underscores, and a variant in lowercase, "en-US-posix".
func Tag(name string) string {
	if name == "root" {
		return "und"
	}
	parts := strings.Split(name, "_")
	_, script, region, _ := splitName(name)
	for i, p := range parts {
		if i > 0 && p != script && p != region {
			parts[i] = strings.ToLower(p)
		}
	}
	return strings.Join(parts, "-")
}
