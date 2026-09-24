package intl

import (
	"fmt"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datedata"
)

// Matching a request for fields to a pattern the locale has.
//
// A caller that names the fields it wants -- the year, a long month, the day
// -- is not asking for an order. Where the month goes relative to the day is
// the locale's business, and CLDR answers it with a list of skeletons: "yMMMd"
// resolves to "MMM d, y" in English and "d. MMM y" in German.
//
// A request rarely matches a skeleton exactly. UTS #35 says to find the
// closest, then bend its fields to what was asked for: a pattern found under
// "yMMMd" is used for "yMMMMd" with its month widened, because the order was
// what was wanted from it and the width was not.

// skeletonOrder is the order fields appear in a skeleton, which UTS #35 fixes
// so that two requests for the same fields spell the same skeleton.
const skeletonOrder = "GyMdEahmsz"

// requestedSkeleton builds the skeleton the options ask for.
func (f *DateTimeFormat) requestedSkeleton() string {
	o := &f.opts
	var b strings.Builder

	repeat := func(letter byte, n int) {
		for i := 0; i < n; i++ {
			b.WriteByte(letter)
		}
	}
	nameWidth := func(w FieldWidth, letter byte) {
		switch w {
		case WidthLong:
			repeat(letter, 4)
		case WidthShort:
			repeat(letter, 3)
		case WidthNarrow:
			repeat(letter, 5)
		}
	}

	switch o.Era {
	case WidthLong:
		repeat('G', 4)
	case WidthShort:
		repeat('G', 3)
	case WidthNarrow:
		repeat('G', 5)
	}
	switch o.Year {
	case WidthNumeric:
		repeat('y', 1)
	case Width2Digit:
		repeat('y', 2)
	}
	switch o.Month {
	case WidthNumeric:
		repeat('M', 1)
	case Width2Digit:
		repeat('M', 2)
	default:
		nameWidth(o.Month, 'M')
	}
	switch o.Day {
	case WidthNumeric:
		repeat('d', 1)
	case Width2Digit:
		repeat('d', 2)
	}
	nameWidth(o.Weekday, 'E')

	hour := f.hourLetter()
	if hour == 0 {
		hour = f.preferredHour()
	}
	switch o.Hour {
	case WidthNumeric:
		repeat(hour, 1)
	case Width2Digit:
		repeat(hour, 2)
	}
	switch o.Minute {
	case WidthNumeric:
		repeat('m', 1)
	case Width2Digit:
		repeat('m', 2)
	}
	switch o.Second {
	case WidthNumeric:
		repeat('s', 1)
	case Width2Digit:
		repeat('s', 2)
	}
	switch o.TimeZoneName {
	case WidthLong:
		repeat('z', 4)
	case WidthShort:
		repeat('z', 1)
	}
	return b.String()
}

// skeletonPattern finds the pattern for what the options asked for.
func (f *DateTimeFormat) skeletonPattern() (string, error) {
	// The constructor has supplied the default fields, so something is
	// always asked for.
	want := f.requestedSkeleton()

	if pattern, ok := f.calendar.Skeleton(want); ok {
		return plainDayPeriod(pattern), nil
	}

	// A request that mixes date fields with time fields rarely has a skeleton
	// of its own: no locale lists one for every combination. UTS #35 says to
	// answer each half separately and join them with the same glue a whole
	// date and a whole time take.
	if date, clock := splitSkeleton(want); date != "" && clock != "" {
		datePattern, err := f.onePattern(date)
		if err != nil {
			return "", err
		}
		timePattern, err := f.onePattern(clock)
		if err != nil {
			return "", err
		}
		glue := f.calendar.DateTimeFormats[glueLength(date)]
		if glue == "" {
			glue = "{1}, {0}"
		}
		joined := strings.ReplaceAll(glue, "{1}", datePattern)
		return strings.ReplaceAll(joined, "{0}", timePattern), nil
	}

	return f.onePattern(want)
}

