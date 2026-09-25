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
	// Coptic, Ethiopic and EthiopicAmeteAlem count thirteen months, the
	// last of five or six days.
	Coptic            CalendarSystem = "coptic"
	Ethiopic          CalendarSystem = "ethiopic"
	EthiopicAmeteAlem CalendarSystem = "ethioaa"
	// Indian is the Indian national calendar, the Saka era.
	Indian CalendarSystem = "indian"
	// IslamicCivil and IslamicTabular are the arithmetic Islamic calendars,
	// counted from one day apart.
	IslamicCivil   CalendarSystem = "islamic-civil"
	IslamicTabular CalendarSystem = "islamic-tbla"
	// ROC counts the Gregorian years from 1912, the Republic of China.
	ROC CalendarSystem = "roc"
)

// implemented lists the calendars this can reckon in. The rest of CLDR's are
// refused when a formatter is built rather than answered in the wrong one.
var implemented = map[CalendarSystem]bool{
	Gregory:           true,
	Buddhist:          true,
	Persian:           true,
	Coptic:            true,
	Ethiopic:          true,
	EthiopicAmeteAlem: true,
	Indian:            true,
	IslamicCivil:      true,
	IslamicTabular:    true,
	ROC:               true,
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
	case Buddhist, ROC:
		// V8 makes ICU's Gregorian calendar proleptic, but only a calendar
		// that is exactly ICU's GregorianCalendar; the Buddhist and ROC
		// calendars are subclasses, and keep its Julian dates before the
		// changeover of October 1582.
		if jd := julianDay(t); jd < gregorianCutover {
			p.extYear, p.month, p.day, p.dayOfYear = julianDate(jd)
			p.relatedYear = p.extYear
		}
		p.yearLength = changeoverYearLength
	}
	switch system {
	case Persian:
		p.year, p.month, p.day = persianDate(julianDay(t))
		p.era = 0
		p.extYear = p.year
		p.dayOfYear = persianMonthStart[p.month-1] + p.day
		p.relatedYear = p.year + 622
		p.yearLength = persianYearLength
		return p
	case Coptic, Ethiopic, EthiopicAmeteAlem:
		// CECalendar::handleComputeFields, with each calendar's eras.
		epoch, related := copticEpoch, 284
		switch system {
		case Ethiopic:
			epoch, related = ethiopicEpoch, 8
		case EthiopicAmeteAlem:
			epoch, related = ameteAlemEpoch, 8
		}
		eyear, month, day, doy := ceDate(julianDay(t), epoch)
		p.extYear, p.month, p.day, p.dayOfYear = eyear, month, day, doy
		p.relatedYear = eyear + related
		p.yearLength = ceYearLength
		switch {
		case system == EthiopicAmeteAlem:
			p.year, p.era = eyear, 0
		case system == Ethiopic && eyear <= 0:
			p.year, p.era = eyear+ameteMihretDiff, 0
		case system == Coptic && eyear <= 0:
			p.year, p.era = 1-eyear, 0
		default:
			p.year, p.era = eyear, 1
		}
		return p
	case IslamicCivil, IslamicTabular:
		epoch := islamicCivilEpoch
		if system == IslamicTabular {
			epoch = islamicAstroEpoch
		}
		p.year, p.month, p.day, p.dayOfYear = islamicTabularDate(julianDay(t), epoch)
		p.era, p.extYear = 0, p.year
		p.relatedYear = islamicRelatedYear(p.year)
		p.yearLength = islamicTabularYearLength
		return p
	case Indian:
		p.year, p.month, p.day, p.dayOfYear = indianDate(t)
		p.era, p.extYear = 0, p.year
		p.relatedYear = p.year + 79
		p.yearLength = indianYearLength
		return p
	case ROC:
		// TaiwanCalendar::handleComputeFields: the extended year stays, and
		// the displayed year counts from 1912 or back from it.
		if y := p.extYear - 1911; y > 0 {
			p.year, p.era = y, 1
		} else {
			p.year, p.era = 1-y, 0
		}
		return p
	}
	if system == Buddhist {
		// BuddhistCalendar::handleComputeFields: the extended year stays.
		p.year = p.extYear + 543
		p.era = 0
	}
	return p
}

// gregorianCutover is the Julian day ICU's Gregorian calendar changes over
// from the Julian one on, 15 October 1582.
const gregorianCutover = 2299161

// julianDate is the Julian-calendar half of GregorianCalendar's
// handleComputeFields: the extended year, month from one, day and day of the
// year.
func julianDate(jd int) (year, month, day, dayOfYear int) {
	epochDay := jd - (1721426 - 2)
	year = floorDiv(4*epochDay+1464, 1461)
	jan1 := 365*(year-1) + floorDiv(year-1, 4)
	doy := epochDay - jan1
	leap := mod(year, 4) == 0
	correction, march1 := 0, 59
	if leap {
		march1 = 60
	}
	if doy >= march1 {
		correction = 2
		if leap {
			correction = 1
		}
	}
	m := (12*(doy+correction) + 6) / 367
	before := [...]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	day = doy - before[m] + 1
	if leap && m > 1 {
		day--
	}
	return year, m + 1, day, doy + 1
}

// changeoverYearLength is GregorianCalendar's year length with the
// changeover: Julian leap years before 1582.
func changeoverYearLength(year int) int {
	if year < 1582 {
		if mod(year, 4) == 0 {
			return 366
		}
		return 365
	}
	return gregorianYearLength(year)
}
