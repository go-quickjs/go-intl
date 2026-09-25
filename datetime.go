package intl

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/numdata"
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

// ZoneStyle is how a time zone is named, ECMA-402's timeZoneName.
type ZoneStyle int

const (
	ZoneNone ZoneStyle = iota
	// ZoneShort and ZoneLong are the specific names, "EST" and "Eastern
	// Standard Time", which say which half of the year it is.
	ZoneShort
	ZoneLong
	// ZoneShortOffset and ZoneLongOffset are the distance from UTC,
	// "GMT-5" and "GMT-05:00".
	ZoneShortOffset
	ZoneLongOffset
	// ZoneShortGeneric and ZoneLongGeneric are the names whatever the
	// season, "ET" and "Eastern Time".
	ZoneShortGeneric
	ZoneLongGeneric
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
	TimeZoneName ZoneStyle

	// DayPeriod names the part of the day, "in the afternoon", at a width:
	// short, long or narrow.
	DayPeriod FieldWidth
	// FractionalSecondDigits writes one to three digits of the second's
	// fraction. Zero writes none.
	FractionalSecondDigits int

	// TimeZone is the zone to write the instant in. Empty means UTC.
	TimeZone string
	// HourCycle chooses how the hours are counted, and Hour12 overrides it
	// when set.
	HourCycle HourCycle
	Hour12    *bool

	// Calendar is the calendar to reckon in. Empty means the locale's own,
	// which so far is always the Gregorian one.
	Calendar string

	// NumberingSystem names the digits to write, as for NumberFormat.
	NumberingSystem string

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

	tz *timeZone
	// zoneName is the zone as resolvedOptions reports it, and zoneID the
	// identifier its names are found by, empty for a zone that has none.
	zoneName string
	zoneID   string

	// pattern is the pattern this formatter writes, already taken apart.
	pattern datePattern
	// ranges writes a range of two moments, as ICU's interval formatter.
	ranges *rangeFormat
	// week is the region's week conventions, read when the pattern has a
	// week-based year.
	week weekRules
	// rules are the tables the calendar reckons with, read when it is the
	// one in use.
	rules calendarRules
	// hourCycle is the cycle resolvedOptions reports, unset unless an hour
	// or a time style was asked for.
	hourCycle HourCycle
	decimal   string
	minus     string
	// overrides are the numbering systems some fields of a style pattern
	// are written in, by pattern letter; nil for most.
	overrides map[byte]string
	// systems are the numeric numbering systems, read when an override
	// names one.
	systems []numdata.NumberingSystem
	// rbnf are the rule-based numbering systems overrides name, by name.
	rbnf map[string]rbnfSystem
	// zones are the locale's zone names and zone the formatter's zone as
	// they see it.
	zones *zoneNames
	zone  zoneInfo
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
	system, keep, err := chooseCalendar(src, loc, opts.Calendar)
	if err != nil {
		return nil, err
	}
	cal, ok, err := data.Calendar(string(system))
	if err != nil {
		return nil, fmt.Errorf("intl: the dates for %s: %w", loc, err)
	}
	if !ok {
		// A locale with no data for the calendar its region uses falls back to
		// the Gregorian one rather than to nothing.
		if cal, ok, err = data.Calendar(string(Gregory)); err != nil {
			return nil, fmt.Errorf("intl: the dates for %s: %w", loc, err)
		}
		if !ok {
			return nil, fmt.Errorf("intl: %s has no calendar data: %w", loc, ErrNotFound)
		}
		system, keep = Gregory, ""
	}

	// Of the Unicode extension, DateTimeFormat uses the calendar, the hour
	// cycle and the numbering system.
	f := &DateTimeFormat{locale: loc.onlyKeywords("hc", "nu").withKeyword("ca", keep), opts: opts, data: data, calendar: cal, system: system}
	if system == Japanese {
		if f.rules.eras, err = loadJapaneseEras(src); err != nil {
			return nil, err
		}
	}
	if system == IslamicUmmAlQura {
		if f.rules.ummAlQura, err = loadUmmAlQura(src); err != nil {
			return nil, err
		}
	}
	if f.tz, f.zoneName, f.zoneID, err = loadZone(src, opts.TimeZone); err != nil {
		return nil, err
	}
	if numbers, err := loadNumbers(src, loc); err == nil {
		chosen, nu, err := selectNumberingSystem(src, numbers, loc, opts.NumberingSystem)
		if err != nil {
			return nil, err
		}
		f.numbers = &numberDigits{digits: chosen.Digits, system: chosen.NumberingSystem}
		f.locale = f.locale.withKeyword("nu", nu)
		f.decimal = chosen.Symbols.Decimal
		f.minus = chosen.Symbols.MinusSign
	}

	pattern, cycle, g, err := f.choosePattern(src, f.decimal)
	if err != nil {
		return nil, err
	}
	f.hourCycle = cycle
	f.pattern = compileDatePattern(pattern, f.overrides)
	if f.ranges, err = f.newRangeFormat(src, g, pattern); err != nil {
		return nil, err
	}
	hasZone, hasWeek := false, false
	for _, fd := range f.pattern.fields {
		hasZone = hasZone || datePartKind(fd.letter) == PartTimeZoneName
		hasWeek = hasWeek || fd.letter == 'Y'
	}
	if hasWeek {
		f.week = loadWeekRules(src, loc)
		if system == ISO8601 {
			// ISO8601Calendar's weeks start on Monday and need four days,
			// wherever the locale is.
			f.week = weekRules{firstDay: 1, minDays: 4}
		}
	}
	// Most patterns write no zone, and the zone names are much the largest
	// thing a formatter would otherwise read.
	if hasZone {
		if f.zones, err = loadZoneNames(src, loc); err != nil {
			return nil, err
		}
		f.zone = f.zones.zone(f.zoneID, f.tz)
	}
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
		if o.DayPeriod != WidthNone || o.Hour != WidthNone || o.Minute != WidthNone ||
			o.Second != WidthNone || o.FractionalSecondDigits != 0 {
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
		o.Minute != WidthNone || o.Second != WidthNone || o.TimeZoneName != ZoneNone ||
		o.DayPeriod != WidthNone || o.FractionalSecondDigits != 0
}

func loadDates(src Source, loc Locale) (*datedata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.ChainIn(treeLocales, loc.Data())
	}
	shared, err := src.Open(MarkerDatesShared, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the shared date data: %w", err)
	}
	pool, err := blob.ReadShared(shared, datedata.Version)
	if err != nil {
		return nil, fmt.Errorf("intl: the shared date data: %w", err)
	}
	for _, d := range chain {
		b, err := src.Open(MarkerDates, d)
		if err != nil {
			continue
		}
		l, err := datedata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the dates for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no date data for %s: %w", loc, ErrNotFound)
}

// loadZone finds a time zone: a named one, or an offset from UTC. An empty
// name is UTC, which is what ECMA-402 falls back to when the host says
// nothing. It returns the zone, the name resolvedOptions reports, and the
// identifier the zone is named by.
func loadZone(src Source, name string) (*timeZone, string, string, error) {
	if name == "" {
		name = "UTC"
	}
	if seconds, resolved, ok := parseOffsetZone(name); ok {
		// V8 hands ICU an offset as a custom zone, "GMT+05:30", which has
		// no names and so is written as its offset. ICU spells a custom
		// zone of no offset "GMT", which is Etc/GMT and has the names of
		// Greenwich Mean Time.
		id := ""
		if seconds == 0 {
			id = "GMT"
		}
		return fixedZone(seconds), resolved, id, nil
	}
	z, err := loadTimeZone(src, name)
	if err != nil {
		return nil, "", "", err
	}
	return z, z.resolvedID(), z.id, nil
}

// parseOffsetZone reads an offset time zone as ECMA-402 allows one: a sign
// and two digits of hours, then optionally two of minutes, with or without a
// colon -- "+05:30", "+0530", "-08". It returns the offset in seconds and
// the form resolvedOptions reports, "+05:30"; minus zero is "+00:00".
func parseOffsetZone(s string) (int, string, bool) {
	if len(s) < 3 || s[0] != '+' && s[0] != '-' {
		return 0, "", false
	}
	digits := s[1:]
	switch {
	case len(digits) == 2:
		digits += "00"
	case len(digits) == 5 && digits[2] == ':':
		digits = digits[:2] + digits[3:]
	case len(digits) != 4:
		return 0, "", false
	}
	for i := 0; i < 4; i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, "", false
		}
	}
	hours := int(digits[0]-'0')*10 + int(digits[1]-'0')
	minutes := int(digits[2]-'0')*10 + int(digits[3]-'0')
	if hours > 23 || minutes > 59 {
		return 0, "", false
	}
	seconds := hours*3600 + minutes*60
	sign := s[0]
	if seconds == 0 {
		sign = '+'
	} else if sign == '-' {
		seconds = -seconds
	}
	return seconds, string(sign) + digits[:2] + ":" + digits[2:], true
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
// number writes a numeric field, in the numbering system an override gives
// its letter or else the formatter's.
// signedNumber is number for a year that may be before the first: the
// extended year of 1 BC is 0, and of 2 BC -1.
func (f *DateTimeFormat) signedNumber(p *dateParts, letter byte, v, width int) string {
	if v < 0 {
		minus := f.minus
		if minus == "" {
			minus = "-"
		}
		return minus + f.number(p, letter, -v, width)
	}
	return f.number(p, letter, v, width)
}

