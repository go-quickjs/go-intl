package intl

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/layout"
)

// A locale none of whose bundles, nor those it falls back to, are in a tree
// reads the default locale's before the root's, as ICU's ures_open does; a
// default the tree has no bundle for either, or the root, reads the root's.
// A browser's filtered trees leave whole languages out: Chrome's unit tree
// has no Akan. The default is WithDefaultLocale's, and an overlay laid over
// it keeps it.
func TestChainInDefaultLocale(t *testing.T) {
	tree, err := blob.BuildIndex(layout.ICUTree, map[string][]byte{"bundle root": nil, "bundle en": nil, "bundle de": nil})
	if err != nil {
		t.Fatal(err)
	}
	data, err := NewOverlay(Embedded, NewFS(fstest.MapFS{
		"overlay.bin":      {Data: []byte("overlay 1\n")},
		"icutree-unit.bin": {Data: tree},
	}))
	if err != nil {
		t.Fatal(err)
	}
	locale := func(tag string) Locale {
		l, err := ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	beneath, err := NewOverlay(WithDefaultLocale(Embedded, locale("de-CH")), NewFS(fstest.MapFS{
		"overlay.bin":      {Data: []byte("overlay 1\n")},
		"icutree-unit.bin": {Data: tree},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		src  Source
		tag  string
		want string
	}{
		{"en-US", WithDefaultLocale(data, locale("en-US")), "ak", "[en und]"},
		{"en-US", WithDefaultLocale(data, locale("en-US")), "de-AT", "[de und]"},
		{"en-US", WithDefaultLocale(data, locale("en-US")), "und", "[und]"},
		{"de", WithDefaultLocale(data, locale("de")), "ak", "[de und]"},
		{"fr, which the tree has no bundle for", WithDefaultLocale(data, locale("fr")), "ak", "[und]"},
		{"the root", WithDefaultLocale(data, Locale{}), "ak", "[und]"},
		{"de-CH, beneath an overlay", beneath, "ak", "[de und]"},
	} {
		f, err := NewFallbacker(c.src)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(f.ChainIn(treeUnit, locale(c.tag).Data())); got != c.want {
			t.Errorf("default %s, %s: %s, want %s", c.name, c.tag, got, c.want)
		}
	}
}
