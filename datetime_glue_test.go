package intl_test

import (
	"strings"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// Traditional Chinese joins a short date to a time with a thin space and a
// longer one with a plain space, as CLDR says and ICU writes. The thin space
// was once flattened to a plain one in every glue.
func TestDateTimeGlueKeepsThinSpace(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		date intl.DateTimeLength
		want string
	}{
		{intl.LengthShort, "2024/1/5 下午3:04"},
		{intl.LengthMedium, "2024年1月5日 下午3:04"},
	} {
		f := newDateTime(t, "zh-Hant", intl.DateTimeFormatOptions{
			TimeZone: "UTC", DateStyle: c.date, TimeStyle: intl.LengthShort,
		})
		if got := f.Format(when); got != c.want {
			t.Errorf("zh-Hant date style %d = %+q, want %+q", c.date, got, c.want)
		}
	}
}

// ICU joins date fields to time fields with the "atTime" glue, the one a
// whole date and a whole time take, rather than the plain one: Bengali's
// plain glue is a space and its atTime glue a comma. go-intl used the plain
// one and was wrong in thirteen of sixty-four locales checked against Node.
func TestDateTimeFieldsTakeAtTimeGlue(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	fields := intl.DateTimeFormatOptions{TimeZone: "UTC", Year: intl.WidthNumeric,
		Month: intl.WidthNumeric, Day: intl.WidthNumeric, Hour: intl.WidthNumeric, Minute: intl.WidthNumeric,
		Compat: intl.NodeICU}
	long := fields
	long.Month = intl.WidthLong
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want string
	}{
		{"bn", fields, "৫/১/২০২৪, ৩:০৪ PM"},
		{"bs", fields, "5. 1. 2024. u 15:04"},
		{"az", fields, "05.01.2024, 15:04"},
		{"ur", fields, "5/1/2024، 3:04 PM"},
		{"en", long, "January 5, 2024 at 3:04 PM"},
		{"zh-Hant", fields, "2024/1/5 下午3:04"},
	} {
		if got := newDateTime(t, c.tag, c.opts).Format(when); got != c.want {
			t.Errorf("%s = %+q, want %+q", c.tag, got, c.want)
		}
	}
}

// A calendar other than the Gregorian takes its date-and-time glue from the
// locale's Gregorian calendar unless the locale gives it one of its own, as
// ICU looks it up. cldr-json resolves CLDR's locale-relative alias to the
// root's glue instead, which gave Arabic's Buddhist dates a Latin comma.
func TestOtherCalendarsTakeGregorianGlue(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	styles := intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthShort, TimeStyle: intl.LengthShort}
	fields := intl.DateTimeFormatOptions{TimeZone: "UTC", Year: intl.WidthNumeric,
		Month: intl.WidthNumeric, Day: intl.WidthNumeric, Hour: intl.WidthNumeric, Minute: intl.WidthNumeric}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want string
	}{
		{"ar-u-ca-buddhist", styles, "5‏/1‏/2567 BE، 3:04 م"},
		{"ar-u-ca-buddhist", fields, "5‏/1‏/2567 BE، 3:04 م"},
		{"bn-u-ca-buddhist", styles, "৫/১/২৫৬৭ BE, ৩:০৪ PM"},
		// Punjabi is one of two locales that give the Buddhist calendar
		// glue of its own.
		{"pa-u-ca-buddhist", styles, "05/01/2567 ਈ. ਪੂ., 3:04 PM"},
	} {
		if got := newDateTime(t, c.tag, c.opts).Format(when); got != c.want {
			t.Errorf("%s %+v = %+q, want %+q", c.tag, c.opts, got, c.want)
		}
	}
}

// Chrome's V8, from Chrome 148, writes a plain space wherever ICU 78 writes
// a thin one, in a date and in a range; the standard profile keeps CLDR's
// character, and so does Node.
func TestThinSpaceByProfile(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		compat     intl.Compat
		date, span string
	}{
		{intl.Standard, "2024/1/5\u2009下午3:04", "5/1/2024\u2009–\u20097/1/2024"},
		{intl.NodeICU, "2024/1/5\u2009下午3:04", "5/1/2024\u2009–\u20097/1/2024"},
		{intl.NodeICU | intl.ThinSpace, "2024/1/5 下午3:04", "5/1/2024 – 7/1/2024"},
	} {
		f := newDateTime(t, "zh-Hant", intl.DateTimeFormatOptions{
			TimeZone: "UTC", DateStyle: intl.LengthShort, TimeStyle: intl.LengthShort, Compat: c.compat,
		})
		if got := f.Format(when); got != c.date {
			t.Errorf("%v: %+q, want %+q", c.compat, got, c.date)
		}
		r := newDateTime(t, "af", intl.DateTimeFormatOptions{TimeZone: "UTC", Compat: c.compat})
		if got := r.FormatRange(when, when.Add(48*time.Hour)); got != c.span {
			t.Errorf("%v: range %+q, want %+q", c.compat, got, c.span)
		}
	}
}