// splitSkeleton divides a request into its date fields and its time fields.
func splitSkeleton(want string) (date, clock string) {
	var d, c strings.Builder
	for i := 0; i < len(want); i++ {
		switch want[i] {
		case 'G', 'y', 'Y', 'u', 'r', 'M', 'L', 'd', 'D', 'E', 'e', 'c', 'w', 'W', 'Q', 'q':
			d.WriteByte(want[i])
		default:
			c.WriteByte(want[i])
		}
	}
	return d.String(), c.String()
}

// glueLength picks which of the four glue patterns joins a date to a time.
//
// UTS #35 takes it from how the date is written: a date naming its weekday is
// the full one, a date spelling its month out is long, an abbreviated month is
// medium and everything else short.
func glueLength(date string) int {
	months := strings.Count(date, "M") + strings.Count(date, "L")
	switch {
	case strings.ContainsAny(date, "E"):
		return datedata.Full
	case months >= 4:
		return datedata.Long
	case months == 3:
		return datedata.Medium
	}
	return datedata.ShortLength
}

// onePattern answers a request that is all date or all time.
func (f *DateTimeFormat) onePattern(want string) (string, error) {
	if pattern, ok := f.calendar.Skeleton(want); ok {
		return plainDayPeriod(pattern), nil
	}
	best, found := f.closestSkeleton(want)
	if !found {
		return "", fmt.Errorf("intl: %s has no pattern for %q: %w",
			f.locale, want, ErrNotFound)
	}
	return plainDayPeriod(f.adjustWidths(best, want)), nil
}

// plainDayPeriod writes the two halves of the day where a pattern asks for the
// finer parts.
//
// It applies only where the caller named the fields it wanted. ECMA-402 has no
// option for the part of the day, so a request for an hour is answered with the
// half: Chinese writes 上午 at midnight for a named hour and 凌晨 for a whole
// time asked for by length, from the same "Bh:mm" the locale supplies.
func plainDayPeriod(pattern string) string {
	if !strings.ContainsRune(pattern, 'B') {
		return pattern
	}
	var b strings.Builder
	for _, fd := range parseDatePattern(pattern) {
		switch {
		case fd.letter == 0:
			b.WriteString(quoteLiteral(fd.literal))
		case fd.letter == 'B':
			b.WriteString(strings.Repeat("a", fd.count))
		default:
			b.WriteString(strings.Repeat(string(fd.letter), fd.count))
		}
	}
	return b.String()
}

// fieldCounts reduces a skeleton to how many times each letter appears.
func fieldCounts(skeleton string) map[byte]int {
	out := map[byte]int{}
	for i := 0; i < len(skeleton); i++ {
		c := skeleton[i]
		switch c {
		case 'L':
			c = 'M'
		case 'c':
			c = 'E'
		case 'k', 'K', 'H', 'h', 'j':
			c = 'h'
		case 'v', 'V', 'Z', 'O':
			c = 'z'
		case 'b', 'B':
			c = 'a'
		}
		out[c]++
	}
	return out
}

// closestSkeleton finds the available format nearest to what was asked for.
//
// Nearness is a distance rather than a score, and the ordering matters more
// than the numbers: a candidate missing a field the request wanted, or
// carrying one it did not, is far away, while one that merely writes a field
// at another width is close. That is what makes "yMMMd" answer a request for
// "yMMMMd" -- the order was what was wanted from it and the width was not --
// rather than "MMMM" answering it.
//
// Within a field, writing a name where a number was asked for is a bigger
// difference than writing two digits where one was asked for, since the first
// changes what the field says and the second only how wide it is.
func (f *DateTimeFormat) closestSkeleton(want string) (string, bool) {
	wanted := fieldCounts(want)
	wantHour := hourLetterOf(want)

	const (
		missing  = 4096
		extra    = 4096
		classGap = 64
	)
	bestDistance, bestPattern, bestID := -1, "", ""
	for _, s := range f.calendar.Available {
		// The hour letter is not a width: h counts to twelve and H to
		// twenty-four, and a skeleton that counts the other way is the wrong
		// one however well its other fields line up.
		if wantHour != 0 {
			if got := hourLetterOf(s.ID); got != 0 && got != wantHour {
				continue
			}
		}
		have := fieldCounts(s.ID)

		distance := 0
		for letter, n := range wanted {
			m, present := have[letter]
			if !present {
				distance += missing
				continue
			}
			if isTextWidth(letter, n) != isTextWidth(letter, m) {
				distance += classGap
			}
			if diff := n - m; diff > 0 {
				distance += diff
			} else {
				distance -= diff
			}
		}
		for letter := range have {
			if _, asked := wanted[letter]; !asked {
				distance += extra
			}
		}
		if bestDistance < 0 || distance < bestDistance ||
			(distance == bestDistance && s.ID < bestID) {
			bestDistance, bestPattern, bestID = distance, s.Pattern, s.ID
		}
	}
	return bestPattern, bestPattern != ""
}

