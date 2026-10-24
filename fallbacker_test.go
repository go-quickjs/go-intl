package intl

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/layout"
)

// A locale none of whose bundles, nor those it falls back to, are in a tree
// reads the default locale's before the root's, as ICU's ures_open does;
// without a default, the root's. A browser's filtered trees leave whole
// languages out: Chrome's unit tree has no Akan.
func TestChainInDefaultLocale(t *testing.T) {
	index := func(version byte, records map[string][]byte) []byte {
		b, err := blob.BuildIndex(version, records)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	tree := index(layout.ICUTree, map[string][]byte{"bundle root": nil, "bundle en": nil, "bundle de": nil})
	for _, c := range []struct {
		name     string
		fallback map[string][]byte
		tag      string
		want     string
	}{
		{"default", map[string][]byte{"default": []byte("en_US")}, "ak", "[en und]"},
		{"default", map[string][]byte{"default": []byte("en_US")}, "de-AT", "[de und]"},
		{"default", map[string][]byte{"default": []byte("en_US")}, "und", "[und]"},
		{"none", map[string][]byte{}, "ak", "[und]"},
	} {
		src, err := NewOverlay(Embedded, NewFS(fstest.MapFS{
			"overlay.bin":      {Data: []byte("overlay 1\n")},
			"icutree-unit.bin": {Data: tree},
			"icufallback.bin":  {Data: index(layout.ICUFallback, c.fallback)},
		}))
		if err != nil {
			t.Fatal(err)
		}
		f, err := NewFallbacker(src)
		if err != nil {
			t.Fatal(err)
		}
		l, err := ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(f.ChainIn(treeUnit, l.Data())); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.name, c.tag, got, c.want)
		}
	}
}
