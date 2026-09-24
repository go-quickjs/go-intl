package intl

import (
	"strings"
	"time"

	"github.com/go-quickjs/go-intl/internal/datedata"
)

// UTS #35 date patterns.
//
// A pattern is a run of field letters and literal text: "EEEE, MMMM d, y" is
// the weekday wide, a comma, the month wide, the day, a comma and the year.
// How many times a letter repeats says how wide the field is written, and the
// meaning of that differs by letter: MMM is an abbreviated month name while dd
// is a day padded to two digits.
//
// Text outside the fields is quoted with apostrophes, so a pattern can write a
// letter that would otherwise be a field.

// a dateField is one run of the same pattern letter.
type dateField struct {
	// letter is the field, and count how many times it repeated. A literal
	// run has a zero letter and its text in literal.
	letter  byte
	count   int
	literal string
}

// parseDatePattern takes a pattern apart into its fields and literals.
func parseDatePattern(pattern string) []dateField {
	var out []dateField
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			out = append(out, dateField{literal: literal.String()})
			literal.Reset()
		}
	}
	for i := 0; i < len(pattern); {
		c := pattern[i]
		switch {
		case c == '\'':
			// An apostrophe quotes what follows, and two of them are one
			// apostrophe.
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				literal.WriteByte('\'')
				i += 2
				continue
			}
			i++
			for i < len(pattern) && pattern[i] != '\'' {
				literal.WriteByte(pattern[i])
				i++
			}
			i++
		case isPatternLetter(c):
			flush()
			n := 0
			for i < len(pattern) && pattern[i] == c {
				n++
				i++
			}
			out = append(out, dateField{letter: c, count: n})
		default:
			literal.WriteByte(c)
			i++
		}
	}
	flush()
	return out
}

func isPatternLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// a dateParts is what a formatter needs to write one instant: the fields of
// the date in its calendar and zone, and the zone's own names.
type dateParts struct {
	year, month, day int
	weekday          int // Sunday is zero
	hour, minute     int
	second, millis   int
	era              int // zero before the epoch, one after
	// zone is what the time zone is called and how far it is from UTC.
	zoneShort, zoneLong string
	zoneOffset          int
	// hour12 and dayPeriod are worked out once rather than per field.
	hour12    int
	afternoon bool
}

func partsOf(t time.Time) dateParts {
	year, month, day := t.Date()
	era := 1
	if year <= 0 {
		// The year before 1 is 1 BC, and there is no year zero.
		era, year = 0, 1-year
	}
	p := dateParts{
		year: year, month: int(month), day: day,
		weekday: int(t.Weekday()),
		hour:    t.Hour(), minute: t.Minute(),
		second: t.Second(), millis: t.Nanosecond() / 1e6,
		era: era,
	}
	p.afternoon = p.hour >= 12
	p.hour12 = p.hour % 12
	if p.hour12 == 0 {
		p.hour12 = 12
	}
	return p
}

