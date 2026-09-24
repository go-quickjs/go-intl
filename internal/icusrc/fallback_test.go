package icusrc

import "testing"

// TestICUFallback checks the vendored tables read as ICU reads them.
func TestICUFallback(t *testing.T) {
	fb, err := ICUFallback()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ language, region, want string }{
		{"sr", "", "Cyrl"},
		{"sr", "ME", "Cyrl"}, // not the likely Latn: the table has no sr_ME
		{"az", "IQ", "Arab"},
		{"zh", "AU", "Hant"},
		{"xx", "", "Latn"},
	} {
		if got := fb.DefaultScript(c.language, c.region); got != c.want {
			t.Errorf("DefaultScript(%q, %q) = %q, want %q", c.language, c.region, got, c.want)
		}
	}
	for name, want := range map[string]string{"en_150": "en_001", "zh_Hant": "root", "sr_Latn": "root", "es_MX": "es_419"} {
		if got, ok := fb.Parent(name); !ok || got != want {
			t.Errorf("Parent(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	// sr_Cyrl_ME: Cyrillic is the default script of sr in Montenegro, so
	// the script goes and the region stays.
	if got, ok := fb.parent("sr_Cyrl_ME", "sr_Cyrl_ME"); !ok || got != "sr_ME" {
		t.Errorf("parent(sr_Cyrl_ME) = %q, %v; want sr_ME", got, ok)
	}
}
