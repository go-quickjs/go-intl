package intl

import (
	"math"
	"strconv"
	"strings"
)

// A mag is the magnitude of a number as its decimal digits: the integer part
// with no leading zeros ("0" for none) and the fraction with no trailing
// ones. Everything a number formatter does to a number -- rounding it,
// dividing it into a compact or scientific form, making it a percent -- is
// done on the digits, so that a number given exactly, as a string or a
// BigInt, is written exactly, however many digits it has.
type mag struct {
	integer, fraction string
}

// magOf is the digits of a float's magnitude: the shortest that read back as
// the same float, which is what ICU works from; see decimal.go.
func magOf(v float64) mag {
	if v < 0 {
		v = -v
	}
	integer, fraction := splitFloat(v)
	return makeMag(integer, fraction)
}

// makeMag normalizes digits into a mag.
func makeMag(integer, fraction string) mag {
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	return mag{integer: integer, fraction: strings.TrimRight(fraction, "0")}
}

// isZero reports whether the magnitude is zero. The zero value of a mag, as
// in the zero value of a Decimal, is zero too.
func (m mag) isZero() bool { return (m.integer == "0" || m.integer == "") && m.fraction == "" }

// exponent is the power of ten of the first significant digit: the floor of
// the base-ten logarithm. Zero has none, and answers zero.
func (m mag) exponent() int {
	if m.integer != "0" {
		return len(m.integer) - 1
	}
	lead := len(m.fraction) - len(strings.TrimLeft(m.fraction, "0"))
	if lead == len(m.fraction) {
		return 0
	}
	return -lead - 1
}

// shift multiplies the magnitude by a power of ten.
func (m mag) shift(by int) mag {
	if by == 0 || m.isZero() {
		return m
	}
	integer, fraction := shiftDigits(m.integer, m.fraction, by)
	return makeMag(integer, fraction)
}

