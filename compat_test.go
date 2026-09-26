package intl_test

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// Each divergence's flag chooses Node's side of it alone, and every flag has
// an entry in Divergences.
func TestCompatFlags(t *testing.T) {
	var all intl.Compat
	for _, d := range intl.Divergences {
		if d.Flag == 0 || all&d.Flag != 0 {
			t.Errorf("%s: flag %d is zero or another's", d.Name, d.Flag)
		}
		all |= d.Flag
		if !intl.NodeICU.Has(d.Flag) || intl.Standard.Has(d.Flag) {
			t.Errorf("%s: not in NodeICU, or in Standard", d.Name)
		}
		if d.Flag.String() != d.Name {
			t.Errorf("%s: String() = %q", d.Name, d.Flag.String())
		}
	}
	if all != intl.NodeICU {
		t.Errorf("the divergences make %d, NodeICU is %d", all, intl.NodeICU)
	}
	if got := (intl.NarrowSpace | intl.TwoLetterTags).String(); got != "NarrowSpace|TwoLetterTags" {
		t.Errorf("String() = %q", got)
	}
}

// hour12 in Japanese: ECMA-402 takes the locale's twelve-hour cycle, which
// Japan's time data gives as K, h11; V8 takes h12 for "ja", which names no
// region. Node's is its answer.
func TestTwelveHourCycle(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		want   string
		cycle  intl.HourCycle
	}{
		{intl.Standard, "午前0:00", intl.H11},
		{intl.TwelveHourCycle, "午前12:00", intl.H12},
	} {
		f := newDateTime(t, "ja", intl.DateTimeFormatOptions{TimeZone: "UTC", Hour: intl.WidthNumeric,
			Minute: intl.Width2Digit, Hour12: intl.Bool(true), Compat: c.compat})
		if got := f.Format(time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC)); got != c.want {
			t.Errorf("%v: %q, want %q", c.compat, got, c.want)
		}
		if got := f.ResolvedOptions().HourCycle; got != c.cycle {
			t.Errorf("%v: hour cycle %v, want %v", c.compat, got, c.cycle)
		}
	}
	// Where the region allows only one twelve-hour cycle, the two agree.
	for _, c := range []intl.Compat{intl.Standard, intl.NodeICU} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, Hour12: intl.Bool(false), Compat: c})
		if got := f.ResolvedOptions().HourCycle; got != intl.H23 {
			t.Errorf("%v: en with hour12 false is %v", c, got)
		}
	}
}

// A year before the Hijrah: CLDR 48.2's era for it, the year counted back,
// or ICU4C's negative year of the era after. Node's is ICU4C's.
func TestIslamicEras(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "127 Before Hijrah"},
		{intl.IslamicEras, "-126 Anno Hegirae"},
	} {
		for _, calendar := range []string{"islamic", "islamic-civil", "islamic-tbla", "islamic-umalqura"} {
			f := newDateTime(t, "en", intl.DateTimeFormatOptions{Calendar: calendar, TimeZone: "UTC",
				Era: intl.WidthLong, Year: intl.WidthNumeric, Compat: c.compat})
			if got := f.Format(time.Date(500, 1, 1, 0, 0, 0, 0, time.UTC)); got != c.want {
				t.Errorf("%v %s: %q, want %q", c.compat, calendar, got, c.want)
			}
		}
	}
	// After the Hijrah the two agree.
	for _, c := range []intl.Compat{intl.Standard, intl.NodeICU} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{Calendar: "islamic-civil", TimeZone: "UTC",
			Era: intl.WidthShort, Year: intl.WidthNumeric, Compat: c})
		if got := f.Format(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)); got != "1445 AH" {
			t.Errorf("%v: %q", c, got)
		}
	}
}

