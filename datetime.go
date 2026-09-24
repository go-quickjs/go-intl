package intl

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/zonedata"
)

// Intl.DateTimeFormat.
//
// A date is asked for two ways and they do not mix. A caller may ask for a
// whole date or time at a length -- full, long, medium, short -- and get the
// locale's own pattern for it. Or a caller may name the fields it wants and
// let the locale decide how to arrange them, which is UTS #35's skeleton
// matching. ECMA-402 forbids mixing the two, and so does this.
//
// Only the Gregorian calendar is written so far.

// DateTimeLength is how much of a date or time is written.
type DateTimeLength int

const (
	// LengthNone means the caller did not ask for a whole date or time at a
	// length, and named the fields instead.
	LengthNone DateTimeLength = iota
	LengthFull
	LengthLong
	LengthMedium
	LengthShort
)

// FieldWidth is how one named field is written.
type FieldWidth int

const (
	// WidthNone means the field was not asked for.
	WidthNone FieldWidth = iota
	// WidthNumeric is "1", and Width2Digit "01".
	WidthNumeric
	Width2Digit
	// WidthLong is "January", Short "Jan" and Narrow "J".
	WidthLong
	WidthShort
	WidthNarrow
)

// HourCycle is how the hours are counted.
type HourCycle int

const (
	// HourCycleAuto takes the locale's own, and is the default.
	HourCycleAuto HourCycle = iota
	// H11 counts 0 to 11, H12 1 to 12, H23 0 to 23 and H24 1 to 24.
	H11
	H12
	H23
	H24
)

// DateTimeComponents names a group of fields, for ECMA-402's "required" and
// "defaults": which fields a formatter must be asked for before it stops
// supplying its own, and which it supplies.
type DateTimeComponents int

const (
	// ComponentsUnset takes Intl.DateTimeFormat's: any field is enough, and
	// the date is the default.
	ComponentsUnset DateTimeComponents = iota
	// ComponentsDate is the weekday, year, month and day.
	ComponentsDate
	// ComponentsTime is the day period, hour, minute, second and fraction.
	ComponentsTime
	// ComponentsAny is either, and is only something required.
	ComponentsAny
	// ComponentsAll is both, and is only a default.
	ComponentsAll
)

// DateTimeFormatOptions mirrors the option bag of Intl.DateTimeFormat. Its
// zero value asks for nothing, which ECMA-402 answers with the year, month and
// day.
type DateTimeFormatOptions struct {
	// DateStyle and TimeStyle ask for a whole date or time at a length. They
	// cannot be combined with the fields below.
	DateStyle DateTimeLength
	TimeStyle DateTimeLength

	Weekday      FieldWidth
	Era          FieldWidth
	Year         FieldWidth
	Month        FieldWidth
	Day          FieldWidth
	Hour         FieldWidth
	Minute       FieldWidth
	Second       FieldWidth
	TimeZoneName FieldWidth

	// TimeZone is the zone to write the instant in. Empty means UTC.
	TimeZone string
	// HourCycle chooses how the hours are counted, and Hour12 overrides it
	// when set.
	HourCycle HourCycle
	Hour12    *bool

	// Calendar is the calendar to reckon in. Empty means the locale's own,
	// which so far is always the Gregorian one.
	Calendar string

	// Required and Defaults are what the legacy methods differ in, and are
	// the arguments ECMA-402 passes to CreateDateTimeFormat:
	//
	//	Intl.DateTimeFormat            any   date   (the zero values)
	//	Date.prototype.toLocaleString  any   all
	//	toLocaleDateString             date  date
	//	toLocaleTimeString             time  time
	Required DateTimeComponents
	Defaults DateTimeComponents

	Compat Compat
}

// Bool returns a pointer to v, for the options where not saying anything is
// different from saying false.
func Bool(v bool) *bool { return &v }

// A DateTimeFormat writes instants in one locale. It never changes after it is
// built and is safe for any number of goroutines to share.
type DateTimeFormat struct {
	locale   Locale
	opts     DateTimeFormatOptions
	data     *datedata.Locale
	calendar *datedata.Calendar
	system   CalendarSystem
	numbers  *numberDigits

	location *time.Location
	zoneName string

	// fields is the pattern this formatter writes, already taken apart.
	fields []dateField
	// gmtPattern and gmtHourFormat write a zone as an offset from UTC.
	gmtPattern    string
	gmtHourFormat string
	gmtZero       string
	zones         *zoneNames
	zoneNames     zonedata.Names
	zoneKnown     bool
}

// numberDigits is the little a date formatter needs from the number data: the
// digits to write, which are not the ASCII ones in every locale.
type numberDigits struct {
	digits string
	system string
}

// NewDateTimeFormat builds a formatter from the data built into the package.
func NewDateTimeFormat(loc Locale, opts DateTimeFormatOptions) (*DateTimeFormat, error) {
	return NewDateTimeFormatFrom(Embedded, loc, opts)
}

