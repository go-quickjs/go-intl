package intl

import "math"

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
func (f *NumberFormat) exponentFor(magnitude float64) int {
	if magnitude == 0 {
		return 0
	}
	e := int(math.Floor(math.Log10(magnitude)))
	if f.opts.Notation == NotationEngineering {
		// Down to the multiple of three at or below it, which is a floor and
		// not a truncation: -4 goes to -6, not to -3.
		e = int(math.Floor(float64(e)/3)) * 3
	}
	return e
}

// scientificParts writes a number against a power of ten.
func (f *NumberFormat) scientificParts(magnitude float64, negative bool) []Part {
	exponent := f.exponentFor(magnitude)
	mantissa := magnitude
	if magnitude != 0 {
		mantissa = magnitude / math.Pow(10, float64(exponent))
	}

	integer, fraction := f.round(mantissa, negative)
	// Rounding can carry the mantissa past what the notation allows: 9.9995
	// becomes 10.000, which in scientific notation is 1.000 one power higher.
	limit := 1
	if f.opts.Notation == NotationEngineering {
		limit = 3
	}
	if len(integer) > limit {
		exponent += len(integer) - limit
		mantissa = magnitude / math.Pow(10, float64(exponent))
		integer, fraction = f.round(mantissa, negative)
	}
	integer = padInteger(integer, f.minInt)

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