// Temporal values: an era alone, and an hour cycle with no hour asked for.
// The Node side's expectations are Node's; the standard side's follow the
// Temporal proposal's GetDateTimeFormat.
func TestTemporalFormats(t *testing.T) {
	type value struct {
		kind intl.TemporalKind
		at   time.Time
	}
	instant := value{intl.TemporalInstant, time.UnixMilli(0)}
	date := value{intl.TemporalPlainDate, time.Date(2000, 5, 2, 0, 0, 0, 0, time.UTC)}
	dateTime := value{intl.TemporalPlainDateTime, time.Date(2000, 5, 2, 14, 46, 0, 0, time.UTC)}
	yearMonth := value{intl.TemporalPlainYearMonth, time.Date(2000, 5, 1, 0, 0, 0, 0, time.UTC)}
	clock := value{intl.TemporalPlainTime, time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)}
	any_ := [2]intl.DateTimeComponents{intl.ComponentsAny, intl.ComponentsAll}
	dates := [2]intl.DateTimeComponents{intl.ComponentsDate, intl.ComponentsDate}
	times := [2]intl.DateTimeComponents{intl.ComponentsTime, intl.ComponentsTime}
	for _, c := range []struct {
		name           string
		opts           intl.DateTimeFormatOptions
		method         [2]intl.DateTimeComponents
		value          value
		standard, node string
	}{
		{"era, Instant", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, any_, instant,
			"1/1/1970 A, 12:00:00 AM", "A"},
		{"era, PlainDate", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, dates, date, "5/2/2000 A", "A"},
		{"era, PlainDateTime", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, any_, dateTime,
			"5/2/2000 A, 2:46:00 PM", "A"},
		{"era, PlainYearMonth", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, dates, yearMonth, "5/2000 A", "A"},
		{"era, Intl.DateTimeFormat", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, [2]intl.DateTimeComponents{},
			date, "5/2/2000 A", "A"},
		{"hour12 false, PlainTime", intl.DateTimeFormatOptions{Hour12: intl.Bool(false)}, times, clock,
			"00:00:00", "12:00:00 AM"},
		{"hour12 false, Instant", intl.DateTimeFormatOptions{Hour12: intl.Bool(false)}, any_, instant,
			"1/1/1970, 00:00:00", "1/1/1970, 12:00:00 AM"},
		{"h24, PlainTime", intl.DateTimeFormatOptions{HourCycle: intl.H24}, times, clock,
			"24:00:00", "12:00:00 AM"},
		{"h11, PlainTime", intl.DateTimeFormatOptions{HourCycle: intl.H11}, times, clock,
			"0:00:00 AM", "12:00:00 AM"},
	} {
		for _, compat := range []intl.Compat{intl.Standard, intl.TemporalFormats} {
			opts := c.opts
			opts.TimeZone, opts.Compat = "UTC", compat
			opts.Required, opts.Defaults = c.method[0], c.method[1]
			f := newDateTime(t, "en", opts)
			k, err := f.ForTemporal(c.value.kind)
			if err != nil {
				t.Fatalf("%s %v: %v", c.name, compat, err)
			}
			want := c.standard
			if compat != intl.Standard {
				want = c.node
			}
			if got := k.Format(c.value.at); got != want {
				t.Errorf("%s %v: %+q, want %+q", c.name, compat, got, want)
			}
		}
	}
}

// "yes" as a keyword's value: UTS #35 makes it "true", and leaves that out,
// only for the keys whose BCP 47 data has the alias; ICU does it for every
// key. The expectations are test262's (unicode-ext-canonicalize-yes-to-true)
// and Node's.
func TestYesValues(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		want   []string
	}{
		{intl.Standard, []string{"und-u-kb", "und-u-kc", "und-u-kh", "und-u-kk", "und-u-kn",
			"und-u-ka-yes", "und-u-kf-yes", "und-u-kr-yes", "und-u-ks-yes", "und-u-kv-yes"}},
		{intl.YesValues, []string{"und-u-kb", "und-u-kc", "und-u-kh", "und-u-kk", "und-u-kn",
			"und-u-ka", "und-u-kf", "und-u-kr", "und-u-ks", "und-u-kv"}},
	} {
		canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		for i, key := range []string{"kb", "kc", "kh", "kk", "kn", "ka", "kf", "kr", "ks", "kv"} {
			l, err := canon.Canonicalize("und-u-" + key + "-yes")
			if err != nil {
				t.Fatal(err)
			}
			if got := l.String(); got != c.want[i] {
				t.Errorf("%v: und-u-%s-yes is %q, want %q", c.compat, key, got, c.want[i])
			}
		}
	}
}