// Chrome 148 writes no month name in the ISO 8601 calendar; the standard,
// and Node, write the Gregorian calendar's.
func TestISOMonthNamesByProfile(t *testing.T) {
	when := time.Date(1900, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "1900 March 1, Thursday"},
		{intl.NodeICU, "1900 March 1, Thursday"},
		{intl.NodeICU | intl.ISOMonthNames, "1900  1, Thursday"},
	} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{
			Calendar: "iso8601", TimeZone: "UTC", DateStyle: intl.LengthFull, Compat: c.compat,
		})
		if got := f.Format(when); got != c.want {
			t.Errorf("%v: %+q, want %+q", c.compat, got, c.want)
		}
	}
}

// ICU before 78 counts the Amete Alem calendar's weeks in Amete Mihret
// years, which Telugu's "MMMM YYYY" shows; the standard, and Node, count
// them in the calendar's own.
func TestAmeteMihretExtendedYearByProfile(t *testing.T) {
	when := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "\u0c24\u0c39\u0c38\u0c3e\u0c38\u0c4d 7462"},
		{intl.NodeICU, "\u0c24\u0c39\u0c38\u0c3e\u0c38\u0c4d 7462"},
		{intl.NodeICU | intl.AmeteMihretExtendedYear, "\u0c24\u0c39\u0c38\u0c3e\u0c38\u0c4d 1962"},
	} {
		f := newDateTime(t, "te", intl.DateTimeFormatOptions{
			Calendar: "ethioaa", TimeZone: "UTC", Year: intl.WidthNumeric, Month: intl.WidthLong, Compat: c.compat,
		})
		if got := f.Format(when); got != c.want {
			t.Errorf("%v: %+q, want %+q", c.compat, got, c.want)
		}
	}
	// The flag touches no other calendar.
	f := newDateTime(t, "te", intl.DateTimeFormatOptions{
		Calendar: "ethiopic", TimeZone: "UTC", Year: intl.WidthNumeric, Month: intl.WidthLong,
		Compat: intl.NodeICU | intl.AmeteMihretExtendedYear,
	})
	if got, want := f.Format(when), "\u0c24\u0c39\u0c38\u0c3e\u0c38\u0c4d 1962"; got != want {
		t.Errorf("ethiopic: %+q, want %+q", got, want)
	}
}

// ICU before 76 gives a range's pattern generator the time styles
// SimpleDateFormat makes for the -u-hc V8 adds, which Bulgarian's available
// formats write with "ч."; from 76, as Node has it, the bundle's own.
func TestHourCycleStyleTimesByProfile(t *testing.T) {
	from, to := time.UnixMilli(1704467045000), time.UnixMilli(1707296400000)
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "5.01.2024\u202f\u0433., 15:04 \u2013 7.02.2024\u202f\u0433., 9:00"},
		{intl.NodeICU, "5.01.2024 \u0433., 15:04 \u2013 7.02.2024 \u0433., 9:00"},
		{intl.NodeICU | intl.HourCycleStyleTimes, "5.01.2024 \u0433., 15:04 \u0447. \u2013 7.02.2024 \u0433., 09:00 \u0447."},
	} {
		f := newDateTime(t, "bg", intl.DateTimeFormatOptions{
			TimeZone: "UTC", DateStyle: intl.LengthMedium, TimeStyle: intl.LengthShort, Compat: c.compat,
		})
		if got := f.FormatRange(from, to); got != c.want {
			t.Errorf("%v: %+q, want %+q", c.compat, got, c.want)
		}
	}
}

