package intl_test

import (
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// TestMinimizeLength pins Intl.Locale's minimize on both sides of V8's
// check of whether minimizeSubtags changed a locale. The standard, ICU and
// Node v26.10 answer the minimal tag. V8 before 14.4 compared the base
// names' lengths, so a minimal tag as long as the locale's in ICU's
// spelling -- "_Hant" and "zh_TW" -- was taken for no change. A minimal tag
// of another length is answered either way.
func TestMinimizeLength(t *testing.T) {
	for _, c := range []struct {
		tag              string
		standard, chrome string
	}{
		{"und-Hant", "zh-TW", "und-Hant"},
		{"zh-Hant-TW", "zh-TW", "zh-TW"},
		{"und-Arab", "ar", "ar"},
		{"en-Latn-US", "en", "en"},
		{"en", "en", "en"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []struct {
			compat intl.Compat
			want   string
		}{
			{intl.Standard, c.standard},
			{intl.NodeICU, c.standard},
			{intl.NodeICU | intl.MinimizeLength, c.chrome},
		} {
			info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{Compat: p.compat})
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Minimize(loc).String(); got != p.want {
				t.Errorf("%v %s: %s, want %s", p.compat, c.tag, got, p.want)
			}
		}
	}
}
