package intl_test

import (
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
