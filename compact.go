package intl

import (
	"math"
	"strings"

	"github.com/go-quickjs/go-intl/internal/numdata"
)

// Compact notation: 1234 as "1.2K", or "1.2 thousand".
//
// Two things make this more than dividing and appending a letter.
//
// The pattern is chosen by the plural category of the divided amount, because
// a language may write one million differently from three. That is why this
// waits on plural rules rather than arriving with the rest of NumberFormat.
//
// The rounding is ECMA-402's "more precision", which is not a rounding mode so
// much as a choice between two. A compact number is rounded both to no
// decimals and to two significant digits, and whichever keeps more is used:
// 1.2345 thousand is "1.2K" because two significant digits keep more than no
// decimals would, and 123.456 billion is "123B" because no decimals keeps more
// than two significant digits would.

// compactForm is the pattern chosen for one magnitude and plural category.
type compactForm struct {
	// divisor is what the number is divided by before it is written.
	divisor float64
	// exponent is the power of ten that divisor stands for, which is the "c"
	// operand a plural rule may test.
	exponent int
	prefix   string
	suffix   string
	// zeros is how many digits the pattern asks for, which sets the minimum
	// integer digits of the divided amount.
	zeros int
}

// chooseCompact picks the pattern for a magnitude. It returns false when the
// number is too small to compact, which is every number below a thousand.
func chooseCompact(patterns []numdata.CompactPattern, magnitude float64) (int, bool) {
	if magnitude < 1 {
		return 0, false
	}
	// The largest power of ten the patterns cover that is no larger than the
	// number itself.
	want := int(math.Floor(math.Log10(magnitude)))
	best, found := 0, false
	for _, p := range patterns {
		if p.Exponent <= want && (!found || p.Exponent > best) {
			best, found = p.Exponent, true
		}
	}
	return best, found
}

// compactFor builds the form for one exponent and plural category, falling
// back to the "other" category as CLDR's own lookup does.
func compactFor(patterns []numdata.CompactPattern, exponent int, count string) (compactForm, bool) {
	var chosen string
	for _, p := range patterns {
		if p.Exponent != exponent {
			continue
		}
		if p.Count == count {
			chosen = p.Pattern
			break
		}
		if p.Count == "other" && chosen == "" {
			chosen = p.Pattern
		}
	}
	if chosen == "" {
		return compactForm{}, false
	}

	// The zeros in the pattern say how many digits of the magnitude stay: "0K"
	// at a thousand divides by a thousand, "00K" at ten thousand also divides
	// by a thousand and writes two digits.
	start := strings.IndexByte(chosen, '0')
	if start < 0 {
		return compactForm{}, false
	}
	end := start
	for end < len(chosen) && chosen[end] == '0' {
		end++
	}
	zeros := end - start
	form := compactForm{
		exponent: exponent - (zeros - 1),
		prefix:   unquote(chosen[:start]),
		suffix:   unquote(chosen[end:]),
		zeros:    zeros,
	}
	form.divisor = math.Pow(10, float64(form.exponent))
	return form, true
}

// significantDigits rounds a number to at most n significant digits, giving
// the integer and fraction parts. It is the other half of "more precision".
func significantDigits(v float64, n int) (integer, fraction string) {
	if v == 0 || n <= 0 {
		return "0", ""
	}
	// The place to round at is n digits after the first significant one.
	e := int(math.Floor(math.Log10(v)))
	places := n - 1 - e
	if places < 0 {
		// Rounding above the decimal point: round at the right place and then
		// put the zeros back, so 123456 to two digits is 120000 rather than 12.
		scale := math.Pow(10, float64(-places))
		integer, fraction = digitsOf(v/scale, 0)
		if integer != "0" {
			integer += strings.Repeat("0", -places)
		}
		return integer, ""
	}
	return digitsOf(v, places)
}

// morePrecision picks between rounding to a fixed number of decimals and
// rounding to a number of significant digits, keeping whichever holds more.
//
// The two are compared by where they round: the one that rounds at the smaller
// place keeps more, so that is the one used.
func morePrecision(v float64, maxFrac, maxSignificant int) (integer, fraction string) {
	if v == 0 {
		return digitsOf(v, maxFrac)
	}
	e := int(math.Floor(math.Log10(v)))
	fixedAt := -maxFrac
	significantAt := e - maxSignificant + 1
	if significantAt < fixedAt {
		return significantDigits(v, maxSignificant)
	}
	return digitsOf(v, maxFrac)
}

// compactParts writes a number in compact notation.
func (f *NumberFormat) compactParts(magnitude float64) []Part {
	patterns := f.data.CompactShort
	if f.opts.CompactDisplay == CompactLong {
		patterns = f.data.CompactLong
	}

	exponent, ok := chooseCompact(patterns, magnitude)
	form := compactForm{divisor: 1, zeros: 1}
	if ok {
		// The plural category is that of the divided amount, so the amount has
		// to be divided before the pattern can be chosen -- and the pattern is
		// what says by how much. CLDR's patterns for one magnitude all divide
		// by the same amount, so a first pass with the "other" category settles
		// the divisor and a second picks the wording.
		if first, ok := compactFor(patterns, exponent, "other"); ok {
			divided := magnitude / first.divisor
			category := string(PluralOther)
			if f.plurals != nil {
				integer, fraction := morePrecision(divided, 0, 2)
				o := operandsFor(integer, fraction, first.exponent)
				category = string(f.plurals.selectOperands(&o))
			}
			if chosen, ok := compactFor(patterns, exponent, category); ok {
				form = chosen
			} else {
				form = first
			}
		}
	}

	value := magnitude / form.divisor
	var integer, fraction string
	if f.opts.MaximumFractionDigits != nil || f.opts.MinimumFractionDigits != nil {
		// A caller who asked for decimals gets those rather than the default
		// two significant digits.
		integer, fraction = digitsOf(value, f.maxFrac)
		fraction = trimTrailingZeros(fraction, f.minFrac)
		for len(fraction) < f.minFrac {
			fraction += "0"
		}
	} else {
		integer, fraction = morePrecision(value, 0, 2)
	}
	integer = padInteger(integer, max(f.minInt, 1))

	var parts []Part
	if form.prefix != "" {
		parts = append(parts, Part{PartLiteral, form.prefix})
	}
	parts = append(parts, f.groupedInteger(integer)...)
	if fraction != "" {
		parts = append(parts, Part{PartDecimal, f.decimalSep})
		parts = append(parts, Part{PartFraction, f.digits(fraction)})
	}
	if form.suffix != "" {
		parts = append(parts, Part{PartLiteral, form.suffix})
	}
	return parts
}

// selectOperands is Select for operands that have already been worked out,
// which the compact path needs because it has the digits in hand.
func (p *PluralRules) selectOperands(o *operands) PluralCategory {
	for _, r := range p.rules {
		if r.rule.matches(o) {
			return r.category
		}
	}
	return PluralOther
}
