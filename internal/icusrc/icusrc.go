// Package icusrc opens the ICU release artifacts the generators read, after
// checking each is the one SOURCES.md pins. Neither is vendored: the export is
// 5.6 MB and the data sources 20 MB.
package icusrc

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// The pinned artifacts of the ICU 78.3 release, from SOURCES.md.
const (
	// ExportSHA256 is icu4x-icuexportdata-78.3.zip: ICU's tables as it
	// exports them for ICU4X.
	ExportSHA256 = "eb63a12439f3fd9199886808275900229a5638fb0ee88d3c3c528eca7b811e60"
	// DataSHA256 is icu4c-78.3-data.zip: ICU's data sources, CLDR converted
	// to ICU's resource bundle text.
	DataSHA256 = "9d8b3899096aeb83e4e21ef8a40fec9e03b28db18c48452efac882ce25a91e27"
)

// Open opens a zip archive after checking its checksum.
func Open(name, want string) (*zip.ReadCloser, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return nil, err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return nil, fmt.Errorf("%s has sha256 %s, but SOURCES.md pins %s", name, got, want)
	}
	return zip.OpenReader(name)
}

// ReadFile reads one file from an archive.
func ReadFile(z *zip.ReadCloser, name string) ([]byte, error) {
	for _, f := range z.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("%s is not in the archive", name)
}

// Locales reads one tree of ICU's locale bundles -- data/locales/*.txt, or
// data/zone/*.txt -- from the data archive, each once.
type Locales struct {
	z     *zip.ReadCloser
	tree  string
	files map[string]*zip.File
	cache map[string]*icutxt.Node
}

// OpenLocales opens the data archive for its locale bundles.
func OpenLocales(zipPath string) (*Locales, error) {
	return OpenTree(zipPath, "locales")
}

// OpenTree opens the data archive for one tree of bundles: "locales", "zone",
// "region" and so on.
func OpenTree(zipPath, tree string) (*Locales, error) {
	z, err := Open(zipPath, DataSHA256)
	if err != nil {
		return nil, err
	}
	out := &Locales{z: z, tree: tree, files: map[string]*zip.File{}, cache: map[string]*icutxt.Node{}}
	for _, f := range z.File {
		if name, ok := strings.CutPrefix(f.Name, "data/"+tree+"/"); ok && strings.HasSuffix(name, ".txt") {
			out.files[strings.TrimSuffix(name, ".txt")] = f
		}
	}
	return out, nil
}

