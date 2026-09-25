package intl

import (
	"strings"
	"time"
)

// The calendars a date may be reckoned in.
//
// A calendar is two things: an arithmetic, which says what year and month an
// instant falls in, and a set of names and patterns, which the locale supplies
// separately for each one. The names live in the date data; the arithmetic is
// here.
//
// Two are implemented. The Gregorian one needs no arithmetic of its own, since
// that is what Go's time package already reckons in. The Buddhist one counts
// the same months and days from a different year, 543 earlier, and has one era
// rather than two.

// A CalendarSystem is a way of reckoning dates.
type CalendarSystem string

const (
	// Gregory is the Gregorian calendar, and the default nearly everywhere.
	Gregory CalendarSystem = "gregory"
	// Buddhist counts its years from 543 BC, and is what Thailand reckons in.
	Buddhist CalendarSystem = "buddhist"
	// Persian is the solar hijri calendar of Iran and Afghanistan.
	Persian CalendarSystem = "persian"
)

// implemented lists the calendars this can reckon in. The rest of CLDR's are
// refused when a formatter is built rather than answered in the wrong one.
var implemented = map[CalendarSystem]bool{
	Gregory:  true,
	Buddhist: true,
	Persian:  true,
}

// calendarPreferences maps a region to the calendar it reckons in, as lines of
// "region calendar". It is one table for every locale, since which calendar a
// place uses is not a matter of language.
type calendarPreferences string

func (p calendarPreferences) of(region string) (CalendarSystem, bool) {
	if region == "" {
		return "", false
	}
	for line := range strings.SplitSeq(strings.TrimRight(string(p), "\n"), "\n") {
		name, calendar, ok := strings.Cut(line, " ")
		if ok && name == region {
			return CalendarSystem(calendar), true
		}
	}
	return "", false
}

// chooseCalendar settles which calendar a formatter reckons in.
//
// The option wins, then the locale's own "-u-ca-" extension, then the calendar
// the locale's region uses, and failing all of that the Gregorian one. A
// locale with no region of its own is given the one its likely subtags supply,
// so that "th" reckons as "th-TH" does.
func chooseCalendar(src Source, loc Locale, asked string) (CalendarSystem, error) {
	if asked == "" {
		if value, ok := loc.Keyword("ca"); ok {
			asked = value
		}
	}
	if asked != "" {
		system := CalendarSystem(asked)
		if !implemented[system] {
			return "", &unimplementedCalendarError{asked}
		}
		return system, nil
	}

	region := loc.Region.String()
	if region == "" {
		if f, err := NewFallbacker(src); err == nil {
			if full, ok := f.Maximize(loc.Data()); ok {
				region = full.Region.String()
			}
		}
	}
	if b, err := src.Open(MarkerCalendarPrefs, DataLocale{}); err == nil {
		if system, ok := calendarPreferences(b).of(region); ok && implemented[system] {
			return system, nil
		}
	}
	return Gregory, nil
}

// gregorianYearLength is the proleptic Gregorian year's length, as V8 sets
// ICU's Gregorian calendar to reckon with no Julian changeover.
func gregorianYearLength(year int) int {
	if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		return 366
	}
	return 365
}

// unimplementedCalendarError names a calendar CLDR has and this does not.
type unimplementedCalendarError struct{ name string }

func (e *unimplementedCalendarError) Error() string {
	return "intl: the " + e.name + " calendar is not implemented yet"
}

// reckon turns an instant into the fields of a date in one calendar.
//
// Only the year and the era differ between the two implemented calendars: the
// Buddhist one keeps the Gregorian months and days and counts the years from
// 543 years earlier, so 2024 is 2567 and every date is in its single era.
func reckon(t time.Time, system CalendarSystem) dateParts {
	p := partsOf(t)
	// The Gregorian calendar's own fields, which the Buddhist one keeps:
	// its extended year is the Gregorian one, counting on through 0 for
	// 1 BC.
	p.extYear, p.dayOfYear, p.relatedYear = t.Year(), t.YearDay(), t.Year()
	p.yearLength = gregorianYearLength
	switch system {
	case Persian:
		p.year, p.month, p.day = persianDate(julianDay(t))
		p.era = 0
		p.extYear = p.year
		p.dayOfYear = persianMonthStart[p.month-1] + p.day
		p.relatedYear = p.year + 622
		p.yearLength = persianYearLength
		return p
	}
	if system == Buddhist {
		// partsOf has already turned a year before the epoch into a positive
		// count in the earlier era, which has to be undone before the offset.
		year := p.year
		if p.era == 0 {
			year = 1 - year
		}
		p.year = year + 543
		p.era = 0
	}
	return p
}