// NewDateTimeFormatFrom builds a formatter from a source of the caller's own.
func NewDateTimeFormatFrom(src Source, loc Locale, opts DateTimeFormatOptions) (*DateTimeFormat, error) {
	if (opts.DateStyle != LengthNone || opts.TimeStyle != LengthNone) && opts.hasFields() {
		return nil, fmt.Errorf("intl: a date style cannot be combined with named fields")
	}
	if err := opts.applyDefaults(); err != nil {
		return nil, err
	}
	data, err := loadDates(src, loc)
	if err != nil {
		return nil, err
	}
	system, err := chooseCalendar(src, loc, opts.Calendar)
	if err != nil {
		return nil, err
	}
	cal, ok := data.Calendar(string(system))
	if !ok {
		// A locale with no data for the calendar its region uses falls back to
		// the Gregorian one rather than to nothing.
		if cal, ok = data.Calendar(string(Gregory)); !ok {
			return nil, fmt.Errorf("intl: %s has no calendar data: %w", loc, ErrNotFound)
		}
		system = Gregory
	}

	f := &DateTimeFormat{locale: loc, opts: opts, data: data, calendar: cal, system: system}
	if f.location, f.zoneName, err = loadZone(opts.TimeZone); err != nil {
		return nil, err
	}
	if numbers, err := loadNumbers(src, loc); err == nil {
		f.numbers = &numberDigits{digits: numbers.Digits, system: numbers.NumberingSystem}
	}
	if f.zones, err = loadZoneNames(src, loc); err != nil {
		return nil, err
	}
	f.gmtPattern, f.gmtHourFormat = f.zones.gmtFormat, f.zones.hourFormat
	f.gmtZero = f.zones.gmtZero
	if f.gmtZero == "" {
		f.gmtZero = "GMT"
	}
	f.zoneNames, f.zoneKnown = f.zones.namesForZone(f.zoneName)

	pattern, err := f.choosePattern()
	if err != nil {
		return nil, err
	}
	f.fields = parseDatePattern(pattern)
	return f, nil
}

// applyDefaults supplies the fields nobody asked for, as ECMA-402's
// CreateDateTimeFormat does with its required and defaults arguments.
func (o *DateTimeFormatOptions) applyDefaults() error {
	required, defaults := o.Required, o.Defaults
	if required == ComponentsUnset {
		required = ComponentsAny
	}
	if defaults == ComponentsUnset {
		defaults = ComponentsDate
	}
	if required == ComponentsAll || defaults == ComponentsAny {
		return fmt.Errorf("intl: %d cannot be required, nor %d a default", required, defaults)
	}
	// A method that writes only a date refuses a time style, and the other
	// way about.
	if required == ComponentsDate && o.TimeStyle != LengthNone {
		return fmt.Errorf("intl: a time style where only a date is written")
	}
	if required == ComponentsTime && o.DateStyle != LengthNone {
		return fmt.Errorf("intl: a date style where only a time is written")
	}

	need := o.DateStyle == LengthNone && o.TimeStyle == LengthNone
	if required == ComponentsDate || required == ComponentsAny {
		if o.Weekday != WidthNone || o.Year != WidthNone || o.Month != WidthNone || o.Day != WidthNone {
			need = false
		}
	}
	if required == ComponentsTime || required == ComponentsAny {
		if o.Hour != WidthNone || o.Minute != WidthNone || o.Second != WidthNone {
			need = false
		}
	}
	if !need {
		return nil
	}
	if defaults == ComponentsDate || defaults == ComponentsAll {
		o.Year, o.Month, o.Day = WidthNumeric, WidthNumeric, WidthNumeric
	}
	if defaults == ComponentsTime || defaults == ComponentsAll {
		o.Hour, o.Minute, o.Second = WidthNumeric, WidthNumeric, WidthNumeric
	}
	return nil
}

func (o *DateTimeFormatOptions) hasFields() bool {
	return o.Weekday != WidthNone || o.Era != WidthNone || o.Year != WidthNone ||
		o.Month != WidthNone || o.Day != WidthNone || o.Hour != WidthNone ||
		o.Minute != WidthNone || o.Second != WidthNone || o.TimeZoneName != WidthNone
}

func loadDates(src Source, loc Locale) (*datedata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.Chain(loc.Data())
	}
	for _, d := range chain {
		b, err := src.Open(MarkerDates, d)
		if err != nil {
			continue
		}
		l, err := datedata.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the dates for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no date data for %s: %w", loc, ErrNotFound)
}

// loadZone finds a named time zone. An empty name is UTC, which is what
// ECMA-402 falls back to when the host says nothing.
func loadZone(name string) (*time.Location, string, error) {
	if name == "" {
		return time.UTC, "UTC", nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, "", fmt.Errorf("intl: %q is not a time zone: %w", name, err)
	}
	return loc, name, nil
}