func (f *DateTimeFormat) number(p *dateParts, letter byte, v, width int) string {
	if system, ok := p.overrides[letter]; ok {
		if rules, ok := f.rbnf[system]; ok {
			// SimpleDateFormat writes a Hebrew year of this millennium
			// without its thousands: 5784 is תשפ״ד.
			if system == "hebr" && (letter == 'y' || letter == 'Y') && v > 5000 && v < 6000 {
				v -= 5000
			}
			return rules.rules.format(rules.set, int64(v))
		}
		for _, s := range f.systems {
			if s.Name == system {
				return mapDigits(pad(v, width), s.Digits)
			}
		}
	}
	return f.digits(pad(v, width))
}

// rbnfSystem is a rule-based numbering system and the rule set it starts at.
type rbnfSystem struct {
	rules *rbnfRules
	set   string
}

// The fields an override without a letter applies to, ICU's kDateFields and
// kTimeFields as pattern letters.
const (
	overrideDateLetters = "yMdDFwWYugLqQUr"
	overrideTimeLetters = "kHmsSKhA"
)

// parseNumberingOverride reads CLDR's override for a style pattern: either a
// system for every date or time field, or "letter=system" pairs separated by
// semicolons.
func parseNumberingOverride(into map[byte]string, s string, date bool) {
	if s == "" {
		return
	}
	for _, part := range strings.Split(s, ";") {
		letter, system, found := strings.Cut(part, "=")
		if !found {
			letters := overrideTimeLetters
			if date {
				letters = overrideDateLetters
			}
			for i := 0; i < len(letters); i++ {
				into[letters[i]] = part
			}
			continue
		}
		if letter != "" {
			into[letter[0]] = system
		}
	}
}

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
	var out []Part
	for _, seg := range f.render(&f.pattern, f.instant(t)) {
		kind := PartLiteral
		if seg.letter != 0 {
			kind = datePartKind(seg.letter)
		}
		out = append(out, Part{kind, seg.value})
	}
	if f.opts.Compat == NodeICU {
		// V8 writes a plain space wherever ICU writes a narrow no-break
		// one, reverting ICU 72 for the web's sake (Replace202F).
		for i := range out {
			out[i].Value = strings.ReplaceAll(out[i].Value, "\u202f", " ")
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
	cycle := f.hourCycle
	return ResolvedDateTimeFormat{
		Locale:          f.locale.String(),
		Calendar:        string(f.system),
		NumberingSystem: system,
		TimeZone:        f.zoneName,
		HourCycle:       cycle,
	}
}
