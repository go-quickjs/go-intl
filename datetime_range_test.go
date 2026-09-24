package intl_test

import (
	"fmt"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// A range is written with the locale's interval pattern for the largest field
// that differs -- the month shared, the days apart -- or, failing one, both
// ends whole; moments that differ in nothing shown are written once. Japanese
// writes a long-date range in its interval pattern for "yMd", which is
// numeric: ICU looks the pattern up by the skeleton, not the style.
func TestFormatRange(t *testing.T) {
	start := time.Date(2024, 1, 5, 15, 4, 5, 0, time.UTC)
	// Node's values, so Node's profile: a plain space before PM.
	date := intl.DateTimeFormatOptions{TimeZone: "UTC", Year: intl.WidthNumeric,
		Month: intl.WidthShort, Day: intl.WidthNumeric, Compat: intl.NodeICU}
	clock := intl.DateTimeFormatOptions{TimeZone: "UTC", Hour: intl.WidthNumeric,
		Minute: intl.Width2Digit, Compat: intl.NodeICU}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		end  time.Time
		want string
	}{
		{"en", date, start.AddDate(0, 0, 2), "Jan 5\u2009\u2013\u20097, 2024"},
		{"en", clock, start.Add(3 * time.Hour), "3:04\u2009\u2013\u20096:04 PM"},
		{"en", clock, start.Add(500 * time.Millisecond), "3:04 PM"},
		{"en", clock, start.AddDate(0, 0, 2), "1/5/2024, 3:04 PM\u2009\u2013\u20091/7/2024, 3:04 PM"},
		{"ja", intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthLong},
			time.Date(2024, 2, 7, 0, 0, 0, 0, time.UTC), "2024/01/05\uff5e2024/02/07"},
	} {
		if got := newDateTime(t, c.tag, c.opts).FormatRange(start, c.end); got != c.want {
			t.Errorf("%s %v = %+q, want %+q", c.tag, c.end, got, c.want)
		}
	}

	var got []string
	for _, p := range newDateTime(t, "en", date).FormatRangeToParts(start, start.AddDate(0, 0, 2)) {
		got = append(got, fmt.Sprintf("%s %q %s", p.Kind, p.Value, p.Source))
	}
	want := []string{
		`month "Jan" shared`, `literal " " shared`, `day "5" startRange`,
		`literal "\u2009–\u2009" shared`, `day "7" endRange`, `literal ", " shared`, `year "2024" shared`,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("parts = %q, want %q", got, want)
	}
}
