package icusrc

import (
	"os"
	"path/filepath"
	"testing"
)

// A tree of sources is read through its filter: a file it leaves out is
// not there, a bundle has only what it keeps, and a file of the tree that
// is no bundle, which ICU's build reads unfiltered, is read whole.
func TestFilteredTree(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("data/locales/root.txt", `root{ NumberElements{ default{"latn"} } }`)
	write("data/locales/en.txt", `en{ NumberElements{ latn{ symbols{ decimal{"."} } } minimalPairs{ one{"1"} } } }`)
	write("data/locales/ne.txt", `ne{ %%Parent{"root"} NumberElements{ default{"deva"} } }`)
	write("data/locales/dsb.txt", `dsb{ NumberElements{ default{"latn"} } }`)
	write("data/locales/LOCALE_DEPS.json", `{"parents": {}}`)
	write("data/misc/supplementalData.txt", `supplementalData{ calendarData{ x{"y"} } currencyData{ z{"w"} } }`)
	write("filter.json", `{
  "localeFilter": {"filterType": "language", "includelist": ["en", "ne"]},
  "resourceFilters": [
    {"categories": ["locales_tree"], "rules": ["-/NumberElements/minimalPairs"]},
    {"categories": ["locales_tree"], "files": {"includelist": ["ne"]}, "rules": ["-/*", "+/%%Parent"]},
    {"categories": ["misc"], "files": {"includelist": ["supplementalData"]}, "rules": ["-/*", "+/calendarData"]}
  ]
}`)
	c, err := OpenLocales(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.Names(); len(got) != 3 || got[0] != "en" || got[1] != "ne" || got[2] != "root" {
		t.Errorf("Names = %v, want [en ne root]", got)
	}
	en, err := c.Get("en")
	if err != nil {
		t.Fatal(err)
	}
	if en.Get("NumberElements", "minimalPairs") != nil || en.Get("NumberElements", "latn", "symbols", "decimal") == nil {
		t.Error("en's minimal pairs kept, or its symbols dropped")
	}
	ne, _ := c.Get("ne")
	if ne.Get("NumberElements") != nil || ne.Get("%%Parent") == nil {
		t.Error("ne's numbers kept, or its parent dropped")
	}
	if b, err := c.ReadTreeFile("LOCALE_DEPS.json"); err != nil || len(b) == 0 {
		t.Errorf("LOCALE_DEPS.json: %v", err)
	}
	supp, err := c.Misc("supplementalData")
	if err != nil {
		t.Fatal(err)
	}
	if supp.Get("currencyData") != nil || supp.Get("calendarData", "x") == nil {
		t.Error("supplementalData not filtered as its rules say")
	}
}
