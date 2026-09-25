// Package datedata is the model layer for date and time formatting: the names
// a locale writes dates with and the patterns it writes them in.
//
// Everything here is what CLDR says, kept in CLDR's own shape. The patterns
// stay patterns -- "EEEE, MMMM d, y" -- and are taken apart when a formatter
// is built. A field holding a formatted date would mean the layering had been
// violated.
package datedata

import (
	"fmt"
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 5

// The widths a name may be written at, in the order they are stored.
const (
	Wide = iota
	Abbreviated
	Narrow
	// Short is a fourth width that only weekday names have.
	Short
	Widths
)

// The two contexts a name may appear in. A month inside a date may take a
// different form from one standing on its own, which is what the distinction
// is for: "1 de enero" against "enero".
const (
	Format = iota
	StandAlone
	Contexts
)

// The lengths a whole date or time may be written at.
const (
	Full = iota
	Long
	Medium
	ShortLength
	Lengths
)

// LengthNames are CLDR's names for the lengths, in storage order.
var LengthNames = [Lengths]string{"full", "long", "medium", "short"}

// A Skeleton is one of CLDR's available formats: a request for a set of
// fields, and the pattern that answers it.
type Skeleton struct {
	// ID is the skeleton as CLDR writes it, "yMMMd".
	ID string
	// Pattern is what it resolves to, "MMM d, y".
	Pattern string
}

// A DayPeriod is one of the parts a language divides the day into, beyond the
// two halves: morning, afternoon, evening, night, and the points noon and
// midnight. ID is CLDR's name for it, "morning1".
type DayPeriod struct {
	ID   string
	Text string
}

// A PeriodRule says which part of the day an hour falls in. From and Before
// are minutes past midnight; a rule with At set names a single moment rather
// than a range.
type PeriodRule struct {
	ID     string
	From   int
	Before int
	At     int
	Point  bool
}

// Names holds one set of names, indexed by the thing they name: months by
// number less one, weekdays by day of the week with Sunday first.
type Names struct {
	Text []string
}

// Calendar is everything a locale says about writing dates in one calendar.
type Calendar struct {
	// Months and Days are indexed [context*Widths + width].
	Months [Contexts * Widths]Names
	Days   [Contexts * Widths]Names
	// DayPeriods are indexed by width and hold the two halves of the day.
	AM [Widths]string
	PM [Widths]string
	// Periods are the finer parts of the day, indexed by width and sorted by
	// name within each.
	Periods [Widths][]DayPeriod
	// Eras are indexed by width.
	Eras [Widths]Names

	// DateFormats, TimeFormats and DateTimeFormats are indexed by length.
	// DateTimeFormats is the glue that puts a date and a time together,
	// "{1}, {0}", where {1} is the date.
	DateFormats     [Lengths]string
	TimeFormats     [Lengths]string
	DateTimeFormats [Lengths]string
	// AtTimeFormats is the glue used when a whole date and a whole time were
	// both asked for by length. It differs from the plain one: English joins
	// them with "at" there and with a comma otherwise.
	AtTimeFormats [Lengths]string

	// Available are the skeletons the locale answers, sorted by id.
	Available []Skeleton

	// DateNumbers and TimeNumbers are the numbering overrides of the style
	// patterns, as CLDR writes them: "M=romanlow" has Hawaiian write the
	// month of its short date in lowercase Roman numerals, and "hanidec"
	// would write every number in Chinese digits. Empty for most.
	DateNumbers [Lengths]string
	TimeNumbers [Lengths]string

	// AppendItems say how to add a field a pattern lacks, "{0} {1}" for the
	// zone, by pattern-generator field; see Fields. {0} is the pattern, {1}
	// the field and {2} the field's name.
	AppendItems [Fields]string

	// IntervalFallback joins the two ends of a range no interval pattern
	// covers, "{0} – {1}".
	IntervalFallback string
	// Intervals are the locale's interval patterns, in the order ICU's
	// DateIntervalInfo first stores each skeleton: the locale's own before
	// its parents', sorted within each bundle. The order decides which of
	// two equally good skeletons ICU picks.
	Intervals []Interval

	// LeapMonthPatterns are what the Chinese calendars wrap a leap month's
	// name or number in, "{0}bis", indexed as ICU's DateFormatSymbols
	// indexes them (LeapFormatWide and on). All empty in a calendar without
	// leap months, or one whose locale does not give all seven.
	LeapMonthPatterns [LeapPatterns]string
	// CyclicYears are the names of the sixty years of the cycle, "jia-zi",
	// which the Chinese calendars write for "U": the abbreviated format
	// names, the only ones ICU reads.
	CyclicYears []string
}

// The leap-month patterns, in DateFormatSymbols' order.
const (
	LeapFormatWide = iota
	LeapFormatAbbreviated
	LeapFormatNarrow
	LeapStandAloneWide
	LeapStandAloneAbbreviated
	LeapStandAloneNarrow
	LeapNumeric
	LeapPatterns
)

// IntervalFields is how many fields an interval pattern can be keyed by:
// the largest field that differs between the two ends of the range.
const IntervalFields = 9

// The interval fields, in ICU's order.
const (
	IntervalEra = iota
	IntervalYear
	IntervalMonth
	IntervalDay
	IntervalDayPeriod
	IntervalHour
	IntervalMinute
	IntervalSecond
	IntervalMillisecond
)

// An Interval is the patterns of one skeleton, by the largest field that
// differs; empty where the locale gives none.
type Interval struct {
	Skeleton string
	Patterns [IntervalFields]string
}

// Fields is how many fields ICU's pattern generator distinguishes: era, year,
// quarter, month, week of year, week of month, weekday, day of year, weekday
// of month, day, day period, hour, minute, second, fraction, zone.
const Fields = 16

// Month returns a month's name, counting from one, falling back from a width
// the locale does not give to the wide one.
func (c *Calendar) Month(context, width, month int) string {
	return pick(c.Months[:], context, width, month-1)
}

// Day returns a weekday's name, with Sunday as zero.
func (c *Calendar) Day(context, width, day int) string {
	return pick(c.Days[:], context, width, day)
}

// Era returns an era's name: zero is before the epoch, one after it.
func (c *Calendar) Era(width, era int) string {
	for w := width; w >= Wide; w-- {
		if names := c.Eras[w]; era >= 0 && era < len(names.Text) && names.Text[era] != "" {
			return names.Text[era]
		}
		if w == Wide {
			break
		}
	}
	return ""
}

func pick(sets []Names, context, width, index int) string {
	for _, ctx := range [...]int{context, Format} {
		for w := width; w >= Wide; w-- {
			names := sets[ctx*Widths+w]
			if index >= 0 && index < len(names.Text) && names.Text[index] != "" {
				return names.Text[index]
			}
			if w == Wide {
				break
			}
		}
	}
	return ""
}

// Skeleton finds the pattern for an exact skeleton.
func (c *Calendar) Skeleton(id string) (string, bool) {
	i := sort.Search(len(c.Available), func(i int) bool { return c.Available[i].ID >= id })
	if i < len(c.Available) && c.Available[i].ID == id {
		return c.Available[i].Pattern, true
	}
	return "", false
}

// Locale holds the calendars a locale has data for, keyed by name, and the
// rules that say which part of the day an hour falls in -- which are the
// language's rather than any one calendar's.
type Locale struct {
	Calendars []NamedCalendar
	// PeriodRules are sorted so that the ranges come before the points: a
	// language that names both a range covering midnight and the moment
	// itself is written with the range, which is what ICU does.
	PeriodRules []PeriodRule

	// FieldNames are what the locale calls each field, by pattern-generator
	// field, for an append item that names the field it adds.
	FieldNames [Fields]string

	// DateTimeGlue is the Gregorian calendar's default date-time glue,
	// which ICU's interval formatter joins a date to a time range with
	// whatever the calendar.
	DateTimeGlue string
}

// Period returns the part of the day a time falls in, as minutes past
// midnight. It answers the empty string when the language has no rules.
func (l *Locale) Period(minutes int) string {
	for _, r := range l.PeriodRules {
		if r.Point {
			continue
		}
		// A range that wraps past midnight covers both ends of the day.
		if r.From <= r.Before {
			if minutes >= r.From && minutes < r.Before {
				return r.ID
			}
			continue
		}
		if minutes >= r.From || minutes < r.Before {
			return r.ID
		}
	}
	return ""
}

// HasPoint reports whether the language's rules name a moment of the day,
// "noon" or "midnight".
func (l *Locale) HasPoint(id string) bool {
	for _, r := range l.PeriodRules {
		if r.Point && r.ID == id {
			return true
		}
	}
	return false
}

// PeriodName returns what a calendar calls one part of the day.
func (c *Calendar) PeriodName(width int, id string) string {
	for w := width; w >= Wide; w-- {
		for _, p := range c.Periods[w] {
			if p.ID == id && p.Text != "" {
				return p.Text
			}
		}
		if w == Wide {
			break
		}
	}
	return ""
}

// A NamedCalendar is one calendar and which one it is, "gregory".
type NamedCalendar struct {
	Name     string
	Calendar Calendar
}

// Calendar finds one by name.
func (l *Locale) Calendar(name string) (*Calendar, bool) {
	for i := range l.Calendars {
		if l.Calendars[i].Name == name {
			return &l.Calendars[i].Calendar, true
		}
	}
	return nil, false
}

// Encode writes a locale's calendars.
func Encode(l *Locale) []byte {
	b := blob.NewWriter(Version)
	b.Uint(len(l.PeriodRules))
	for _, r := range l.PeriodRules {
		b.String(r.ID)
		b.Uint(r.From)
		b.Uint(r.Before)
		b.Uint(r.At)
		if r.Point {
			b.Uint(1)
		} else {
			b.Uint(0)
		}
	}
	for _, name := range l.FieldNames {
		b.String(name)
	}
	b.String(l.DateTimeGlue)
	// A calendar a locale says exactly the same of as an earlier one -- the
	// five Islamic calendars share their names and patterns -- is written as
	// the number of that one, counting from one; zero is followed by the
	// calendar itself.
	b.Uint(len(l.Calendars))
	var written []string
	for i := range l.Calendars {
		b.String(l.Calendars[i].Name)
		one := blob.NewWriter(0)
		encodeCalendar(one, &l.Calendars[i].Calendar)
		body := string(one.Bytes()[1:])
		same := 0
		for j, earlier := range written {
			if earlier == body {
				same = j + 1
				break
			}
		}
		written = append(written, body)
		b.Uint(same)
		if same == 0 {
			encodeCalendar(b, &l.Calendars[i].Calendar)
		}
	}
	return b.Bytes()
}

func encodeCalendar(b *blob.Writer, c *Calendar) {
	for _, sets := range [][]Names{c.Months[:], c.Days[:]} {
		for _, n := range sets {
			b.Uint(len(n.Text))
			for _, s := range n.Text {
				b.String(s)
			}
		}
	}
	for w := 0; w < Widths; w++ {
		b.String(c.AM[w])
		b.String(c.PM[w])
		b.Uint(len(c.Periods[w]))
		for _, p := range c.Periods[w] {
			b.String(p.ID)
			b.String(p.Text)
		}
	}
	for _, n := range c.Eras {
		b.Uint(len(n.Text))
		for _, s := range n.Text {
			b.String(s)
		}
	}
	for _, set := range [][Lengths]string{c.DateFormats, c.TimeFormats,
		c.DateTimeFormats, c.AtTimeFormats} {
		for _, s := range set {
			b.String(s)
		}
	}
	b.Uint(len(c.Available))
	for _, s := range c.Available {
		b.String(s.ID)
		b.String(s.Pattern)
	}
	for _, item := range c.AppendItems {
		b.String(item)
	}
	for _, set := range [][Lengths]string{c.DateNumbers, c.TimeNumbers} {
		for _, s := range set {
			b.String(s)
		}
	}
	b.String(c.IntervalFallback)
	b.Uint(len(c.Intervals))
	for _, iv := range c.Intervals {
		b.String(iv.Skeleton)
		for _, p := range iv.Patterns {
			b.String(p)
		}
	}
	for _, p := range c.LeapMonthPatterns {
		b.String(p)
	}
	b.Uint(len(c.CyclicYears))
	for _, s := range c.CyclicYears {
		b.String(s)
	}
}

// Decode reads what Encode wrote.
func Decode(data []byte) (*Locale, error) {
	r, err := blob.NewReader(data, Version)
	if err != nil {
		return nil, err
	}
	var l Locale
	if n := r.Uint(); n >= 0 && n <= r.Left() {
		l.PeriodRules = make([]PeriodRule, 0, n)
		for i := 0; i < n; i++ {
			var rule PeriodRule
			rule.ID = r.String()
			rule.From = r.Uint()
			rule.Before = r.Uint()
			rule.At = r.Uint()
			rule.Point = r.Uint() == 1
			l.PeriodRules = append(l.PeriodRules, rule)
		}
	}
	for i := range l.FieldNames {
		l.FieldNames[i] = r.String()
	}
	l.DateTimeGlue = r.String()
	n := r.Uint()
	if n < 0 || n > r.Left() {
		n = 0
	}
	for i := 0; i < n; i++ {
		name := r.String()
		var c Calendar
		switch same := r.Uint(); {
		case same == 0:
			decodeCalendar(r, &c)
		case same <= len(l.Calendars):
			c = l.Calendars[same-1].Calendar
		default:
			return nil, fmt.Errorf("datedata: calendar %s is the same as calendar %d of %d", name, same, len(l.Calendars))
		}
		l.Calendars = append(l.Calendars, NamedCalendar{Name: name, Calendar: c})
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}

func decodeCalendar(r *blob.Reader, c *Calendar) {
	for _, sets := range []*[Contexts * Widths]Names{&c.Months, &c.Days} {
		for i := range sets {
			n := r.Uint()
			if n < 0 || n > r.Left() {
				return
			}
			text := make([]string, 0, n)
			for j := 0; j < n; j++ {
				text = append(text, r.String())
			}
			sets[i].Text = text
		}
	}
	for w := 0; w < Widths; w++ {
		c.AM[w] = r.String()
		c.PM[w] = r.String()
		n := r.Uint()
		if n < 0 || n > r.Left() {
			return
		}
		periods := make([]DayPeriod, 0, n)
		for i := 0; i < n; i++ {
			id := r.String()
			text := r.String()
			periods = append(periods, DayPeriod{ID: id, Text: text})
		}
		c.Periods[w] = periods
	}
	for i := range c.Eras {
		n := r.Uint()
		if n < 0 || n > r.Left() {
			return
		}
		text := make([]string, 0, n)
		for j := 0; j < n; j++ {
			text = append(text, r.String())
		}
		c.Eras[i].Text = text
	}
	for _, set := range []*[Lengths]string{&c.DateFormats, &c.TimeFormats,
		&c.DateTimeFormats, &c.AtTimeFormats} {
		for i := range set {
			set[i] = r.String()
		}
	}
	n := r.Uint()
	if n < 0 || n > r.Left() {
		return
	}
	c.Available = make([]Skeleton, 0, n)
	for i := 0; i < n; i++ {
		id := r.String()
		pattern := r.String()
		c.Available = append(c.Available, Skeleton{ID: id, Pattern: pattern})
	}
	for i := range c.AppendItems {
		c.AppendItems[i] = r.String()
	}
	for _, set := range []*[Lengths]string{&c.DateNumbers, &c.TimeNumbers} {
		for i := range set {
			set[i] = r.String()
		}
	}
	c.IntervalFallback = r.String()
	n = r.Uint()
	if n < 0 || n > r.Left() {
		return
	}
	c.Intervals = make([]Interval, n)
	for i := range c.Intervals {
		c.Intervals[i].Skeleton = r.String()
		for j := range c.Intervals[i].Patterns {
			c.Intervals[i].Patterns[j] = r.String()
		}
	}
	for i := range c.LeapMonthPatterns {
		c.LeapMonthPatterns[i] = r.String()
	}
	n = r.Uint()
	if n < 0 || n > r.Left() {
		return
	}
	if n > 0 {
		c.CyclicYears = make([]string, n)
		for i := range c.CyclicYears {
			c.CyclicYears[i] = r.String()
		}
	}
}
