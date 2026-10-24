package chrome_test

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/chrome"
)

// A version is found whole or by its start, the newest it starts.
func TestForVersion(t *testing.T) {
	for _, c := range []struct{ asked, version string }{
		{"155.0.8059.84", "155.0.8059.84"},
		{"155", "155.0.8059.86"},
		{"148.0.7778", "148.0.7778.218"},
		{"148.0.7778.169", "148.0.7778.169"},
		{"140", "140.0.7339.210"},
	} {
		p, err := chrome.ForVersion(c.asked)
		if err != nil {
			t.Fatal(err)
		}
		if p.Version != c.version {
			t.Errorf("%s is %s, want %s", c.asked, p.Version, c.version)
		}
	}
	for _, v := range []string{"139", "1550", "155.0.8059.1", ""} {
		if p, err := chrome.ForVersion(v); err == nil && v != "" {
			t.Errorf("%q found %s", v, p.Version)
		}
	}
}

// Each Chrome answers Node's divergences but those it answers as the
// standard does, and its own, from the version each began in.
func TestCompat(t *testing.T) {
	for _, c := range []struct {
		version  string
		has, not intl.Compat
	}{
		{"142", intl.NarrowSpace | intl.RoundingWindow | intl.AmeteMihretExtendedYear, intl.DurationOverflow | intl.CurrencyNames | intl.ThinSpace},
		{"146", intl.DurationOverflow | intl.PlainValueZone | intl.AmeteMihretExtendedYear, intl.ThinSpace},
		{"148.0.7778.169", intl.ThinSpace | intl.ISOMonthNames | intl.RoundingWindow, intl.PlainValueZone | intl.AmeteMihretExtendedYear},
		{"148.0.7778.218", intl.ThinSpace | intl.RoundingWindow, intl.ISOMonthNames | intl.PlainValueZone},
		{"150", intl.ThinSpace, intl.ISOMonthNames | intl.RoundingWindow | intl.RepeatedMidnight},
		{"155", intl.ThinSpace, intl.SubdivisionHourCycles},
	} {
		p, err := chrome.ForVersion(c.version)
		if err != nil {
			t.Fatal(err)
		}
		if p.Compat&c.has != c.has || p.Compat&c.not != 0 {
			t.Errorf("Chrome %s answers in %v", c.version, p.Compat)
		}
	}
}

// What each profile's data says, as its Chrome writes it
// (testdata/chrome).
func TestProfiles(t *testing.T) {
	when := time.UnixMilli(1704467045000) // 2024-01-05T15:04:05Z
	for _, c := range []struct {
		version, tag string
		opts         intl.DateTimeFormatOptions
		want         string
	}{
		// ICU 77 writes a day period of the "a" field from the wide names.
		{"146", "bg", intl.DateTimeFormatOptions{TimeZone: "UTC", Hour: intl.WidthNumeric, HourCycle: intl.H11}, "3 ч. сл.об."},
		{"155", "bg", intl.DateTimeFormatOptions{TimeZone: "UTC", Hour: intl.WidthNumeric, HourCycle: intl.H11}, "3 ч. pm"},
		// ICU 77 writes an offset of zero as the locale's GMT.
		{"146", "en", intl.DateTimeFormatOptions{TimeZone: "Africa/Accra", Hour: intl.WidthNumeric, TimeZoneName: intl.ZoneLongOffset}, "3 PM GMT"},
		{"155", "en", intl.DateTimeFormatOptions{TimeZone: "Africa/Accra", Hour: intl.WidthNumeric, TimeZoneName: intl.ZoneLongOffset}, "3 PM GMT+00:00"},
		// Chrome's zone tree has no Akan, which reads the default locale's
		// names.
		{"155", "ak", intl.DateTimeFormatOptions{TimeZone: "America/New_York", Hour: intl.WidthNumeric, TimeZoneName: intl.ZoneShort}, "10 AN EST"},
	} {
		p, err := chrome.ForVersion(c.version)
		if err != nil {
			t.Fatal(err)
		}
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		c.opts.Compat = p.Compat
		f, err := intl.NewDateTimeFormatFrom(p.Source, loc, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Format(when); got != c.want {
			t.Errorf("Chrome %s %s: %+q, want %+q", c.version, c.tag, got, c.want)
		}
	}
}
