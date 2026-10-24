package icusrc

import (
	"bytes"
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
// (ja_JP_TRADITIONAL) is kept only where its file differs from the one of
// the locale without the variant, which is what a variant locale without
// data of its own reads.
func EachLocale(c *Locales, fb Fallback, build func(*View) ([]byte, error)) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, name := range c.Names() {
		n, err := c.Get(name)
		if err != nil {
			return nil, err
		}
		if n == nil || n.Get("%%ALIAS") != nil {
			continue
		}
		file, err := buildOne(c, fb, name, build)
		if err != nil || file == nil {
			if err != nil {
				return nil, err
			}
			continue
		}
		if _, _, _, variant := splitName(name); variant {
			base := name[:strings.LastIndex(name, "_")]
			without, err := buildOne(c, fb, base, build)
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

func buildOne(c *Locales, fb Fallback, name string, build func(*View) ([]byte, error)) ([]byte, error) {
	v, err := c.View(name, fb)
	if err != nil {
		return nil, err
	}
	return build(v)
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
