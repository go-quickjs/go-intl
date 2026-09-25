package intl

import (
	"math"
	"strings"
)

// Scientific and engineering notation: 1234.5 as "1.235E3", or "1.2345E3" as
// engineering would write 123456789012, "123.457E9".
//
// The two differ only in which exponents are allowed. Scientific puts exactly
// one digit before the point, so the exponent is whatever it has to be.
// Engineering restricts the exponent to multiples of three, which is how
// magnitudes line up with the words for them -- thousand, million, billion --
// so the mantissa has one, two or three digits before the point instead.

// exponentFor returns the power of ten a number is written against, and the
// mantissa that goes with it.
func (f *NumberFormat) exponentFor(magnitude mag) int {
	if magnitude.isZero() {
		return 0
	}
	e := magnitude.exponent()
	if f.opts.Notation == NotationEngineering {
		// Down to the multiple of three at or below it, which is a floor and
		// not a truncation: -4 goes to -6, not to -3.
		e = int(math.Floor(float64(e)/3)) * 3
	}
	return e
}

// scientificParts writes a number against a power of ten.
func (f *NumberFormat) scientificParts(magnitude mag, negative bool) []Part {
	exponent, integer, fraction := f.scientificDigits(magnitude, negative)

	parts := f.groupedInteger(integer)
	if fraction != "" {
		parts = append(parts, Part{PartDecimal, f.decimalSep})
		parts = append(parts, Part{PartFraction, f.digits(fraction)})
	}

	parts = append(parts, Part{PartExponentSeparator, f.data.Symbols.Exponential})
	if exponent < 0 {
		parts = append(parts, Part{PartExponentMinusSign, f.data.Symbols.MinusSign})
		exponent = -exponent
	}
	parts = append(parts, Part{PartExponentInteger, f.digits(itoa(exponent))})
	return parts
}

// scientificDigits is the exponent a number is written against and the
// rounded digits of its mantissa.
func (f *NumberFormat) scientificDigits(magnitude mag, negative bool) (int, string, string) {
	exponent := f.exponentFor(magnitude)
	mantissa := magnitude.shift(-exponent)

	integer, fraction := f.round(mantissa, negative)
	// Rounding can carry the mantissa past what the notation allows: 9.9995
	// becomes 10.000, which in scientific notation is 1.000 one power higher.
	limit := 1
	if f.opts.Notation == NotationEngineering {
		limit = 3
	}
	if len(integer) > limit {
		exponent += len(integer) - limit
		mantissa = magnitude.shift(-exponent)
		integer, fraction = f.round(mantissa, negative)
	}
	return exponent, padInteger(integer, f.minInt), fraction
}

// shiftDigits moves the decimal point of written digits by a power of ten:
// ICU's plural operands for "1.5M" are those of 1500000, with the exponent
// beside them, so that "1.5M" has no visible fraction.
func shiftDigits(integer, fraction string, exponent int) (string, string) {
	digits := integer + fraction
	point := len(integer) + exponent
	switch {
	case point <= 0:
		integer, fraction = "0", strings.Repeat("0", -point)+digits
	case point >= len(digits):
		integer, fraction = digits+strings.Repeat("0", point-len(digits)), ""
	default:
		integer, fraction = digits[:point], digits[point:]
	}
	if trimmed := strings.TrimLeft(integer, "0"); trimmed != "" {
		integer = trimmed
	} else {
		integer = "0"
	}
	return integer, fraction
}

// pluralOperands are what plural rules see of a number as this formatter
// writes it: the digits after rounding, and in compact or scientific
// notation the power of ten written apart, which French and others count.
func (f *NumberFormat) pluralOperands(v float64) operands {
	negative := math.Signbit(v)
	magnitude := magOf(v)
	switch f.opts.Notation {
	case NotationCompact:
		form := f.compactForm(magnitude, negative)
		integer, fraction := f.round(magnitude.shift(-form.exponent), negative)
		integer, fraction = shiftDigits(integer, fraction, form.exponent)
		return operandsFor(integer, fraction, form.exponent)
	case NotationScientific, NotationEngineering:
		exponent, integer, fraction := f.scientificDigits(magnitude, negative)
		integer, fraction = shiftDigits(integer, fraction, exponent)
		return operandsFor(integer, fraction, exponent)
	}
	integer, fraction := f.round(magnitude, negative)
	return operandsFor(padInteger(integer, f.minInt), fraction, 0)
}

// itoa writes a non-negative number without pulling in strconv's formatting
// rules, since an exponent is small and always ASCII here.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 && i > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