// A pattern that writes the week-based year reports it as the year, as the
// standard reports the fields of the format it chose; Node's V8 reads only
// "y" and reports none.
func TestWeekYearOmittedByProfile(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		year   intl.FieldWidth
	}{
		{intl.Standard, intl.WidthNumeric},
		{intl.NodeICU, intl.WidthNone},
		{intl.NodeICU &^ intl.WeekYearOmitted, intl.WidthNumeric},
	} {
		f := newDateTime(t, "te", intl.DateTimeFormatOptions{
			Calendar: "buddhist", TimeZone: "UTC", Year: intl.WidthNumeric, Month: intl.WidthLong, Compat: c.compat,
		})
		if got := f.ResolvedOptions().Year; got != c.year {
			t.Errorf("%v: year %q, want %q", c.compat, got, c.year)
		}
		// The year is written either way.
		if got, want := f.Format(time.UnixMilli(1704467045000)), "\u0c1c\u0c28\u0c35\u0c30\u0c3f 2024"; got != want {
			t.Errorf("%v: %+q, want %+q", c.compat, got, want)
		}
	}
}

// V8 without Temporal, Chrome before 144, keeps the narrow no-break space in
// formatToParts, and in formatRangeToParts of a range that is one moment,
// while its strings write a plain space; the standard keeps it everywhere
// and Node writes a plain space everywhere.
func TestNarrowSpacePartsByProfile(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	join := func(parts []intl.Part) string {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Value)
		}
		return b.String()
	}
	joinRange := func(parts []intl.RangePart) string {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Value)
		}
		return b.String()
	}
	for _, c := range []struct {
		compat                                       intl.Compat
		format, parts, rangeString, oneMoment, apart string
	}{
		{intl.Standard, "3:04\u202fPM", "3:04\u202fPM", "3:04\u2009\u2013\u20094:04\u202fPM", "3:04\u202fPM", "3:04\u2009\u2013\u20094:04\u202fPM"},
		{intl.NodeICU, "3:04 PM", "3:04 PM", "3:04\u2009\u2013\u20094:04 PM", "3:04 PM", "3:04\u2009\u2013\u20094:04 PM"},
		{intl.NodeICU | intl.NarrowSpaceParts, "3:04 PM", "3:04\u202fPM", "3:04\u2009\u2013\u20094:04 PM", "3:04\u202fPM", "3:04\u2009\u2013\u20094:04 PM"},
	} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{TimeZone: "UTC", TimeStyle: intl.LengthShort, Compat: c.compat})
		later := when.Add(time.Hour)
		for _, g := range []struct{ what, got, want string }{
			{"format", f.Format(when), c.format},
			{"formatToParts", join(f.FormatToParts(when)), c.parts},
			{"formatRange", f.FormatRange(when, later), c.rangeString},
			{"formatRangeToParts of one moment", joinRange(f.FormatRangeToParts(when, when)), c.oneMoment},
			{"formatRangeToParts", joinRange(f.FormatRangeToParts(when, later)), c.apart},
		} {
			if g.got != g.want {
				t.Errorf("%v %s: %+q, want %+q", c.compat, g.what, g.got, g.want)
			}
		}
	}
}

// V8 writes a plain space wherever ICU writes a narrow no-break one; the
// standard profile keeps CLDR's character.
func TestNarrowNoBreakSpaceByProfile(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "3:04\u202fPM"},
		{intl.NodeICU, "3:04 PM"},
	} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{
			TimeZone: "UTC", TimeStyle: intl.LengthShort, Compat: c.compat,
		})
		if got := f.Format(when); got != c.want {
			t.Errorf("%v: %+q, want %+q", c.compat, got, c.want)
		}
	}
}

// A locale with no day-period rules of its own takes those of the first
// locale its name truncates to that has some, as ICU's DayPeriodRules does:
// hi-Latn-IN's are hi-Latn's, which have a noon, and not Hindi's, which do
// not. Node 26 writes noon so.
func TestDayPeriodRulesOfATruncation(t *testing.T) {
	noon := time.UnixMilli(1704456000000)
	for _, c := range []struct {
		tag   string
		width intl.FieldWidth
		want  string
	}{
		{"hi-Latn-IN", intl.WidthNarrow, "n"},
		{"hi-Latn-IN", intl.WidthShort, "noon"},
		{"hi-Latn-IN", intl.WidthLong, "Madhyanh"},
		{"hi-Latn", intl.WidthLong, "Madhyanh"},
	} {
		f := newDateTime(t, c.tag, intl.DateTimeFormatOptions{TimeZone: "UTC", DayPeriod: c.width})
		if got := f.Format(noon); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.tag, c.width, got, c.want)
		}
	}
}
