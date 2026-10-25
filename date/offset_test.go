package date

import (
	"math"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// getTimezoneOffset counts the minutes and their fraction where an offset
// has seconds, as ECMA-262 has it and Chrome from 156 answers: Abidjan's
// local mean time in 1900 is 16 minutes and 8 seconds. V8 before 15.6, and
// so Node, keeps the offset in whole minutes, truncated
// (intl.WholeMinuteOffset). An offset of whole minutes is the same either
// way, and toString writes the minutes truncated in both.
func TestWholeMinuteOffset(t *testing.T) {
	tz, err := intl.LoadTimeZone(intl.Embedded, "Africa/Abidjan")
	if err != nil {
		t.Fatal(err)
	}
	en, err := intl.ParseLocale("en-US")
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1790340029924)
	const lmt, modern = -2208988800000, 1704467045000 // 1900, 2024
	for _, c := range []struct {
		compat      intl.Compat
		lmt, modern float64
	}{
		{intl.Standard, 968000.0 / msPerMinute, 0},
		{intl.NodeICU, 16, 0},
		{intl.NodeICU &^ intl.WholeMinuteOffset, 968000.0 / msPerMinute, 0},
	} {
		e, err := New(Options{Locale: &en, TimeZone: tz, Now: func() time.Time { return now }, Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		if got := e.TimezoneOffset(lmt); got != c.lmt {
			t.Errorf("%v: 1900's offset %v, want %v", c.compat, got, c.lmt)
		}
		if got := e.TimezoneOffset(modern); got != c.modern {
			t.Errorf("%v: 2024's offset %v, want %v", c.compat, got, c.modern)
		}
		if got := e.TimezoneOffset(math.NaN()); !math.IsNaN(got) {
			t.Errorf("%v: NaN's offset %v", c.compat, got)
		}
		if got, want := e.String(lmt), "Sun Dec 31 1899 23:43:52 GMT-0016 (Greenwich Mean Time)"; got != want {
			t.Errorf("%v: %q, want %q", c.compat, got, want)
		}
	}
}