// A currency Intl.supportedValuesOf does not list: named only on Node's
// side, as test262's currencies-accepted-by-DisplayNames requires of the
// standard. The Node side's expectation is Node's.
func TestCurrencyNames(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	for _, c := range []struct {
		compat    intl.Compat
		adp, name string
		found     bool
	}{
		{intl.Standard, "ADP", "", false},
		{intl.CurrencyNames, "ADP", "Andorran Peseta", true},
	} {
		d, err := intl.NewDisplayNames(loc, intl.DisplayNamesOptions{Kind: intl.DisplayCurrency,
			Fallback: intl.FallbackNone, Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := d.Of(c.adp); got != c.name || ok != c.found {
			t.Errorf("%v: %s is %q %v, want %q %v", c.compat, c.adp, got, ok, c.name, c.found)
		}
		// Every listed currency is named on both sides.
		codes, err := intl.Currencies()
		if err != nil {
			t.Fatal(err)
		}
		for _, code := range codes {
			if _, ok := d.Of(code); !ok {
				t.Errorf("%v: listed %s has no name", c.compat, code)
			}
		}
	}
}

// Numeric minutes after numeric hours that are left out: a group of their
// own, as test262's digital-style-with-hours-display-auto-with-zero-hour
// requires of the standard, or joined to what came before, as Node writes.
func TestDurationSeparator(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	for _, c := range []struct {
		compat intl.Compat
		want   [3]string
	}{
		{intl.Standard, [3]string{"1 day, 01:02", "-1 day, 01:02", "01:02"}},
		{intl.DurationSeparator, [3]string{"1 day:01:02", "-1 day:01:02", "01:02"}},
	} {
		opts := intl.DurationFormatOptions{Style: intl.DurationDigital, Compat: c.compat}
		opts.Display[intl.DurationHours] = intl.DurationDisplayAuto
		f, err := intl.NewDurationFormat(loc, opts)
		if err != nil {
			t.Fatal(err)
		}
		for i, d := range []intl.Duration{
			{intl.DurationDays: 1, intl.DurationMinutes: 1, intl.DurationSeconds: 2},
			{intl.DurationDays: -1, intl.DurationMinutes: -1, intl.DurationSeconds: -2},
			{intl.DurationMinutes: 1, intl.DurationSeconds: 2},
		} {
			if got, err := f.Format(d); err != nil || got != c.want[i] {
				t.Errorf("%v %v: %q, %v; want %q", c.compat, d, got, err, c.want[i])
			}
		}
	}
}

// A pattern whose quoted literal holds a field's letter: Portuguese writes a
// long month and a year "MMMM 'de' y". The standard reports the fields of
// the format chosen; Node finds a day in the "de".
func TestLiteralFields(t *testing.T) {
	loc, _ := intl.ParseLocale("pt")
	for _, c := range []struct {
		compat intl.Compat
		day    intl.FieldWidth
	}{
		{intl.Standard, intl.WidthNone},
		{intl.LiteralFields, intl.WidthNumeric},
	} {
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
			Year: intl.WidthNumeric, Month: intl.WidthLong, Compat: c.compat,
		})
		if err != nil {
			t.Fatal(err)
		}
		r := f.ResolvedOptions()
		if r.Year != intl.WidthNumeric || r.Month != intl.WidthLong || r.Day != c.day {
			t.Errorf("%v: year %v month %v day %v, want %v %v %v", c.compat,
				r.Year, r.Month, r.Day, intl.WidthNumeric, intl.WidthLong, c.day)
		}
	}
}

