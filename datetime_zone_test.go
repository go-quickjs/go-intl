package intl_test

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// The names ICU's TimeZoneFormat gives zones, in the cases go-intl once got
// wrong: an offset of zero written as the locale's "GMT" rather than
// "GMT+0"; a specific name falling back to a generic one; a generic name that
// is really a location ("United Kingdom Time") or a standard name ("India
// Standard Time"); and a locale ICU keeps no zone bundle for, which falls
// back by ICU's resource rules rather than CLDR's.
func TestZoneNamesAsICU(t *testing.T) {
	winter := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		tag, zone string
		style     intl.ZoneStyle
		want      string
	}{
		{"en", "UTC", intl.ZoneShortOffset, "GMT+0"},
		{"en", "UTC", intl.ZoneLongOffset, "GMT+00:00"},
		{"en", "UTC", intl.ZoneShort, "UTC"},
		{"en", "UTC", intl.ZoneLongGeneric, "GMT+00:00"},
		{"fr", "Europe/London", intl.ZoneShort, "UTC+0"},
		{"af", "Europe/London", intl.ZoneShort, "GMT+0"},
		{"en", "Europe/London", intl.ZoneShort, "GMT"},
		{"en", "Europe/London", intl.ZoneShortGeneric, "United Kingdom Time"},
		{"de", "Europe/London", intl.ZoneLongGeneric, "Vereinigtes Königreich (Ortszeit)"},
		{"en", "Africa/Abidjan", intl.ZoneLongGeneric, "Greenwich Mean Time"},
		{"de", "Africa/Abidjan", intl.ZoneShortGeneric, "Côte d’Ivoire (Ortszeit)"},
		{"en", "Asia/Kolkata", intl.ZoneLongGeneric, "India Standard Time"},
		{"en", "America/New_York", intl.ZoneShortGeneric, "ET"},
		{"en", "Australia/Lord_Howe", intl.ZoneShortGeneric, "Lord Howe Island Time"},
		{"en", "Etc/GMT+5", intl.ZoneShort, "GMT-5"},
		{"sr-Cyrl-ME", "America/New_York", intl.ZoneLong, "Severnoameričko istočno standardno vreme"},
	} {
		f := newDateTime(t, c.tag, intl.DateTimeFormatOptions{
			TimeZone: c.zone, Hour: intl.WidthNumeric, TimeZoneName: c.style,
		})
		var got string
		for _, p := range f.FormatToParts(winter) {
			if p.Kind == intl.PartTimeZoneName {
				got = p.Value
			}
		}
		if got != c.want {
			t.Errorf("%s %s style %d = %+q, want %+q", c.tag, c.zone, c.style, got, c.want)
		}
	}
}

// ICU writes the noon period only when the time as written is exactly noon:
// a pattern without seconds calls 12:00:30 noon, one with them does not. It
// never writes midnight, which it finds ambiguous, and calls the hour by its
// part of the day instead.
func TestDayPeriodNoon(t *testing.T) {
	noon := time.UnixMilli(1704456000000)
	noonAndABit := time.UnixMilli(1704456030000)
	midnight := time.UnixMilli(1704412800000)
	long := intl.DateTimeFormatOptions{TimeZone: "UTC", DayPeriod: intl.WidthLong}
	seconds := intl.DateTimeFormatOptions{TimeZone: "UTC", Hour: intl.WidthNumeric,
		Minute: intl.Width2Digit, Second: intl.Width2Digit, DayPeriod: intl.WidthShort}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		when time.Time
		want string
	}{
		{"en", long, noon, "noon"},
		{"en", long, noonAndABit, "noon"},
		{"en", long, midnight, "in the morning"},
		{"en", seconds, noon, "12:00:00 noon"},
		{"en", seconds, noonAndABit, "12:00:30 in the afternoon"},
		{"am", long, noon, "ቀትር"},
		{"az", intl.DateTimeFormatOptions{TimeZone: "UTC", DayPeriod: intl.WidthNarrow}, noon, "g"},
	} {
		if got := newDateTime(t, c.tag, c.opts).Format(c.when); got != c.want {
			t.Errorf("%s %v = %+q, want %+q", c.tag, c.when, got, c.want)
		}
	}
}
