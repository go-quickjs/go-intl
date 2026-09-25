package intl_test

import (
	"strings"
	"testing"

	"github.com/go-quickjs/go-intl"
)

// ResolveLocale and SupportedLocales by lookup, with Node's answers: the
// first request the service has, cut short, with its Unicode extension (the
// service keeps only the keywords it reads: Node's NumberFormat drops
// "-u-ca-buddhist" from "fr"), else the default.
func TestLocaleMatcherResolve(t *testing.T) {
	m, err := intl.NewLocaleMatcher(intl.Embedded, intl.ServiceNumberFormat)
	if err != nil {
		t.Fatal(err)
	}
	def, _ := intl.ParseLocale("en-US")
	for _, c := range []struct {
		requested []string
		resolved  string
		supported string
	}{
		{[]string{"xx", "de-CH-u-nu-thai"}, "de-CH-u-nu-thai", "de-CH-u-nu-thai"},
		{[]string{"ht"}, "en-US", ""},
		{[]string{"az-Arab-u-nu-latn"}, "az-u-nu-latn", "az-Arab-u-nu-latn"},
		{[]string{"en-t-ja"}, "en", "en-t-ja"},
		{[]string{"zh-Hant-SG"}, "zh-Hant", "zh-Hant-SG"},
		{[]string{"sr-Latn-XK"}, "sr-Latn-XK", "sr-Latn-XK"},
		{[]string{"qaa", "fr-XY-u-ca-buddhist"}, "fr-u-ca-buddhist", "fr-XY-u-ca-buddhist"},
	} {
		var requested []intl.Locale
		for _, tag := range c.requested {
			l, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}
			requested = append(requested, l)
		}
		for _, kind := range []intl.MatcherKind{intl.BestFit, intl.Lookup} {
			if got := m.Resolve(requested, kind, def).String(); got != c.resolved {
				t.Errorf("Resolve(%v) = %s, want %s", c.requested, got, c.resolved)
			}
			var supported []string
			for _, l := range m.Supported(requested, kind) {
				supported = append(supported, l.String())
			}
			if got := strings.Join(supported, " "); got != c.supported {
				t.Errorf("Supported(%v) = %q, want %q", c.requested, got, c.supported)
			}
		}
	}
}
