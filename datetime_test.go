package intl_test

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

func newDateTime(t *testing.T, tag string, opts intl.DateTimeFormatOptions) *intl.DateTimeFormat {
	t.Helper()
	loc, err := intl.ParseLocale(tag)
	if err != nil {
		t.Fatalf("ParseLocale(%q): %v", tag, err)
	}
	f, err := intl.NewDateTimeFormat(loc, opts)
	if err != nil {
		t.Fatalf("NewDateTimeFormat(%q): %v", tag, err)
	}
	return f
}

// Which calendar a locale reckons in is a property of its region rather than
// its language, and the caller may override it either way. The expectations
// are node's.
func TestCalendarSelection(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		tag, calendar string
		want          string
		resolved      string
	}{
		// Thailand reckons in the Buddhist calendar although nothing said so.
		{"th", "", "5 มกราคม 2567", "buddhist"},
		// The locale's own extension overrides its region.
		{"th-u-ca-gregory", "", "5 มกราคม ค.ศ. 2024", "gregory"},
		// And the option overrides everything.
		{"en", "buddhist", "January 5, 2567 BE", "buddhist"},
		{"en", "", "January 5, 2024", "gregory"},
		{"ja", "", "2024年1月5日", "gregory"},
	} {
		f := newDateTime(t, c.tag, intl.DateTimeFormatOptions{
			TimeZone: "UTC", DateStyle: intl.LengthLong, Calendar: c.calendar,
		})
		if got := f.Format(when); got != c.want {
			t.Errorf("%s calendar=%q = %q, want %q", c.tag, c.calendar, got, c.want)
		}
		if got := f.ResolvedOptions().Calendar; got != c.resolved {
			t.Errorf("%s calendar=%q resolved to %q, want %q",
				c.tag, c.calendar, got, c.resolved)
		}
	}
}

// A calendar CLDR has and this does not is refused when the formatter is
// built, rather than answered in the wrong one.
func TestUnimplementedCalendarIsRefused(t *testing.T) {
	loc, err := intl.ParseLocale("fa-IR")
	if err != nil {
		t.Fatal(err)
	}
	for _, calendar := range []string{"persian", "hebrew", "japanese", "islamic"} {
		if _, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
			Calendar: calendar,
		}); err == nil {
			t.Errorf("the %s calendar was accepted", calendar)
		}
	}
}

// A date style and named fields are two ways of asking and ECMA-402 forbids
// mixing them.
func TestDateStyleAndFieldsCannotMix(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
		DateStyle: intl.LengthFull, Year: intl.WidthNumeric,
	}); err == nil {
		t.Error("a date style beside a named field was accepted")
	}
}

// The pieces of a date say what each one is, so a caller can style them apart.
func TestDateTimeParts(t *testing.T) {
	f := newDateTime(t, "en", intl.DateTimeFormatOptions{
		TimeZone: "UTC", Year: intl.WidthNumeric,
		Month: intl.WidthLong, Day: intl.WidthNumeric,
	})
	want := []intl.Part{
		{Kind: intl.PartMonth, Value: "January"},
		{Kind: intl.PartLiteral, Value: " "},
		{Kind: intl.PartDay, Value: "5"},
		{Kind: intl.PartLiteral, Value: ", "},
		{Kind: intl.PartYear, Value: "2024"},
	}
	got := f.FormatToParts(time.UnixMilli(1704467045000))
	if len(got) != len(want) {
		t.Fatalf("got %d parts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d is %v, want %v", i, got[i], want[i])
		}
	}
}