// Names lists the tree's bundles, sorted.
func (c *Locales) Names() []string {
	out := make([]string, 0, len(c.files))
	for name := range c.files {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ReadTreeFile reads a file of the tree other than a bundle, such as
// LOCALE_DEPS.json.
func (c *Locales) ReadTreeFile(name string) ([]byte, error) {
	return ReadFile(c.z, "data/"+c.tree+"/"+name)
}

// ReadMisc reads one of the data archive's non-locale files, data/misc/
// <name>.txt.
func (c *Locales) ReadMisc(name string) ([]byte, error) {
	return ReadFile(c.z, "data/misc/"+name+".txt")
}

// Has reports whether the tree has a bundle of this name.
func (c *Locales) Has(name string) bool {
	_, ok := c.files[name]
	return ok
}

// A Fallback is what ICU's getParentLocaleID consults when a bundle a locale
// asks for does not exist: CLDR's parent locales, and the script a language
// is written in by default, in a region or anywhere.
type Fallback struct {
	Parent        func(name string) (string, bool)
	DefaultScript func(language, region string) string
}

// Resolve is the whole of what ICU's ures_open reads for a locale, the most
// specific bundle first and the root last. ICU first finds a bundle that
// exists, asking getParentLocaleID for the next name to try, and then follows
// that bundle's %%Parent or truncates its name. The two walks differ, which is
// why "sr_Cyrl_ME", which has no bundle of its own, reads "sr_Cyrl" and not
// "sr_Latn".
func (c *Locales) Resolve(name string, fb Fallback) ([]*icutxt.Node, error) {
	orig := name
	for i := 0; !c.Has(name) && i < 16; i++ {
		next, ok := fb.parent(name, orig)
		if !ok {
			name = "root"
			break
		}
		name = next
	}
	var out []*icutxt.Node
	for i := 0; name != "" && i < 16; i++ {
		n, err := c.Get(name)
		if err != nil {
			return nil, err
		}
		// A bundle that is only an alias is read as the bundle it names,
		// whose parents then follow: sr_ME is sr_Latn_ME.
		if n != nil {
			if a := n.Get("%%ALIAS"); a != nil && a.Value != "" {
				name = a.Value
				continue
			}
		}
		if n != nil {
			out = append(out, n)
		}
		if name == "root" {
			return out, nil
		}
		next := ""
		if n != nil {
			if p := n.Get("%%Parent"); p != nil {
				next = p.Value
			}
		}
		if next == "" {
			if cut := strings.LastIndex(name, "_"); cut > 0 {
				next = name[:cut]
			} else {
				next = "root"
			}
		}
		name = next
	}
	return out, nil
}

// Bundle is the name of the bundle ICU's ures_open reads first for a
// locale: the locale's own if the tree has it, else the one fallback finds,
// with an alias followed. It is "root" for a locale that finds none.
func (c *Locales) Bundle(name string, fb Fallback) string {
	orig := name
	for i := 0; !c.Has(name) && i < 16; i++ {
		next, ok := fb.parent(name, orig)
		if !ok {
			return "root"
		}
		name = next
	}
	for i := 0; i < 16; i++ {
		n, err := c.Get(name)
		if err != nil || n == nil {
			return name
		}
		a := n.Get("%%ALIAS")
		if a == nil || a.Value == "" {
			return name
		}
		name = a.Value
	}
	return name
}

// parent is ICU's getParentLocaleID for a bundle that does not exist.
func (fb Fallback) parent(name, orig string) (string, bool) {
	language, script, region, variant := splitName(name)
	if variant {
		if cut := strings.LastIndex(name, "_"); cut > 0 {
			return name[:cut], true
		}
		return "", false
	}
	if p, ok := fb.Parent(name); ok {
		return p, true
	}
	switch {
	case script != "" && region != "":
		if fb.DefaultScript(language, region) == script {
			return language + "_" + region, true
		}
		return language + "_" + script, true
	case region != "":
		if _, origScript, _, _ := splitName(orig); origScript != "" {
			return language + "_" + origScript, true
		}
		return language + "_" + fb.DefaultScript(language, region), true
	case script != "":
		if fb.DefaultScript(language, "") == script {
			return language, true
		}
		return "", false
	}
	return "", false
}

// splitName takes an ICU locale name apart.
func splitName(name string) (language, script, region string, variant bool) {
	parts := strings.Split(name, "_")
	language = parts[0]
	for _, p := range parts[1:] {
		switch {
		case script == "" && region == "" && len(p) == 4:
			script = p
		case region == "" && (len(p) == 2 || len(p) == 3 && p[0] >= '0' && p[0] <= '9'):
			region = p
		default:
			variant = true
		}
	}
	return language, script, region, variant
}

// Close closes the archive.
func (c *Locales) Close() error { return c.z.Close() }

// Get returns one locale's bundle, by ICU's name for it ("zh_Hant"), or nil
// when ICU has none.
func (c *Locales) Get(name string) (*icutxt.Node, error) {
	if n, ok := c.cache[name]; ok {
		return n, nil
	}
	f, ok := c.files[name]
	if !ok {
		c.cache[name] = nil
		return nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, err
	}
	n, err := icutxt.Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%s.txt: %w", name, err)
	}
	c.cache[name] = n
	return n, nil
}

// Chain is a locale's bundles below the root, the locale first: ICU's parent
// where the bundle names one with %%Parent, truncation where it does not.
// What only the root says is left out, because the root is where CLDR's
// locale-relative aliases live, and a caller has to resolve those itself.
func (c *Locales) Chain(name string) ([]*icutxt.Node, error) {
	var out []*icutxt.Node
	for i := 0; name != "root" && name != "" && i < 16; i++ {
		n, err := c.Get(name)
		if err != nil {
			return nil, err
		}
		next := ""
		if n != nil {
			out = append(out, n)
			if p := n.Get("%%Parent"); p != nil {
				next = p.Value
			}
		}
		if next == "" {
			if cut := strings.LastIndex(name, "_"); cut > 0 {
				next = name[:cut]
			} else {
				next = "root"
			}
		}
		name = next
	}
	return out, nil
}