// pad writes a number to at least a given width.
func pad(v, width int) string {
	s := itoa(v)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// datePartKind maps a pattern letter to the piece it produces, so that
// formatToParts can name what it wrote.
func datePartKind(letter byte) PartKind {
	switch letter {
	case 'G':
		return PartEra
	case 'y', 'Y', 'u', 'U', 'r':
		return PartYear
	case 'M', 'L':
		return PartMonth
	case 'd':
		return PartDay
	case 'E', 'e', 'c':
		return PartWeekday
	case 'a', 'b', 'B':
		return PartDayPeriod
	case 'h', 'H', 'K', 'k':
		return PartHour
	case 'm':
		return PartMinute
	case 's':
		return PartSecond
	case 'S':
		return PartFractionalSecond
	case 'z', 'Z', 'O', 'v', 'V', 'X', 'x':
		return PartTimeZoneName
	}
	return PartLiteral
}

// writeField writes one field of a pattern.
//
// The width rules differ by letter and are UTS #35's: a month written once or
// twice is a number, three times an abbreviation, four times the whole word
// and five times a single letter. A year is padded to the count except that
// two of them means the last two digits.
func (f *DateTimeFormat) writeField(fd dateField, p *dateParts) string {
	cal := f.calendar
	switch fd.letter {
	case 'G':
		width := datedata.Abbreviated
		switch {
		case fd.count >= 5:
			width = datedata.Narrow
		case fd.count == 4:
			width = datedata.Wide
		}
		return cal.Era(width, p.era)

	case 'y', 'Y', 'u', 'r':
		if fd.count == 2 {
			return f.digits(pad(p.year%100, 2))
		}
		return f.digits(pad(p.year, fd.count))

	case 'M', 'L':
		context := datedata.Format
		if fd.letter == 'L' {
			context = datedata.StandAlone
		}
		switch {
		case fd.count <= 2:
			return f.digits(pad(p.month, fd.count))
		case fd.count == 3:
			return cal.Month(context, datedata.Abbreviated, p.month)
		case fd.count == 4:
			return cal.Month(context, datedata.Wide, p.month)
		default:
			return cal.Month(context, datedata.Narrow, p.month)
		}

	case 'd':
		return f.digits(pad(p.day, fd.count))

	case 'E', 'e', 'c':
		context := datedata.Format
		if fd.letter == 'c' {
			context = datedata.StandAlone
		}
		// e and c written once or twice are the day's number in the week,
		// which nothing here asks for; every other count is a name.
		width := datedata.Abbreviated
		switch {
		case fd.count == 4:
			width = datedata.Wide
		case fd.count == 5:
			width = datedata.Narrow
		case fd.count >= 6:
			width = datedata.Short
		}
		return cal.Day(context, width, p.weekday)

	case 'a', 'b', 'B':
		width := datedata.Abbreviated
		switch {
		case fd.count == 4:
			width = datedata.Wide
		case fd.count >= 5:
			width = datedata.Narrow
		}
		// B asks for the part of the day rather than which half it is: the
		// small hours are 凌晨 in Chinese and neither morning nor afternoon.
		// A language with no rule for the hour, or no word for the part it
		// falls in, is written with the half instead.
		if fd.letter == 'B' {
			if id := f.data.Period(p.hour*60 + p.minute); id != "" {
				if name := cal.PeriodName(width, id); name != "" {
					return name
				}
			}
		}
		if p.afternoon {
			if s := cal.PM[width]; s != "" {
				return s
			}
			return cal.PM[datedata.Abbreviated]
		}
		if s := cal.AM[width]; s != "" {
			return s
		}
		return cal.AM[datedata.Abbreviated]

	case 'h':
		return f.digits(pad(p.hour12, fd.count))
	case 'H':
		return f.digits(pad(p.hour, fd.count))
	case 'K':
		return f.digits(pad(p.hour%12, fd.count))
	case 'k':
		hour := p.hour
		if hour == 0 {
			hour = 24
		}
		return f.digits(pad(hour, fd.count))

	case 'm':
		return f.digits(pad(p.minute, fd.count))
	case 's':
		return f.digits(pad(p.second, fd.count))
	case 'S':
		// The fraction is truncated or padded to the count asked for.
		s := pad(p.millis, 3)
		for len(s) < fd.count {
			s += "0"
		}
		return f.digits(s[:fd.count])

	case 'z', 'v':
		long := fd.count >= 4
		if long {
			if p.zoneLong != "" {
				return p.zoneLong
			}
		} else if p.zoneShort != "" {
			return p.zoneShort
		}
		return f.digits(f.offsetText(p.zoneOffset, !long))
	case 'O':
		return f.digits(f.offsetText(p.zoneOffset, fd.count < 4))
	case 'Z', 'X', 'x', 'V':
		return f.digits(f.offsetText(p.zoneOffset, false))
	}
	return ""
}

// offsetText writes a zone as its distance from UTC, or as the locale's own
// word for no distance at all: English writes "GMT" rather than "GMT+0".
func (f *DateTimeFormat) offsetText(seconds int, short bool) string {
	if seconds == 0 {
		return f.gmtZero
	}
	return gmtOffset(seconds, f.gmtPattern, f.gmtHourFormat, short)
}

// gmtOffset writes a zone as its distance from UTC, by the locale's pattern:
// "GMT{0}" around "+5:30".
//
// The short form drops what it can: the hour is not padded and the minutes are
// left off when they are zero, so Nairobi is "GMT+3" and Kolkata "GMT+5:30".
// The long form always writes both, "GMT+03:00".
func gmtOffset(seconds int, pattern, hourFormat string, short bool) string {
	positive, negative, _ := strings.Cut(hourFormat, ";")
	form := positive
	if seconds < 0 {
		if negative != "" {
			form = negative
		}
		seconds = -seconds
	}
	if form == "" {
		form = "+HH:mm"
		if seconds < 0 {
			form = "-HH:mm"
		}
	}
	hours, minutes := seconds/3600, (seconds%3600)/60

	var b strings.Builder
	fields := parseDatePattern(form)
	for i, fd := range fields {
		switch fd.letter {
		case 'H':
			width := fd.count
			if short {
				width = 1
			}
			b.WriteString(pad(hours, width))
		case 'm':
			if short && minutes == 0 {
				continue
			}
			b.WriteString(pad(minutes, fd.count))
		default:
			// The separator before minutes that are not written goes with
			// them, so that "GMT+3" does not trail a colon.
			if short && minutes == 0 && i+1 < len(fields) && fields[i+1].letter == 'm' {
				continue
			}
			b.WriteString(fd.literal)
		}
	}
	if pattern == "" {
		pattern = "GMT{0}"
	}
	return strings.ReplaceAll(pattern, "{0}", b.String())
}