// float is the magnitude as a float, for the few decisions that only need
// its size.
func (m mag) float() float64 {
	s := m.integer
	if m.fraction != "" {
		s += "." + m.fraction
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// decimalKind says whether a Decimal is a number, NaN or an infinity.
type decimalKind uint8

const (
	decimalFinite decimalKind = iota
	decimalNaN
	decimalInfinite
)

// A Decimal is a number held exactly, as ECMA-402's formatters take one: a
// float, or a string or BigInt carrying more digits than a float can. Its
// zero value is zero.
type Decimal struct {
	kind decimalKind
	neg  bool
	m    mag
}

// DecimalFromFloat is a float as a Decimal: the shortest decimal that reads
// back as the float, which is the one ICU formats.
func DecimalFromFloat(v float64) Decimal {
	switch {
	case math.IsNaN(v):
		return Decimal{kind: decimalNaN}
	case math.IsInf(v, 0):
		return Decimal{kind: decimalInfinite, neg: v < 0}
	}
	neg := math.Signbit(v)
	return Decimal{neg: neg, m: magOf(v)}
}

// ParseDecimal reads a string as ECMA-402's ToIntlMathematicalValue does: as
// a JavaScript numeric string -- surrounding whitespace, a sign, digits, a
// point, an exponent; or "Infinity"; or hexadecimal, octal or binary digits
// without a sign -- but exactly, rather than rounded to a float. An empty
// string is zero, and anything else that is not a number is NaN. "-0" is
// negative zero. A number too large for a float is an infinity, as ECMA-402
// has it; one too small for a float is kept exactly, as V8 keeps it, where
// ECMA-402 would make it zero.
func ParseDecimal(s string) Decimal {
	s = strings.TrimFunc(s, isJSWhitespace)
	if s == "" {
		return Decimal{m: zeroMag}
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			integer, ok := baseToDecimal(s[2:], base)
			if !ok {
				return Decimal{kind: decimalNaN}
			}
			if _, err := strconv.ParseFloat(integer, 64); err != nil {
				return Decimal{kind: decimalInfinite}
			}
			return Decimal{m: makeMag(integer, "")}
		}
	}
	neg := false
	body := s
	switch body[0] {
	case '-':
		neg, body = true, body[1:]
	case '+':
		body = body[1:]
	}
	if body == "Infinity" {
		return Decimal{kind: decimalInfinite, neg: neg}
	}
	mantissa, exp := body, 0
	if i := strings.IndexAny(body, "eE"); i >= 0 {
		mantissa = body[:i]
		e := body[i+1:]
		if e == "" || e == "+" || e == "-" {
			return Decimal{kind: decimalNaN}
		}
		n, err := strconv.Atoi(e)
		if err != nil {
			if !allDigits(strings.TrimLeft(e, "+-")) {
				return Decimal{kind: decimalNaN}
			}
			// An exponent too large for an int: the number is an infinity
			// or zero, as far as any formatter could write it.
			if strings.HasPrefix(e, "-") {
				return Decimal{neg: neg, m: zeroMag}
			}
			return Decimal{kind: decimalInfinite, neg: neg}
		}
		exp = n
	}
	integer, fraction, _ := strings.Cut(mantissa, ".")
	if integer == "" && fraction == "" || !allDigits(integer) || !allDigits(fraction) ||
		strings.Count(mantissa, ".") > 1 {
		return Decimal{kind: decimalNaN}
	}
	m := makeMag(integer, fraction)
	if !m.isZero() {
		// RoundMVResult: what the number would be as a float decides
		// whether it is an infinity.
		v, _ := strconv.ParseFloat(integer+"."+fraction+"e"+strconv.Itoa(exp), 64)
		if math.IsInf(v, 0) {
			return Decimal{kind: decimalInfinite, neg: neg}
		}
	}
	return Decimal{neg: neg, m: m.shift(exp)}
}

// zeroMag is zero's digits.
var zeroMag = mag{integer: "0"}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// baseToDecimal converts digits in base 2, 8 or 16 to decimal digits.
func baseToDecimal(s string, base int) (string, bool) {
	if s == "" {
		return "", false
	}
	out := []byte{0} // little-endian decimal digits
	for i := 0; i < len(s); i++ {
		d, err := strconv.ParseUint(s[i:i+1], base, 8)
		if err != nil {
			return "", false
		}
		carry := int(d)
		for j := range out {
			v := int(out[j])*base + carry
			out[j], carry = byte(v%10), v/10
		}
		for carry > 0 {
			out = append(out, byte(carry%10))
			carry /= 10
		}
	}
	b := make([]byte, len(out))
	for i, d := range out {
		b[len(out)-1-i] = '0' + d
	}
	return string(b), true
}

// isJSWhitespace is JavaScript's WhiteSpace and LineTerminator.
func isJSWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// IsNaN reports whether the Decimal is not a number.
func (d Decimal) IsNaN() bool { return d.kind == decimalNaN }

// IsInf reports whether the Decimal is an infinity.
func (d Decimal) IsInf() bool { return d.kind == decimalInfinite }

// Negative reports whether the Decimal has a minus sign, negative zero
// included.
func (d Decimal) Negative() bool { return d.neg }

// isZero reports whether the number is zero, of either sign.
func (d Decimal) isZero() bool { return d.kind == decimalFinite && d.m.isZero() }

// abs is the number without its sign.
func (d Decimal) abs() Decimal {
	d.neg = false
	return d
}

// Float is the nearest float to the Decimal.
func (d Decimal) Float() float64 {
	switch d.kind {
	case decimalNaN:
		return math.NaN()
	case decimalInfinite:
		if d.neg {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	v := d.m.float()
	if d.neg {
		v = -v
	}
	return v
}

// String writes the Decimal plainly, "-0.001".
func (d Decimal) String() string {
	switch d.kind {
	case decimalNaN:
		return "NaN"
	case decimalInfinite:
		if d.neg {
			return "-Infinity"
		}
		return "Infinity"
	}
	s := d.m.integer
	if d.m.fraction != "" {
		s += "." + d.m.fraction
	}
	if d.neg {
		s = "-" + s
	}
	return s
}
