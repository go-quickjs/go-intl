package intl

import (
	"fmt"

	"github.com/go-quickjs/go-intl/internal/plurdata"
)

// Intl.PluralRules.
//
// Which form a word takes beside a number is the language's business, not the
// number's: English has two forms, Polish four, Arabic six, Japanese one. A
// PluralRules answers which form a particular number calls for, so that a
// caller can choose between wordings it holds itself.
//
// The answer depends on how the number is *written*, not only on its value.
// English "1 day" and "1.0 days" are the same quantity and different plurals,
// because one has a written decimal and the other does not. That is why the
// digit options are here and why selecting formats the number first.

// PluralType is whether the rules are for counting or for ordering.
type PluralType int

const (
	// Cardinal is for counting: one day, two days. It is the default.
	Cardinal PluralType = iota
	// Ordinal is for ordering: the 1st, the 2nd, the 3rd.
	Ordinal
)

// PluralCategory is the form a number calls for.
type PluralCategory string

const (
	PluralZero  PluralCategory = "zero"
	PluralOne   PluralCategory = "one"
	PluralTwo   PluralCategory = "two"
	PluralFew   PluralCategory = "few"
	PluralMany  PluralCategory = "many"
	PluralOther PluralCategory = "other"
)

// PluralRulesOptions mirrors the option bag of Intl.PluralRules. Its zero
// value is ECMA-402's default in every field.
type PluralRulesOptions struct {
	Type PluralType

	// ECMA-402 gives PluralRules the whole set of digit options a
	// NumberFormat has, because which form a number takes depends on how it
	// would be written and these decide that.
	MinimumIntegerDigits     int
	MinimumFractionDigits    *int
	MaximumFractionDigits    *int
	MinimumSignificantDigits *int
	MaximumSignificantDigits *int
	RoundingPriority         RoundingPriority
	RoundingMode             RoundingMode
	RoundingIncrement        int
	TrailingZeroDisplay      TrailingZeroDisplay
	Notation                 Notation
}

// A PluralRules chooses the plural form for a number in one locale. It never
// changes after it is built and is safe for any number of goroutines to share.
type PluralRules struct {
	locale Locale
	opts   PluralRulesOptions
	rules  []compiledRule
	digitPlan
}

type compiledRule struct {
	category PluralCategory
	rule     *pluralRule
}

// NewPluralRules builds rules from the data built into the package.
func NewPluralRules(loc Locale, opts PluralRulesOptions) (*PluralRules, error) {
	return NewPluralRulesFrom(Embedded, loc, opts)
}

// NewPluralRulesFrom builds rules from a source of the caller's own.
func NewPluralRulesFrom(src Source, loc Locale, opts PluralRulesOptions) (*PluralRules, error) {
	data, err := loadPlurals(src, loc)
	if err != nil {
		return nil, err
	}
	set := data.Cardinal
	if opts.Type == Ordinal {
		set = data.Ordinal
	}

	p := &PluralRules{locale: loc, opts: opts}
	for _, r := range set {
		parsed, err := parsePluralRule(r.Condition)
		if err != nil {
			return nil, fmt.Errorf("intl: %s: the %q rule: %w", loc, r.Category, err)
		}
		p.rules = append(p.rules, compiledRule{PluralCategory(r.Category), parsed})
	}

	plan, err := digitRequest{
		minInt:         opts.MinimumIntegerDigits,
		minFrac:        opts.MinimumFractionDigits,
		maxFrac:        opts.MaximumFractionDigits,
		minSig:         opts.MinimumSignificantDigits,
		maxSig:         opts.MaximumSignificantDigits,
		priority:       opts.RoundingPriority,
		mode:           opts.RoundingMode,
		increment:      opts.RoundingIncrement,
		trailingZero:   opts.TrailingZeroDisplay,
		compact:        opts.Notation == NotationCompact,
		maxFracDefault: 3,
	}.resolve()
	if err != nil {
		return nil, err
	}
	p.digitPlan = plan
	return p, nil
}

// loadPlurals walks the fallback chain until a locale has rules. A language
// with none is not an error: the root has a single "other", which is right for
// a language that makes no distinction.
func loadPlurals(src Source, loc Locale) (*plurdata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.Chain(loc.Data())
	}
	for _, d := range chain {
		b, err := src.Open(MarkerPlurals, d)
		if err != nil {
			continue
		}
		l, err := plurdata.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the plural rules for %s: %w", d, err)
		}
		return l, nil
	}
	// Every language has "other", so having no rules at all is the same as
	// having only that one.
	return &plurdata.Locale{
		Cardinal: []plurdata.Rule{{Category: "other"}},
		Ordinal:  []plurdata.Rule{{Category: "other"}},
	}, nil
}

// Select returns the plural form a number calls for.
func (p *PluralRules) Select(v float64) PluralCategory {
	return p.selectWith(v, 0)
}

// selectWith is Select with a compact or scientific exponent, which is the "c"
// operand. A number written as "1.2M" has a different exponent from the same
// value written out, and a few languages notice.
func (p *PluralRules) selectWith(v float64, exponent int) PluralCategory {
	if v < 0 {
		v = -v
	}
	integer, fraction := p.round(v, false)
	integer = padInteger(integer, p.minInt)

	o := operandsFor(integer, fraction, exponent)
	for _, r := range p.rules {
		if r.rule.matches(&o) {
			return r.category
		}
	}
	return PluralOther
}

// Categories returns the forms this locale distinguishes, in CLDR's order.
func (p *PluralRules) Categories() []PluralCategory {
	out := make([]PluralCategory, 0, len(p.rules))
	for _, r := range p.rules {
		out = append(out, r.category)
	}
	return out
}

// ResolvedPluralRules is what a PluralRules settled on.
type ResolvedPluralRules struct {
	Locale                string
	Type                  PluralType
	MinimumIntegerDigits  int
	MinimumFractionDigits int
	MaximumFractionDigits int
	PluralCategories      []PluralCategory
}

// ResolvedOptions returns what the rules settled on.
func (p *PluralRules) ResolvedOptions() ResolvedPluralRules {
	return ResolvedPluralRules{
		Locale:                p.locale.String(),
		Type:                  p.opts.Type,
		MinimumIntegerDigits:  p.minInt,
		MinimumFractionDigits: p.minFrac,
		MaximumFractionDigits: p.maxFrac,
		PluralCategories:      p.Categories(),
	}
}
