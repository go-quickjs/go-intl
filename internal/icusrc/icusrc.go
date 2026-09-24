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

// Locales reads ICU's locale bundles, data/locales/*.txt, from the data
// archive, each once.
type Locales struct {
	z     *zip.ReadCloser
	files map[string]*zip.File
	cache map[string]*icutxt.Node
}

// OpenLocales opens the data archive for its locale bundles.
func OpenLocales(zipPath string) (*Locales, error) {
	z, err := Open(zipPath, DataSHA256)
	if err != nil {
		return nil, err
	}
	out := &Locales{z: z, files: map[string]*zip.File{}, cache: map[string]*icutxt.Node{}}
	for _, f := range z.File {
		if name, ok := strings.CutPrefix(f.Name, "data/locales/"); ok && strings.HasSuffix(name, ".txt") {
			out.files[strings.TrimSuffix(name, ".txt")] = f
		}
	}
	return out, nil
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
