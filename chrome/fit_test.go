package chrome

import (
	"strings"
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// Every overlay fits the data it is laid over: each pool it extends was
// extended from the pool beneath it, which a profile only finds out when a
// service reads the pool. So a Chrome of each overlay builds a service
// reading each pool an overlay extends. The intl package's data rebuilt
// without the chrome package's (regen -chrome=false), after a change that
// adds a part to a pool, fails here rather than in a host.
func TestEveryOverlayFits(t *testing.T) {
	loc, err := intl.ParseLocale("de")
	if err != nil {
		t.Fatal(err)
	}
	firsts := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(versions), "\n") {
		if f := strings.Fields(line); len(f) == 3 && firsts[f[2]] == "" {
			firsts[f[2]] = f[0]
		}
	}
	pools := []struct {
		pool  string
		build func(intl.Source, intl.Compat) error
	}{
		{"dates and zone names", func(s intl.Source, c intl.Compat) error {
			_, err := intl.NewDateTimeFormatFrom(s, loc, intl.DateTimeFormatOptions{
				DateStyle: intl.LengthFull, TimeStyle: intl.LengthFull, TimeZone: "Europe/Berlin", Compat: c})
			return err
		}},
		{"numbers", func(s intl.Source, c intl.Compat) error {
			_, err := intl.NewNumberFormatFrom(s, loc, intl.NumberFormatOptions{
				Style: intl.StyleCurrency, Currency: "EUR", Compat: c})
			return err
		}},
		{"units", func(s intl.Source, c intl.Compat) error {
			_, err := intl.NewNumberFormatFrom(s, loc, intl.NumberFormatOptions{
				Style: intl.StyleUnit, Unit: "kilometer-per-hour", Compat: c})
			return err
		}},
		{"names", func(s intl.Source, c intl.Compat) error {
			_, err := intl.NewDisplayNamesFrom(s, loc, intl.DisplayNamesOptions{Kind: intl.DisplayLanguage, Compat: c})
			return err
		}},
		{"relative times", func(s intl.Source, c intl.Compat) error {
			_, err := intl.NewRelativeTimeFormatFrom(s, loc, intl.RelativeTimeFormatOptions{Compat: c})
			return err
		}},
	}
	for _, name := range chain() {
		version := firsts[name]
		if version == "" {
			t.Errorf("no Chrome takes the overlay %s", name)
			continue
		}
		p, err := ForVersion(version)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range pools {
			if err := c.build(p.Source, p.Compat); err != nil {
				t.Errorf("Chrome %s, overlay %s: the %s: %v", version, name, c.pool, err)
			}
		}
	}
}