// choosePattern settles which pattern this formatter writes.
func (f *DateTimeFormat) choosePattern() (string, error) {
	cal := f.calendar
	date, tim := f.opts.DateStyle, f.opts.TimeStyle
	if date != LengthNone || tim != LengthNone {
		var datePattern, timePattern string
		if date != LengthNone {
			datePattern = cal.DateFormats[date-1]
		}
		if tim != LengthNone {
			timePattern = cal.TimeFormats[tim-1]
		}
		switch {
		case datePattern == "":
			return f.withHourCycle(timePattern), nil
		case timePattern == "":
			return datePattern, nil
		}
		// A whole date beside a whole time takes its own glue, which is not
		// the one a set of fields takes: English joins these with "at".
		glue := cal.AtTimeFormats[date-1]
		if glue == "" {
			glue = cal.DateTimeFormats[date-1]
		}
		if glue == "" {
			glue = "{1}, {0}"
		}
		joined := strings.ReplaceAll(glue, "{1}", datePattern)
		joined = strings.ReplaceAll(joined, "{0}", f.withHourCycle(timePattern))
		return joined, nil
	}

	pattern, err := f.skeletonPattern()
	if err != nil {
		return "", err
	}
	return f.withHourCycle(pattern), nil
}

// withHourCycle rewrites the hour field where the caller asked for a different
// way of counting than the locale's own pattern uses.
func (f *DateTimeFormat) withHourCycle(pattern string) string {
	want := f.hourLetter()
	if want == 0 || pattern == "" {
		return pattern
	}
	twelve := want == 'h' || want == 'K'

	var b strings.Builder
	for _, fd := range parseDatePattern(pattern) {
		switch fd.letter {
		case 'h', 'H', 'K', 'k':
			b.WriteString(strings.Repeat(string(want), fd.count))
		case 'a', 'b', 'B':
			// A twenty-four hour clock has no morning and afternoon, and the
			// space before them goes with them.
			if !twelve {
				continue
			}
			b.WriteString(strings.Repeat(string(fd.letter), fd.count))
		case 0:
			b.WriteString(quoteLiteral(fd.literal))
		default:
			b.WriteString(strings.Repeat(string(fd.letter), fd.count))
		}
	}
	out := b.String()
	if !twelve {
		// Removing the day period leaves the space that stood beside it.
		out = strings.TrimSpace(strings.ReplaceAll(out, " ", " "))
	}
	return out
}

// hourLetter is the pattern letter the options ask for, or zero to leave the
// locale's own alone.
func (f *DateTimeFormat) hourLetter() byte {
	if f.opts.Hour12 != nil {
		if *f.opts.Hour12 {
			return 'h'
		}
		return 'H'
	}
	switch f.opts.HourCycle {
	case H11:
		return 'K'
	case H12:
		return 'h'
	case H23:
		return 'H'
	case H24:
		return 'k'
	}
	return 0
}

// quoteLiteral puts a literal back into a pattern, quoting the letters that
// would otherwise be read as fields.
func quoteLiteral(s string) string {
	if s == "" {
		return ""
	}
	needs := false
	for i := 0; i < len(s); i++ {
		if isPatternLetter(s[i]) || s[i] == '\'' {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// digits writes ASCII digits in the locale's own, where they differ.
func (f *DateTimeFormat) digits(s string) string {
	if f.numbers == nil {
		return s
	}
	return mapDigits(s, f.numbers.digits)
}

// Format writes an instant.
func (f *DateTimeFormat) Format(t time.Time) string {
	var b strings.Builder
	for _, p := range f.FormatToParts(t) {
		b.WriteString(p.Value)
	}
	return b.String()
}

// FormatToParts writes an instant as the pieces it is made of.
func (f *DateTimeFormat) FormatToParts(t time.Time) []Part {
	local := t.In(f.location)
	p := reckon(local, f.system)
	abbr, offset := local.Zone()
	p.zoneOffset = offset
	p.zoneShort, p.zoneLong = f.zoneNamesFor(local, abbr, offset)

	var out []Part
	for _, fd := range f.fields {
		if fd.letter == 0 {
			if fd.literal != "" {
				out = append(out, Part{PartLiteral, fd.literal})
			}
			continue
		}
		if value := f.writeField(fd, &p); value != "" {
			out = append(out, Part{datePartKind(fd.letter), value})
		}
	}
	return out
}

// ResolvedDateTimeFormat is what a formatter settled on.
type ResolvedDateTimeFormat struct {
	Locale          string
	Calendar        string
	NumberingSystem string
	TimeZone        string
	HourCycle       HourCycle
}

// ResolvedOptions returns what the formatter settled on.
func (f *DateTimeFormat) ResolvedOptions() ResolvedDateTimeFormat {
	system := "latn"
	if f.numbers != nil && f.numbers.system != "" {
		system = f.numbers.system
	}
	cycle := f.opts.HourCycle
	if f.opts.Hour12 != nil {
		cycle = H23
		if *f.opts.Hour12 {
			cycle = H12
		}
	}
	return ResolvedDateTimeFormat{
		Locale:          f.locale.String(),
		Calendar:        string(f.system),
		NumberingSystem: system,
		TimeZone:        f.zoneName,
		HourCycle:       cycle,
	}
}
