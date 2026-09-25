package intl

import (
	"strings"
	"unicode"

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
	// exponent is the power of ten the number is divided by before it is
	// written, which is the "c" operand a plural rule may test.
	exponent int
	prefix   string
	suffix   string
	// zeros is how many digits the pattern asks for, which sets the minimum
	// integer digits of the divided amount.
	zeros int
	// noBody is a pattern with no digits, French "mille" for exactly a
	// thousand: ICU writes its text in place of the number.
	noBody bool
}

// chooseCompact picks the pattern for a magnitude. It returns false when the
// number is too small to compact, which is every number below a thousand.
func chooseCompact(patterns []numdata.CompactPattern, want int) (int, bool) {
	if want < 0 {
		return 0, false
	}
	// The largest power of ten the patterns cover that is no larger than the
	// number itself.
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
		// A pattern with no digits divides as the magnitude's others do.
		if count == "other" {
			return compactForm{}, false
		}
		other, ok := compactFor(patterns, exponent, "other")
		if !ok {
			return compactForm{}, false
		}
		return compactForm{exponent: other.exponent, prefix: unquote(chosen), zeros: other.zeros, noBody: true}, true
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
	return form, true
}

// hasCompactCount reports whether a magnitude has a pattern for a count of
// its own, rather than by falling back to "other".
func hasCompactCount(patterns []numdata.CompactPattern, exponent int, count string) bool {
	for _, p := range patterns {
		if p.Exponent == exponent && p.Count == count {
			return true
		}
	}
	return false
}

// compactAffix writes a compact pattern's text as a "compact" part, with
// the spaces around it as literals, as ICU trims the whitespace off a
// field's ends.
func compactAffix(parts []Part, text string) []Part {
	core := strings.TrimLeftFunc(text, isIgnorable)
	if lead := text[:len(text)-len(core)]; lead != "" {
		parts = append(parts, Part{PartLiteral, lead})
	}
	trimmed := strings.TrimRightFunc(core, isIgnorable)
	if trimmed != "" {
		parts = append(parts, Part{PartCompact, trimmed})
	}
	if trail := core[len(trimmed):]; trail != "" {
		parts = append(parts, Part{PartLiteral, trail})
	}
	return parts
}

// isIgnorable is ICU's DEFAULT_IGNORABLES: the space separators, the tab,
// the bidirectional controls and the variation selectors.
func isIgnorable(r rune) bool {
	switch {
	case r == '\t', unicode.Is(unicode.Zs, r), unicode.Is(unicode.Bidi_Control, r),
		unicode.Is(unicode.Variation_Selector, r):
		return true
	}
	return false
}

// compactParts writes a number in compact notation.
func (f *NumberFormat) compactParts(magnitude mag, negative bool) []Part {
	form := f.compactForm(magnitude, negative)
	if form.noBody {
		return compactAffix(nil, form.prefix)
	}

	// The digit counts are settled once, when the formatter is built, so a
	// compact number rounds the same way as any other: by whichever counting
	// the options chose, which for a compact number with nothing asked for is
	// two significant digits or no decimals, whichever keeps more.
	value := magnitude.shift(-form.exponent)
	integer, fraction := f.round(value, negative)
	integer = padInteger(integer, max(f.minInt, 1))

	parts := compactAffix(nil, form.prefix)
	parts = append(parts, f.groupedInteger(integer)...)
	if fraction != "" {
		parts = append(parts, Part{PartDecimal, f.decimalSep})
		parts = append(parts, Part{PartFraction, f.digits(fraction)})
	}
	return compactAffix(parts, form.suffix)
}

// compactForm chooses the compact pattern a magnitude is written with.
func (f *NumberFormat) compactForm(magnitude mag, negative bool) compactForm {
	patterns := f.data.CompactShort
	if f.opts.CompactDisplay == CompactLong {
		patterns = f.data.CompactLong
	}

	form := compactForm{zeros: 1}
	if magnitude.isZero() {
		return form
	}
	// ICU's chooseMultiplierAndApply: the pattern is the one for the
	// number's power of ten, unless rounding carries it into the next power
	// and that one divides by a different amount -- 999,999.5 is "1M", not
	// "1000K".
	want := magnitude.exponent()
	divisor := func(want int) int {
		if exponent, ok := chooseCompact(patterns, want); ok {
			if first, ok := compactFor(patterns, exponent, "other"); ok {
				return first.exponent
			}
		}
		return 0
	}
	by := divisor(want)
	integer, fraction := f.round(magnitude.shift(-by), negative)
	if rounded := makeMag(integer, fraction); !rounded.isZero() && rounded.exponent()+by != want &&
		divisor(want+1) != by {
		want++
	}
	exponent, ok := chooseCompact(patterns, want)
	if ok {
		// The plural category is that of the divided amount, so the amount has
		// to be divided before the pattern can be chosen -- and the pattern is
		// what says by how much. CLDR's patterns for one magnitude all divide
		// by the same amount, so a first pass with the "other" category settles
		// the divisor and a second picks the wording. ICU asks the plural
		// rules about the divided amount alone, before it records the power
		// of ten it divided by.
		if first, ok := compactFor(patterns, exponent, "other"); ok {
			divided := magnitude.shift(-first.exponent)
			integer, fraction := f.round(divided, negative)
			// CompactData::getPattern: an amount that is exactly 0 or 1
			// takes the pattern CLDR gives for that number, where it gives
			// one -- French writes a thousand as "mille". The amount is
			// signed: minus a thousand is "-1 millier".
			if strings.Trim(fraction, "0") == "" {
				exact := ""
				switch strings.TrimLeft(integer, "0") {
				case "":
					exact = "0"
				case "1":
					if !negative {
						exact = "1"
					}
				}
				if exact != "" && hasCompactCount(patterns, exponent, exact) {
					if form, ok := compactFor(patterns, exponent, exact); ok {
						return form
					}
				}
			}
			category := string(PluralOther)
			if f.plurals != nil {
				o := operandsFor(integer, fraction, 0)
				category = string(f.plurals.selectOperands(&o))
			}
			if chosen, ok := compactFor(patterns, exponent, category); ok {
				form = chosen
			} else {
				form = first
			}
		}
	}
	return form
}

// selectOperands is Select for operands that have already been worked out,
// which the compact path needs because it has the digits in hand.
func (p *PluralRules) selectOperands(o *operands) PluralCategory {
	for _, r := range p.tried {
		if r.rule.matches(o) {
			return r.category
		}
	}
	return PluralOther
}