// isTextWidth reports whether a field written this many times is a name rather
// than a number. Only the fields that can be either are asked.
func isTextWidth(letter byte, count int) bool {
	switch letter {
	case 'M':
		return count >= 3
	case 'E':
		return true
	case 'G', 'a':
		return true
	}
	return false
}

// hourLetterOf is the way a skeleton counts hours, or zero if it has none.
func hourLetterOf(skeleton string) byte {
	for i := 0; i < len(skeleton); i++ {
		switch c := skeleton[i]; c {
		case 'h', 'K':
			return 'h'
		case 'H', 'k':
			return 'H'
		}
	}
	return 0
}

// adjustWidths bends a pattern's fields to the widths that were asked for,
// keeping the order and the literals the locale chose.
func (f *DateTimeFormat) adjustWidths(pattern, want string) string {
	wanted := map[byte]int{}
	for i := 0; i < len(want); i++ {
		c := want[i]
		switch c {
		case 'j':
			continue
		case 'L':
			c = 'M'
		case 'c':
			c = 'E'
		case 'K', 'H', 'h', 'k':
			c = 'h'
		case 'v', 'V', 'Z', 'O':
			c = 'z'
		case 'b', 'B':
			c = 'a'
		}
		wanted[c]++
	}

	var b strings.Builder
	for _, fd := range parseDatePattern(pattern) {
		if fd.letter == 0 {
			b.WriteString(quoteLiteral(fd.literal))
			continue
		}
		key := fd.letter
		switch key {
		case 'L':
			key = 'M'
		case 'c':
			key = 'E'
		case 'K', 'H', 'h', 'k':
			key = 'h'
		case 'v', 'V', 'Z', 'O':
			key = 'z'
		case 'b', 'B':
			key = 'a'
		}
		count := fd.count
		if n, ok := wanted[key]; ok && n > 0 {
			// A width is adjusted only within its own class. Turning a number
			// into a name is not a widening: Japanese writes the month as
			// "M月", where the 月 belongs to the pattern, and widening the M to
			// a name would write the character twice.
			if isTextWidth(key, n) == isTextWidth(key, fd.count) {
				count = n
			}
		}
		if key == 'h' && count < 2 && f.padsHour() {
			count = 2
		}
		b.WriteString(strings.Repeat(string(fd.letter), count))
	}
	return b.String()
}

// preferredHour is the way this locale counts hours when nobody said.
//
// It is read from the locale's own medium time pattern rather than from a
// table of preferences: that pattern is what the locale actually writes, so it
// cannot disagree with itself. A locale that writes "h:mm:ss a" counts to
// twelve and one that writes "HH:mm:ss" counts to twenty-four.
func (f *DateTimeFormat) preferredHour() byte {
	for _, length := range []int{datedata.Medium, datedata.Full} {
		for _, fd := range parseDatePattern(f.calendar.TimeFormats[length]) {
			switch fd.letter {
			case 'h', 'H', 'K', 'k':
				return fd.letter
			}
		}
	}
	return 'H'
}

// padsHour reports whether the hour is written to two digits although only a
// plain number was asked for.
//
// It happens in one place: a locale that counts to twelve, asked to count to
// twenty-four instead. English writes "9:30 AM" but "09:30", and German, which
// counts to twenty-four already, writes "9:30" either way. The rule is ICU's
// and the corpus is what establishes it.
func (f *DateTimeFormat) padsHour() bool {
	forced := f.hourLetter()
	if forced != 'H' && forced != 'k' {
		return false
	}
	preferred := f.preferredHour()
	return preferred == 'h' || preferred == 'K'
}
