package chrome_test

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/chrome"
)

// A Chrome answers as ICU does with its language as ICU's default locale,
// which a profile is given with intl.WithDefaultLocale. Akan has dates of
// its own in Chrome's data but no zone, region, currency or unit names, so
// those are the default locale's: "10 AN Eastern Standard Time" in a Chrome
// running in English, "10 AN \ubd81\ubbf8 \ub3d9\ubd80 \ud45c\uc900\uc2dc" in one
// running in Korean, and its name for Ghana in a language's name is the
// default's too. Assamese has no data at all, and a request for it
// resolves to the default locale (V8's ResolveLocale, which a host makes
// with a LocaleMatcher). Each answer is what a Chrome for Testing build
// of the version, started in the language, wrote.
func TestDefaultLocaleProfiles(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		version, language, locale, what, want string
	}{
		{"155", "en-US", "ak", "zone", "10 AN Eastern Standard Time"},
		{"155", "en-US", "ak", "language", "Akan (Ghana)"},
		{"155", "en-US", "ak", "region", "Ghana"},
		{"155", "en-US", "ak", "currency", "2.00 euros"},
		{"155", "en-US", "ak", "unit", "2 kilometers per hour"},
		{"155", "en-US", "as", "zone", "10 AM Eastern Standard Time"},
		{"155", "en-US", "as", "language", "Assamese (Ghana)"},
		{"155", "en-US", "as", "region", "Ghana"},
		{"155", "en-US", "as", "currency", "2.00 euros"},
		{"155", "en-US", "as", "unit", "2 kilometers per hour"},
		{"155", "ko-KR", "ak", "zone", "10 AN \ubd81\ubbf8 \ub3d9\ubd80 \ud45c\uc900\uc2dc"},
		{"155", "ko-KR", "ak", "language", "Akan (\uac00\ub098)"},
		{"155", "ko-KR", "ak", "region", "\uac00\ub098"},
		{"155", "ko-KR", "ak", "currency", "2.00 \uc720\ub85c"},
		{"155", "ko-KR", "ak", "unit", "\uc2dc\uc18d 2\ud0ac\ub85c\ubbf8\ud130"},
		{"155", "ko-KR", "as", "zone", "\uc624\uc804 10\uc2dc \ubd81\ubbf8 \ub3d9\ubd80 \ud45c\uc900\uc2dc"},
		{"155", "ko-KR", "as", "language", "\uc544\uc0bc\uc5b4(\uac00\ub098)"},
		{"155", "ko-KR", "as", "region", "\uac00\ub098"},
		{"155", "ko-KR", "as", "currency", "2.00 \uc720\ub85c"},
		{"155", "ko-KR", "as", "unit", "\uc2dc\uc18d 2\ud0ac\ub85c\ubbf8\ud130"},
		{"142", "en-US", "ak", "zone", "10 AN Eastern Standard Time"},
		{"142", "en-US", "ak", "language", "Akan (Ghana)"},
		{"142", "en-US", "ak", "region", "Ghana"},
		{"142", "en-US", "ak", "currency", "2.00 euros"},
		{"142", "en-US", "ak", "unit", "2 kilometers per hour"},
		{"142", "en-US", "as", "zone", "10 AM Eastern Standard Time"},
		{"142", "en-US", "as", "language", "Assamese (Ghana)"},
		{"142", "en-US", "as", "region", "Ghana"},
		{"142", "en-US", "as", "currency", "2.00 euros"},
		{"142", "en-US", "as", "unit", "2 kilometers per hour"},
		{"142", "ko-KR", "ak", "zone", "10 AN \ubd81\ubbf8 \ub3d9\ubd80 \ud45c\uc900\uc2dc"},
		{"142", "ko-KR", "ak", "language", "Akan (\uac00\ub098)"},
		{"142", "ko-KR", "ak", "region", "\uac00\ub098"},
		{"142", "ko-KR", "ak", "currency", "2.00 \uc720\ub85c"},
		{"142", "ko-KR", "ak", "unit", "\uc2dc\uc18d 2\ud0ac\ub85c\ubbf8\ud130"},
		{"142", "ko-KR", "as", "zone", "\uc624\uc804 10\uc2dc \ubd81\ubbf8 \ub3d9\ubd80 \ud45c\uc900\uc2dc"},
		{"142", "ko-KR", "as", "language", "\uc544\uc0bc\uc5b4(\uac00\ub098)"},
		{"142", "ko-KR", "as", "region", "\uac00\ub098"},
		{"142", "ko-KR", "as", "currency", "2.00 \uc720\ub85c"},
		{"142", "ko-KR", "as", "unit", "\uc2dc\uc18d 2\ud0ac\ub85c\ubbf8\ud130"},
	} {
		p, err := chrome.ForVersion(c.version)
		if err != nil {
			t.Fatal(err)
		}
		def, err := intl.ParseLocale(c.language)
		if err != nil {
			t.Fatal(err)
		}
		src := intl.WithDefaultLocale(p.Source, def)
		requested, err := intl.ParseLocale(c.locale)
		if err != nil {
			t.Fatal(err)
		}
		resolve := func(service intl.Service) intl.Locale {
			m, err := intl.NewLocaleMatcher(src, service)
			if err != nil {
				t.Fatal(err)
			}
			return m.Resolve([]intl.Locale{requested}, intl.BestFit, def)
		}
		var got string
		switch c.what {
		case "zone":
			f, err := intl.NewDateTimeFormatFrom(src, resolve(intl.ServiceDateTimeFormat), intl.DateTimeFormatOptions{
				Hour: intl.WidthNumeric, TimeZoneName: intl.ZoneLong, TimeZone: "America/New_York", Compat: p.Compat})
			if err != nil {
				t.Fatal(err)
			}
			got = f.Format(when)
		case "language", "region":
			kind, code := intl.DisplayLanguage, c.locale+"-GH"
			if c.what == "region" {
				kind, code = intl.DisplayRegion, "GH"
			}
			d, err := intl.NewDisplayNamesFrom(src, resolve(intl.ServiceDisplayNames), intl.DisplayNamesOptions{Kind: kind, Compat: p.Compat})
			if err != nil {
				t.Fatal(err)
			}
			got, _ = d.Of(code)
		case "currency", "unit":
			o := intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", CurrencyDisplay: intl.CurrencyName, Compat: p.Compat}
			if c.what == "unit" {
				o = intl.NumberFormatOptions{Style: intl.StyleUnit, Unit: "kilometer-per-hour", UnitDisplay: intl.UnitLong, Compat: p.Compat}
			}
			f, err := intl.NewNumberFormatFrom(src, resolve(intl.ServiceNumberFormat), o)
			if err != nil {
				t.Fatal(err)
			}
			got = f.Format(2)
		}
		if got != c.want {
			t.Errorf("Chrome %s in %s, %s %s: %+q, want %+q", c.version, c.language, c.locale, c.what, got, c.want)
		}
	}
}
