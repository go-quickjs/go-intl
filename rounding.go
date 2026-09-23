package intl

import (
	"math"
	"strconv"
	"strings"
)

// How a number is cut down to the digits that will be written.
//
// ECMA-402 has two ways of saying how many digits to keep -- a count of
// decimals, or a count of significant digits -- and nine ways of deciding
// which way a discarded remainder pushes the last kept digit. This is both.
//
// The rounding is done on the digits rather than on the float. Scaling a float
// by a power of ten to round it introduces error of its own, and the error
// lands exactly on the ties that the mode is there to decide.

// RoundingMode is how a discarded remainder moves the last kept digit.
type RoundingMode int

const (
	// HalfExpand rounds a half away from zero, and is ECMA-402's default.
	HalfExpand RoundingMode = iota
	Ceil
	Floor
	Expand
	Trunc
	HalfCeil
	HalfFloor
	HalfTrunc
	HalfEven
)

// TrailingZeroDisplay is whether a whole number keeps the decimals it was
// asked for.
type TrailingZeroDisplay int

const (
	// TrailingZeroAuto keeps them, and is the default.
	TrailingZeroAuto TrailingZeroDisplay = iota
	// TrailingZeroStripIfInteger drops them when nothing is after the point,
	// so that 1.00 is written "1" but 1.50 is still "1.50".
	TrailingZeroStripIfInteger
)

// RoundingPriority decides which count wins when both a decimal count and a
// significant-digit count are asked for.
type RoundingPriority int

const (
	// PriorityAuto lets the significant digits decide, which is what ECMA-402
	// does when both are given without a priority.
	PriorityAuto RoundingPriority = iota
	// MorePrecision keeps whichever of the two holds more.
	MorePrecision
	// LessPrecision keeps whichever holds less.
	LessPrecision
)

// remainder says how the discarded part of a number compares with a half.
type remainder int

const (
	remainderZero remainder = iota
	remainderBelowHalf
	remainderHalf
	remainderAboveHalf
)

// compareHalf classifies the digits being thrown away.
func compareHalf(discarded string) remainder {
	if discarded == "" || strings.Trim(discarded, "0") == "" {
		return remainderZero
	}
	switch {
	case discarded[0] > '5':
		return remainderAboveHalf
	case discarded[0] < '5':
		return remainderBelowHalf
	}
	if strings.Trim(discarded[1:], "0") == "" {
		return remainderHalf
	}
	return remainderAboveHalf
}

// roundsUp decides whether the last kept digit moves up.
func (m RoundingMode) roundsUp(r remainder, negative, lastOdd bool) bool {
	if r == remainderZero {
		return false
	}
	switch m {
	case Ceil:
		return !negative
	case Floor:
		return negative
	case Expand:
		return true
	case Trunc:
		return false
	case HalfCeil:
		return r == remainderAboveHalf || (r == remainderHalf && !negative)
	case HalfFloor:
		return r == remainderAboveHalf || (r == remainderHalf && negative)
	case HalfTrunc:
		return r == remainderAboveHalf
	case HalfEven:
		return r == remainderAboveHalf || (r == remainderHalf && lastOdd)
	default: // HalfExpand
		return r == remainderAboveHalf || r == remainderHalf
	}
}

// roundAt cuts a number to a given number of decimals.
//
// Places may be negative, which rounds above the point: -3 rounds to the
// nearest thousand, which is what a rounding increment and the wider
// significant-digit counts need.
func roundAt(v float64, places int, negative bool, mode RoundingMode) (integer, fraction string) {
	integer, fraction = splitFloat(v)

	if places >= 0 {
		if len(fraction) <= places {
			return integer, fraction
		}
		discarded := fraction[places:]
		kept := fraction[:places]
		lastOdd := false
		switch {
		case places > 0:
			lastOdd = (kept[places-1]-'0')%2 == 1
		case len(integer) > 0:
			lastOdd = (integer[len(integer)-1]-'0')%2 == 1
		}
		if mode.roundsUp(compareHalf(discarded), negative, lastOdd) {
			return increment(integer, kept)
		}
		return integer, kept
	}

	// Rounding above the point: the digits below it are discarded as well.
	drop := -places
	if drop >= len(integer) {
		// The whole number is below the place being rounded to.
		discarded := integer + fraction
		if mode.roundsUp(compareHalf(shiftLeft(discarded, len(integer)-drop)), negative, false) {
			return "1" + strings.Repeat("0", drop), ""
		}
		return "0", ""
	}
	kept := integer[:len(integer)-drop]
	discarded := integer[len(integer)-drop:] + fraction
	lastOdd := (kept[len(kept)-1]-'0')%2 == 1
	if mode.roundsUp(compareHalf(discarded), negative, lastOdd) {
		kept, _ = increment(kept, "")
	}
	return kept + strings.Repeat("0", drop), ""
}

// shiftLeft pads a digit string so that comparing it with a half is done at
// the right place when the rounding point is above every digit.
func shiftLeft(digits string, by int) string {
	if by >= 0 {
		return digits
	}
	return strings.Repeat("0", -by) + digits
}

// splitFloat renders the magnitude of a number as its digits.
//
// The shortest form that reads back as the same float is used, which is what
// ICU works from; see the note in decimal.go.
func splitFloat(v float64) (integer, fraction string) {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	integer, fraction, _ = strings.Cut(s, ".")
	return integer, fraction
}

// roundSignificant cuts a number to a number of significant digits.
func roundSignificant(v float64, n int, negative bool, mode RoundingMode) (integer, fraction string) {
	if v == 0 || n <= 0 {
		return "0", ""
	}
	// The place to round at is n digits after the first significant one.
	e := int(math.Floor(math.Log10(v)))
	return roundAt(v, n-1-e, negative, mode)
}

// significantPlace and fractionPlace say where each way of counting would
// round, as a power of ten. The smaller place keeps more.
func significantPlace(v float64, maxSignificant int) int {
	if v == 0 {
		return 0
	}
	return int(math.Floor(math.Log10(v))) - maxSignificant + 1
}

// roundToIncrement rounds to a multiple of an increment at a given number of
// decimals: an increment of 5 at two decimals rounds to the nearest 0.05.
//
// ECMA-402 only allows an increment when the smallest and largest decimal
// counts are the same, so the digits are already at a fixed place and the
// whole thing is integer arithmetic on that place.
func roundToIncrement(integer, fraction string, places, increment int,
	negative bool, mode RoundingMode) (string, string) {
	if increment <= 1 {
		return integer, fraction
	}
	for len(fraction) < places {
		fraction += "0"
	}
	units, ok := parseDigits(integer + fraction[:places])
	if !ok {
		// Too large to count in units of the last place, in which case it is
		// far above any increment and rounding to one changes nothing.
		return integer, fraction
	}
	step := int64(increment)
	low := units - units%step
	r := remainderBelowHalf
	switch rest := units - low; {
	case rest == 0:
		r = remainderZero
	case 2*rest > step:
		r = remainderAboveHalf
	case 2*rest == step:
		r = remainderHalf
	}
	if mode.roundsUp(r, negative, (low/step)%2 == 1) {
		low += step
	}

	out := strconv.FormatInt(low, 10)
	if places == 0 {
		return out, ""
	}
	for len(out) <= places {
		out = "0" + out
	}
	return out[:len(out)-places], out[len(out)-places:]
}

// parseDigits reads a digit string as a number, refusing one too long to hold.
func parseDigits(s string) (int64, bool) {
	if s == "" {
		return 0, true
	}
	if len(s) > 18 {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
