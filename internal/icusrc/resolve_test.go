package icusrc

import (
	"archive/zip"
	"strings"
	"testing"

	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// treeOf is a tree of bundles given as their text, read as the data
// archive's are.
func treeOf(t *testing.T, bundles map[string]string) *Locales {
	t.Helper()
	c := &Locales{files: map[string]*zip.File{}, cache: map[string]*icutxt.Node{}}
	for name, src := range bundles {
		n, err := icutxt.Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c.files[name] = nil
		c.cache[name] = n
	}
	return c
}

var plainFallback = Fallback{
	Parent:        func(string) (string, bool) { return "", false },
	DefaultScript: func(string, string) string { return "Latn" },
}

// A table resolves key by key down the chain, and a /LOCALE/ alias in the
// root names the asking locale's resource: fr's or-short is fr's or.
func TestViewResolves(t *testing.T) {
	c := treeOf(t, map[string]string{
		"root": `root{
			listPattern{
				or{ 2{"{0} or {1}"} end{"{0}, or {1}"} }
				or-short{ 2:alias{"/LOCALE/listPattern/or/2"} end:alias{"/LOCALE/listPattern/or/end"} }
				standard-short:alias{"/LOCALE/listPattern/standard"}
				standard{ 2{"{0}, {1}"} end{"{0}, {1}"} }
			}
		}`,
		"fr":    `fr{ listPattern{ or{ 2{"{0} ou {1}"} } standard{ end{"{0} et {1}"} } } }`,
		"fr_CA": `fr_CA{ listPattern{ standard-short{ 2{"{0} + {1}"} } } }`,
	})
	for _, c2 := range []struct {
		locale string
		path   string
		want   string
	}{
		{"fr", "listPattern/or/2", "{0} ou {1}"},
		{"fr", "listPattern/or/end", "{0}, or {1}"},
		{"fr", "listPattern/or-short/2", "{0} ou {1}"},
		{"fr", "listPattern/standard-short/end", "{0} et {1}"},
		{"fr", "listPattern/standard-short/2", "{0}, {1}"},
		{"fr_CA", "listPattern/standard-short/2", "{0} + {1}"},
		{"fr_CA", "listPattern/standard-short/end", "{0} et {1}"},
		{"fr_CA", "listPattern/or-short/2", "{0} ou {1}"},
		{"root", "listPattern/or-short/2", "{0} or {1}"},
		{"fr", "listPattern/missing", ""},
	} {
		v, err := c.View(c2.locale, plainFallback)
		if err != nil {
			t.Fatal(err)
		}
		n, err := v.Get(strings.Split(c2.path, "/")...)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if n != nil {
			got = n.Value
		}
		if got != c2.want {
			t.Errorf("%s %s = %q, want %q", c2.locale, c2.path, got, c2.want)
		}
	}

	// A merged table has each key once, the most specific bundle's first.
	v, _ := c.View("fr_CA", plainFallback)
	n, err := v.Get("listPattern", "standard-short")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, k := range n.Children {
		keys = append(keys, k.Key+"="+k.Value)
	}
	if got, want := strings.Join(keys, " "), "2={0} + {1} end={0} et {1}"; got != want {
		t.Errorf("fr_CA standard-short = %s, want %s", got, want)
	}
}

// An alias that leads back to itself is an error, not a hang.
func TestViewAliasLoop(t *testing.T) {
	c := treeOf(t, map[string]string{
		"root": `root{ a:alias{"/LOCALE/b"} b:alias{"/LOCALE/a"} }`,
	})
	v, _ := c.View("root", plainFallback)
	if _, err := v.Get("a"); err == nil {
		t.Error("an alias loop resolved")
	}
}

// Tags are BCP 47's.
func TestTag(t *testing.T) {
	for name, want := range map[string]string{
		"root": "und", "en_001": "en-001", "zh_Hant_HK": "zh-Hant-HK", "en_US_POSIX": "en-US-posix",
		"ja_JP_TRADITIONAL": "ja-JP-traditional",
	} {
		if got := Tag(name); got != want {
			t.Errorf("Tag(%q) = %q, want %q", name, got, want)
		}
	}
}
