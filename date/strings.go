package date

import (
	"fmt"
	"math"
)

// The strings Date writes, as V8's ToDateString writes them.

var (
	shortWeekdays = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	shortMonths   = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

// yearText is V8's "%04d", or "%05d" for a year before 1: a sign and at
// least four digits.
func yearText(year int64) string {
	if year < 0 {
		return fmt.Sprintf("%05d", year)
	}
	return fmt.Sprintf("%04d", year)
}

// local is a time value's local fields, its offset from UTC in minutes,
// and the zone's name at it.
func (e *Environment) local(t float64) (Fields, int64, string) {
	ms := int64(t)
	local := e.toLocal(ms)
	offset := -e.timezoneOffset(ms)
	return BreakDown(float64(local)), offset, e.zoneName(ms)
}

func offsetText(minutes int64) string {
	sign := byte('+')
	if minutes < 0 {
		sign = '-'
		minutes = -minutes
	}
	return fmt.Sprintf("GMT%c%02d%02d", sign, minutes/60, minutes%60)
}

// String is Date.prototype.toString: "Fri Sep 25 2026 08:00:00 GMT-0400
// (Eastern Daylight Time)", or "Invalid Date".
func (e *Environment) String(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f, offset, name := e.local(t)
	return fmt.Sprintf("%s %s %02d %s %02d:%02d:%02d %s (%s)", shortWeekdays[f.Weekday], shortMonths[f.Month],
		f.Day, yearText(f.Year), f.Hour, f.Minute, f.Second, offsetText(offset), name)
}

// DateString is Date.prototype.toDateString: "Fri Sep 25 2026".
func (e *Environment) DateString(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f, _, _ := e.local(t)
	return fmt.Sprintf("%s %s %02d %s", shortWeekdays[f.Weekday], shortMonths[f.Month], f.Day, yearText(f.Year))
}

// TimeString is Date.prototype.toTimeString: "08:00:00 GMT-0400 (Eastern
// Daylight Time)".
func (e *Environment) TimeString(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f, offset, name := e.local(t)
	return fmt.Sprintf("%02d:%02d:%02d %s (%s)", f.Hour, f.Minute, f.Second, offsetText(offset), name)
}

// UTCString is Date.prototype.toUTCString: "Fri, 25 Sep 2026 12:00:00 GMT".
func UTCString(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f := BreakDown(t)
	return fmt.Sprintf("%s, %02d %s %s %02d:%02d:%02d GMT", shortWeekdays[f.Weekday], f.Day, shortMonths[f.Month],
		yearText(f.Year), f.Hour, f.Minute, f.Second)
}

// ISOString is Date.prototype.toISOString: "2026-09-25T12:00:00.000Z",
// with six digits and a sign for a year outside 0 to 9999. It is false for
// NaN, where JavaScript throws a RangeError.
func ISOString(t float64) (string, bool) {
	if math.IsNaN(t) {
		return "", false
	}
	f := BreakDown(t)
	var year string
	switch {
	case f.Year >= 0 && f.Year <= 9999:
		year = fmt.Sprintf("%04d", f.Year)
	case f.Year < 0:
		year = fmt.Sprintf("-%06d", -f.Year)
	default:
		year = fmt.Sprintf("+%06d", f.Year)
	}
	return fmt.Sprintf("%s-%02d-%02dT%02d:%02d:%02d.%03dZ", year, f.Month+1, f.Day, f.Hour, f.Minute, f.Second,
		f.Millisecond), true
}