// The calendars ECMA-402 deprecates: the standard settles them on one
// AvailableCalendars lists, as test262's
// constructor-options-calendar-islamic-fallback requires, and writes in it;
// Node keeps them.
func TestIslamicFallback(t *testing.T) {
	for _, c := range []struct {
		tag, calendar string
		compat        intl.Compat
		want          string
	}{
		{"en", "islamic", intl.Standard, "islamic-civil"},
		{"en", "islamic-rgsa", intl.Standard, "islamic-civil"},
		{"en-u-ca-islamic", "", intl.Standard, "islamic-civil"},
		{"en", "islamic", intl.IslamicFallback, "islamic"},
		{"en", "islamic-rgsa", intl.IslamicFallback, "islamic-rgsa"},
		{"en-u-ca-islamic", "", intl.IslamicFallback, "islamic"},
	} {
		loc, _ := intl.ParseLocale(c.tag)
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{Calendar: c.calendar, Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		if got := f.ResolvedOptions().Calendar; got != c.want {
			t.Errorf("%s %q %v: %s, want %s", c.tag, c.calendar, c.compat, got, c.want)
		}
		// The formatter writes in the calendar it reports.
		loc, _ = intl.ParseLocale("en-u-ca-" + c.want)
		want, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		at := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
		if got, w := f.Format(at), want.Format(at); got != w {
			t.Errorf("%s %q %v: wrote %q, want %q", c.tag, c.calendar, c.compat, got, w)
		}
	}
}

// A resolved locale's -u-hc keyword where hour12 or hourCycle was given:
// ResolveLocale drops it for any hour12 and for another hourCycle; V8
// drops it where the formatter's own hour cycle, none without an hour, is
// another.
func TestHourCycleKeyword(t *testing.T) {
	loc, _ := intl.ParseLocale("en-u-hc-h23")
	for _, c := range []struct {
		opts intl.DateTimeFormatOptions
		want [2]string // Standard, HourCycleKeyword
	}{
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, Hour12: intl.Bool(false)}, [2]string{"en", "en-u-hc-h23"}},
		{intl.DateTimeFormatOptions{Hour12: intl.Bool(false)}, [2]string{"en", "en"}},
		{intl.DateTimeFormatOptions{HourCycle: intl.H23}, [2]string{"en-u-hc-h23", "en"}},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, HourCycle: intl.H23}, [2]string{"en-u-hc-h23", "en-u-hc-h23"}},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, HourCycle: intl.H11}, [2]string{"en", "en"}},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric}, [2]string{"en-u-hc-h23", "en-u-hc-h23"}},
	} {
		for i, compat := range []intl.Compat{intl.Standard, intl.HourCycleKeyword} {
			opts := c.opts
			opts.Compat = compat
			f, err := intl.NewDateTimeFormat(loc, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := f.ResolvedOptions().Locale; got != c.want[i] {
				t.Errorf("%+v %v: %s, want %s", c.opts, compat, got, c.want[i])
			}
		}
	}
}

// A plain Temporal value whose wall-clock time the formatter's zone skips:
// the standard writes its fields as they are, as test262's
// PlainDate/prototype/toLocaleString/ignore-timezone and its PlainDateTime
// counterpart require; Node writes the instant they name in the zone.
func TestPlainValueZone(t *testing.T) {
	loc, _ := intl.ParseLocale("en-US")
	for _, c := range []struct {
		zone   string
		kind   intl.TemporalKind
		fields [6]int
		want   [2]string // Standard, PlainValueZone
	}{
		{"Pacific/Apia", intl.TemporalPlainDate, [6]int{2011, 12, 30},
			[2]string{"12/30/2011", "12/31/2011"}},
		{"America/Los_Angeles", intl.TemporalPlainDateTime, [6]int{2026, 3, 8, 2, 30},
			[2]string{"3/8/2026, 2:30:00 AM", "3/8/2026, 3:30:00 AM"}},
		{"America/Los_Angeles", intl.TemporalPlainDateTime, [6]int{2026, 7, 8, 2, 30},
			[2]string{"7/8/2026, 2:30:00 AM", "7/8/2026, 2:30:00 AM"}},
	} {
		for i, compat := range []intl.Compat{intl.Standard, intl.PlainValueZone} {
			opts := intl.DateTimeFormatOptions{TimeZone: c.zone, Compat: compat}
			if c.kind == intl.TemporalPlainDate {
				opts.Required, opts.Defaults = intl.ComponentsDate, intl.ComponentsDate
			} else {
				opts.Required, opts.Defaults = intl.ComponentsAny, intl.ComponentsAll
			}
			f, err := intl.NewDateTimeFormat(loc, opts)
			if err != nil {
				t.Fatal(err)
			}
			kf, err := f.ForTemporal(c.kind)
			if err != nil {
				t.Fatal(err)
			}
			v := c.fields
			if got := kf.Format(f.PlainInstant(v[0], time.Month(v[1]), v[2], v[3], v[4], v[5], 0)); got != c.want[i] {
				t.Errorf("%s %v %v: %q, want %q", c.zone, v, compat, got, c.want[i])
			}
		}
	}
}
