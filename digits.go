package intl

import (
	"fmt"
	"strings"
)

// How many digits get written, and how the rest are rounded away.
//
// Both NumberFormat and PluralRules need this, and they need the same answer:
// which plural form a number takes depends on how it would be written, so the
// two cannot disagree about the digits without disagreeing about plurals. It
// lives here rather than inside either of them.

// roundingKind is which way of counting digits decides the rounding.
type roundingKind int

const (
	roundFractionDigits roundingKind = iota
	roundSignificantDigits
	roundMorePrecision
	roundLessPrecision
)

// A digitRequest is the part of an option bag that decides the plan.
type digitRequest struct {
	minInt           int
	minFrac, maxFrac *int
	minSig, maxSig   *int
	priority         RoundingPriority
	mode             RoundingMode
	increment        int
	trailingZero     TrailingZeroDisplay
	// compact says the number will be written in compact notation, which has
	// its own default: two significant digits or no decimals, whichever keeps
	// more.
	compact bool
	// The defaults for this style: none for a plain number, two for most
	// money, none at all for a percentage.
	minFracDefault, maxFracDefault int
}

// A digitPlan is what a request settled on.
type digitPlan struct {
	minInt           int
	minFrac, maxFrac int
	minSig, maxSig   int
	rounding         roundingKind
	mode             RoundingMode
	increment        int
	trailingZero     TrailingZeroDisplay
}

// resolve settles the counts, by ECMA-402's SetNumberFormatDigitOptions.
//
// There are two ways of counting and both can be given. Which wins is:
// significant digits alone, decimals alone, or -- when both are given with a
// priority -- whichever of the two keeps more or less.
func (r digitRequest) resolve() (digitPlan, error) {
	var p digitPlan
	p.mode, p.increment, p.trailingZero = r.mode, r.increment, r.trailingZero

	p.minInt = r.minInt
	if p.minInt <= 0 {
		p.minInt = 1
	}

	hasFrac := r.minFrac != nil || r.maxFrac != nil
	hasSig := r.minSig != nil || r.maxSig != nil

	p.minFrac = r.minFracDefault
	if r.minFrac != nil {
		p.minFrac = *r.minFrac
	}
	p.maxFrac = max(p.minFrac, r.maxFracDefault)
	if r.maxFrac != nil {
		p.maxFrac = *r.maxFrac
	}
	if p.minFrac > p.maxFrac {
		return p, fmt.Errorf("intl: at least %d decimals but at most %d",
			p.minFrac, p.maxFrac)
	}

	p.minSig, p.maxSig = 1, 21
	if r.minSig != nil {
		p.minSig = *r.minSig
	}
	if r.maxSig != nil {
		p.maxSig = *r.maxSig
	}
	if hasSig && p.minSig > p.maxSig {
		return p, fmt.Errorf("intl: at least %d significant digits but at most %d",
			p.minSig, p.maxSig)
	}

	switch {
	case r.priority == MorePrecision:
		p.rounding = roundMorePrecision
	case r.priority == LessPrecision:
		p.rounding = roundLessPrecision
	case hasSig:
		// Significant digits win over decimals when both are given without a
		// priority, which is what ECMA-402 calls auto.
		p.rounding = roundSignificantDigits
	case hasFrac:
		p.rounding = roundFractionDigits
	case r.compact:
		p.minSig, p.maxSig = 1, 2
		p.minFrac, p.maxFrac = 0, 0
		p.rounding = roundMorePrecision
	default:
		p.rounding = roundFractionDigits
	}

	if p.increment > 1 && (p.rounding != roundFractionDigits || p.minFrac != p.maxFrac) {
		return p, fmt.Errorf("intl: a rounding increment needs the same " +
			"smallest and largest number of decimals and no significant digits")
	}
	return p, nil
}

// round cuts a number down to the digits that will be written, by whichever
// way of counting the options settled on, and then pads or trims the decimals.
func (p *digitPlan) round(magnitude mag, negative bool) (string, string) {
	var integer, fraction string
	switch p.rounding {
	case roundSignificantDigits:
		integer, fraction = roundSignificant(magnitude, p.maxSig, negative, p.mode)
	case roundMorePrecision, roundLessPrecision:
		// The two ways are compared by where each would round: the smaller
		// place keeps more. Which of the two is wanted is the priority.
		sig := significantPlace(magnitude, p.maxSig)
		frac := -p.maxFrac
		useSig := sig < frac
		if p.rounding == roundLessPrecision {
			useSig = sig > frac
		}
		if useSig {
			integer, fraction = roundSignificant(magnitude, p.maxSig, negative, p.mode)
		} else {
			integer, fraction = roundAt(magnitude, p.maxFrac, negative, p.mode)
		}
	default:
		integer, fraction = roundAt(magnitude, p.maxFrac, negative, p.mode)
		if p.increment > 1 {
			integer, fraction = roundToIncrement(integer, fraction, p.maxFrac,
				p.increment, negative, p.mode)
		}
	}
	return p.pad(integer, fraction)
}

// pad trims the decimals that were not asked for and writes the ones that were.
func (p *digitPlan) pad(integer, fraction string) (string, string) {
	minFrac := p.minFrac
	switch p.rounding {
	case roundSignificantDigits, roundMorePrecision, roundLessPrecision:
		// Significant digits set their own minimum: enough decimals to make up
		// the smallest count, and no more.
		minFrac = 0
		if strings.Trim(integer, "0") == "" && strings.Trim(fraction, "0") == "" {
			// Zero counts as one significant digit of its own, so it takes
			// decimals only when more than one was asked for.
			minFrac = p.minSig - 1
		} else if digits := len(strings.TrimLeft(integer, "0")); digits < p.minSig {
			if integer == "0" || digits == 0 {
				// A number below one counts its significant digits from the
				// first that is not a zero, wherever that falls.
				lead := len(fraction) - len(strings.TrimLeft(fraction, "0"))
				minFrac = lead + p.minSig
			} else {
				minFrac = p.minSig - digits
			}
		}
		if minFrac > len(fraction) {
			minFrac = min(minFrac, p.maxSig+len(fraction))
		}
	}
	fraction = trimTrailingZeros(fraction, minFrac)
	for len(fraction) < minFrac {
		fraction += "0"
	}
	if p.trailingZero == TrailingZeroStripIfInteger && strings.Trim(fraction, "0") == "" {
		fraction = ""
	}
	return integer, fraction
}
