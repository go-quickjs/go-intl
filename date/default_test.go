package date

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// Where nothing names the locale, Date names a zone in ICU's default
// locale, the source's: what a Chrome running in Korean writes.
func TestZoneNamedInTheDefaultLocale(t *testing.T) {
	ko, err := intl.ParseLocale("ko-KR")
	if err != nil {
		t.Fatal(err)
	}
	tz, err := intl.LoadTimeZone(intl.Embedded, "Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1792913705306)
	e, err := New(Options{Source: intl.WithDefaultLocale(intl.Embedded, ko), TimeZone: tz, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.String(1704467045000), "Sat Jan 06 2024 00:04:05 GMT+0900 (\ud55c\uad6d \ud45c\uc900\uc2dc)"; got != want {
		t.Errorf("%q, want %q", got, want)
	}
}
