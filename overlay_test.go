package intl

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/datapack"
	"github.com/go-quickjs/go-intl/internal/layout"
)

func dataLocale(t *testing.T, tag string) DataLocale {
	t.Helper()
	l, err := ParseLocale(tag)
	if err != nil {
		t.Fatal(err)
	}
	return l.Data()
}

// samePairs lays out a same.bin of each locale and the one it shares.
func samePairs(t *testing.T, pairs ...string) []byte {
	b := []byte{layout.Pairs}
	for i := 0; i < len(pairs); i += 2 {
		b, _ = dataLocale(t, pairs[i]).AppendBinary(b)
		b, _ = dataLocale(t, pairs[i+1]).AppendBinary(b)
	}
	return b
}

func overlayOf(t *testing.T, base, top Source) Source {
	t.Helper()
	s, err := NewOverlay(base, top)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// An overlay's file replaces the base's, a file it removes is gone, and
// the rest is the base's; a locale the base's same.bin names reads the
// overlay's file for the locale it shares.
func TestOverlayFiles(t *testing.T) {
	base := NewFS(fstest.MapFS{
		"numbers/de.bin":    {Data: []byte("base de")},
		"numbers/fr.bin":    {Data: []byte("base fr")},
		"numbers/it.bin":    {Data: []byte("base it")},
		"numbers/same.bin":  {Data: samePairs(t, "de-AT", "de", "fr-CH", "fr")},
		"likelysubtags.bin": {Data: []byte("base likely")},
	})
	top := NewFS(fstest.MapFS{
		"overlay.bin":    {Data: []byte("overlay 1\nnumbers/fr.bin\n")},
		"numbers/de.bin": {Data: []byte("overlay de")},
	})
	s := overlayOf(t, base, top)
	for _, c := range []struct {
		m    Marker
		tag  string
		want string
	}{
		{MarkerNumbers, "de", "overlay de"},
		{MarkerNumbers, "de-AT", "overlay de"},
		{MarkerNumbers, "it", "base it"},
		{MarkerNumbers, "fr", ""},
		{MarkerNumbers, "fr-CH", ""},
		{MarkerNumbers, "es", ""},
		{MarkerLikelySubtags, "und", "base likely"},
	} {
		d := DataLocale{}
		if c.tag != "und" {
			d = dataLocale(t, c.tag)
		}
		b, err := s.Open(c.m, d)
		switch {
		case c.want == "" && !errors.Is(err, ErrNotFound):
			t.Errorf("%s %s: %q, %v, want ErrNotFound", c.m, c.tag, b, err)
		case c.want != "" && (err != nil || string(b) != c.want):
			t.Errorf("%s %s: %q, %v, want %q", c.m, c.tag, b, err, c.want)
		}
	}
	if b, err := s.(*overlaySource).readFile(overlayManifest); err == nil {
		t.Errorf("the manifest is served as data: %q", b)
	}
}

// A pool an overlay extends is read over the base's, and one it leaves be
// is the base's, through any number of overlays.
func TestOverlayPools(t *testing.T) {
	const v = 3
	// tables[s] is a table of one string, s, written through the pool that
	// added it, which a reader reads back through any pool extending it.
	tables := map[string][]byte{}
	add := func(p *blob.Pool, parts ...string) {
		for _, s := range parts {
			w := blob.NewPooledWriter(v, p)
			w.SharedString(s)
			tables[s] = w.Bytes()
		}
	}
	basePool := blob.NewPool(v)
	add(basePool, "Greenwich Mean Time", "British Summer Time")
	baseShared, _ := blob.ReadShared(basePool.Bytes(), v)
	first := blob.SeedPool(v, baseShared)
	add(first, "British Time")
	firstExt, _ := first.Extension()
	firstShared, _ := blob.Extend(baseShared, firstExt, v)
	second := blob.SeedPool(v, firstShared)
	add(second, "Irish Time")
	secondExt, _ := second.Extension()

	base := NewFS(fstest.MapFS{"zonenamesshared.bin": {Data: basePool.Bytes()}})
	manifest := &fstest.MapFile{Data: []byte("overlay 1\n")}
	one := overlayOf(t, base, NewFS(fstest.MapFS{"overlay.bin": manifest, "zonenamesshared.bin": {Data: firstExt}}))
	two := overlayOf(t, one, NewFS(fstest.MapFS{"overlay.bin": manifest, "zonenamesshared.bin": {Data: secondExt}}))
	// An overlay that leaves the pool be, over one that extends it.
	three := overlayOf(t, two, NewFS(fstest.MapFS{"overlay.bin": manifest}))
	for _, c := range []struct {
		name string
		src  Source
		want []string
	}{
		{"base", base, []string{"Greenwich Mean Time", "British Summer Time"}},
		{"one", one, []string{"Greenwich Mean Time", "British Summer Time", "British Time"}},
		{"two", two, []string{"Greenwich Mean Time", "British Summer Time", "British Time", "Irish Time"}},
		{"three", three, []string{"Greenwich Mean Time", "British Summer Time", "British Time", "Irish Time"}},
	} {
		pool, err := openShared(c.src, MarkerZoneNamesShared, v)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if pool.Len() != len(c.want) {
			t.Errorf("%s: %d parts, want %d", c.name, pool.Len(), len(c.want))
		}
		for _, w := range c.want {
			r, err := blob.NewPooledReader(tables[w], v, pool)
			if err != nil {
				t.Fatal(err)
			}
			if got := r.SharedString(); got != w || r.Err() != nil {
				t.Errorf("%s: the table of %q reads %q, %v", c.name, w, got, r.Err())
			}
		}
	}

	// An extension is not a pool outside an overlay, nor over a base it
	// was not made over.
	if _, err := openShared(NewFS(fstest.MapFS{"zonenamesshared.bin": {Data: firstExt}}), MarkerZoneNamesShared, v); err == nil {
		t.Error("an extension read as a pool outside an overlay")
	}
	if _, err := openShared(overlayOf(t, base, NewFS(fstest.MapFS{"overlay.bin": manifest, "zonenamesshared.bin": {Data: secondExt}})),
		MarkerZoneNamesShared, v); err == nil {
		t.Error("an extension read over a base it does not extend")
	}
	removed := overlayOf(t, base, NewFS(fstest.MapFS{"overlay.bin": {Data: []byte("overlay 1\nzonenamesshared.bin\n")}}))
	if _, err := openShared(removed, MarkerZoneNamesShared, v); !errors.Is(err, ErrNotFound) {
		t.Errorf("a removed pool: %v, want ErrNotFound", err)
	}
}

// An overlay needs its manifest, sorted, and data directories either side.
func TestOverlayRefused(t *testing.T) {
	base := NewFS(fstest.MapFS{})
	for _, c := range []struct {
		name     string
		manifest string
	}{
		{"no manifest", ""},
		{"another header", "overlay 2\n"},
		{"unsorted", "overlay 1\nnumbers/fr.bin\nnumbers/de.bin\n"},
		{"repeated", "overlay 1\nnumbers/de.bin\nnumbers/de.bin\n"},
		{"empty name", "overlay 1\n\nnumbers/de.bin\n"},
	} {
		files := fstest.MapFS{}
		if c.manifest != "" {
			files["overlay.bin"] = &fstest.MapFile{Data: []byte(c.manifest)}
		}
		if _, err := NewOverlay(base, NewFS(files)); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	other := sourceFunc(func(Marker, DataLocale) ([]byte, error) { return nil, ErrNotFound })
	top := NewFS(fstest.MapFS{"overlay.bin": {Data: []byte("overlay 1\n")}})
	if _, err := NewOverlay(other, top); err == nil {
		t.Error("an overlay over a source that is not a data directory")
	}
	if _, err := NewOverlay(base, other); err == nil {
		t.Error("an overlay that is not a data directory")
	}
}

type sourceFunc func(Marker, DataLocale) ([]byte, error)

func (f sourceFunc) Open(m Marker, d DataLocale) ([]byte, error) { return f(m, d) }

// A pack serves the files it was made of; anything else is refused.
func TestNewPack(t *testing.T) {
	pack, err := datapack.Build(fstest.MapFS{"likelysubtags.bin": {Data: []byte("likely")}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewPack(string(pack))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := s.Open(MarkerLikelySubtags, DataLocale{}); err != nil || string(b) != "likely" {
		t.Errorf("Open = %q, %v", b, err)
	}
	if _, err := s.Open(MarkerParentLocales, DataLocale{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a file the pack lacks: %v", err)
	}
	if _, err := NewPack("not a pack"); err == nil {
		t.Error("NewPack took what is not a pack")
	}
}

// Over the embedded data, an overlay's locale is what the formatters read:
// German numbers written with French data, and, with the German data
// removed, with the root's.
func TestOverlayFormats(t *testing.T) {
	fr, err := Embedded.Open(MarkerNumbers, dataLocale(t, "fr"))
	if err != nil {
		t.Fatal(err)
	}
	format := func(src Source) string {
		f, err := NewNumberFormatFrom(src, Locale{Language: Language{'d', 'e'}}, NumberFormatOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return f.Format(-1234.5)
	}
	if got, want := format(Embedded), "-1.234,5"; got != want {
		t.Fatalf("de without an overlay: %q, want %q", got, want)
	}
	asFrench := overlayOf(t, Embedded, NewFS(fstest.MapFS{
		"overlay.bin":    {Data: []byte("overlay 1\n")},
		"numbers/de.bin": {Data: fr},
	}))
	if got, want := format(asFrench), "-1 234,5"; got != want {
		t.Errorf("de with French data: %q, want %q", got, want)
	}
	asRoot := overlayOf(t, Embedded, NewFS(fstest.MapFS{
		"overlay.bin": {Data: []byte("overlay 1\nnumbers/de.bin\n")},
	}))
	if got, want := format(asRoot), "-1,234.5"; got != want {
		t.Errorf("de with its data removed: %q, want %q", got, want)
	}
}
