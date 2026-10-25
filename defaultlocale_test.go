package intl_test

import (
	"testing"
	"testing/fstest"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/chrome"
)

// A source's default locale is the host's, unless WithDefaultLocale gives
// it another, which an overlay over it, or beneath it, keeps.
func TestDefaultLocale(t *testing.T) {
	ko, err := intl.ParseLocale("ko-KR")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := intl.DefaultLocale(intl.Embedded).String(), intl.HostLocale().String(); got != want {
		t.Errorf("the package's data: %s, want the host's, %s", got, want)
	}
	given := intl.WithDefaultLocale(intl.Embedded, ko)
	over, err := intl.NewOverlay(given, intl.NewFS(fstest.MapFS{"overlay.bin": {Data: []byte("overlay 1\n")}}))
	if err != nil {
		t.Fatal(err)
	}
	p, err := chrome.ForVersion("142")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		src  intl.Source
	}{
		{"given", given},
		{"beneath an overlay", over},
		{"over a profile", intl.WithDefaultLocale(p.Source, ko)},
	} {
		if got := intl.DefaultLocale(c.src).String(); got != "ko-KR" {
			t.Errorf("%s: %s, want ko-KR", c.name, got)
		}
	}
}

// A source given a default reads its data as the source beneath it does,
// shared pools and all, through any number of overlays: Chrome 142's data is
// nine overlays deep.
func TestWithDefaultLocaleReadsTheData(t *testing.T) {
	ko, err := intl.ParseLocale("ko-KR")
	if err != nil {
		t.Fatal(err)
	}
	de, err := intl.ParseLocale("de")
	if err != nil {
		t.Fatal(err)
	}
	p, err := chrome.ForVersion("142")
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []intl.Source{intl.WithDefaultLocale(intl.Embedded, ko), intl.WithDefaultLocale(p.Source, ko)} {
		if _, err := intl.NewDateTimeFormatFrom(src, de, intl.DateTimeFormatOptions{
			DateStyle: intl.LengthFull, TimeStyle: intl.LengthFull, TimeZone: "Europe/Berlin"}); err != nil {
			t.Errorf("dates and zone names: %v", err)
		}
		if _, err := intl.NewNumberFormatFrom(src, de, intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR"}); err != nil {
			t.Errorf("numbers: %v", err)
		}
		if _, err := intl.NewNumberFormatFrom(src, de, intl.NumberFormatOptions{Style: intl.StyleUnit, Unit: "kilometer-per-hour"}); err != nil {
			t.Errorf("units: %v", err)
		}
		if _, err := intl.NewDisplayNamesFrom(src, de, intl.DisplayNamesOptions{Kind: intl.DisplayLanguage}); err != nil {
			t.Errorf("names: %v", err)
		}
		if _, err := intl.NewRelativeTimeFormatFrom(src, de, intl.RelativeTimeFormatOptions{}); err != nil {
			t.Errorf("relative times: %v", err)
		}
	}
}
