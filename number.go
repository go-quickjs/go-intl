package intl

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/numdata"
)

// Intl.NumberFormat.
//
// A formatter is built once and then used: it holds the locale's data and the
// pattern it has already taken apart, it never changes afterwards, and it is
// safe for any number of goroutines to share. There is no process-wide state
// behind it and nothing to warm up.

// Style is what kind of quantity a number is.
type Style int

const (
	// StyleDecimal is a plain number, and the default.
	StyleDecimal Style = iota
	StylePercent
	StyleCurrency
)

// CurrencyDisplay is how the currency is named.
type CurrencyDisplay int

const (
	// CurrencySymbol writes the locale's symbol, and is the default.
	CurrencySymbol CurrencyDisplay = iota
	CurrencyNarrowSymbol
	CurrencyCode
	// CurrencyName writes the currency's name, which is chosen by the plural
	// category of the amount and so waits on plural rules.
	CurrencyName
)

// SignDisplay is when a sign is written.
type SignDisplay int

const (
	// SignAuto writes a sign only for a negative number, and is the default.
	SignAuto SignDisplay = iota
	SignAlways
	SignExceptZero
	SignNever
	SignNegative
)

// Grouping is whether the thousands separators appear. Its zero value is the
// default, which is that they do.
type Grouping int

const (
	GroupingAuto Grouping = iota
	GroupingNever
	GroupingAlways
)

// Notation is how the magnitude is written.
type Notation int

const (
	// NotationStandard writes every digit, and is the default.
	NotationStandard Notation = iota
	// NotationCompact writes "1.2M", which needs the compact patterns and,
	// for the long form, plural rules.
	NotationCompact
	NotationScientific
	NotationEngineering
)

// CompactDisplay is how a compact magnitude is written.
type CompactDisplay int

const (
	// CompactShort writes "1.2K", and is the default.
	CompactShort CompactDisplay = iota
	// CompactLong writes "1.2 thousand".
	CompactLong
)

// Digits returns a pointer to n, for the option fields where zero is a setting
// a caller may mean and so cannot also stand for "not given".
func Digits(n int) *int { return &n }

// NumberFormatOptions mirrors the option bag of Intl.NumberFormat. Its zero
// value is ECMA-402's default in every field.
type NumberFormatOptions struct {
	Style           Style
	Currency        string
	CurrencyDisplay CurrencyDisplay
	Notation        Notation
	CompactDisplay  CompactDisplay
	SignDisplay     SignDisplay
	UseGrouping     Grouping

	// MinimumIntegerDigits is at least one; zero means the default.
	MinimumIntegerDigits int
	// MinimumFractionDigits and MaximumFractionDigits are pointers because
	// zero is a setting: asking for no decimals is not the same as not asking.
	MinimumFractionDigits *int
	MaximumFractionDigits *int

	// Compat chooses between the standard and Node's observable behavior.
	Compat Compat
}

// A NumberFormat writes numbers in one locale.
type NumberFormat struct {
	locale  Locale
	data    *numdata.Locale
	pattern *pattern
	opts    NumberFormatOptions

	minInt           int
	minFrac, maxFrac int
	currencyText     string
	grouping         bool
	decimalSep       string
	groupSep         string
	minGrouping      int
	beforeCurrency   *spacingRule
	afterCurrency    *spacingRule
	plurals          *PluralRules
}

// NewNumberFormat builds a formatter from the data built into the package.
func NewNumberFormat(loc Locale, opts NumberFormatOptions) (*NumberFormat, error) {
	return NewNumberFormatFrom(Embedded, loc, opts)
}

// NewNumberFormatFrom builds a formatter from a source of the caller's own.
func NewNumberFormatFrom(src Source, loc Locale, opts NumberFormatOptions) (*NumberFormat, error) {
	switch opts.Notation {
	case NotationStandard, NotationCompact:
	default:
		return nil, fmt.Errorf("intl: scientific and engineering notation are not implemented yet")
	}
	if opts.CurrencyDisplay == CurrencyName {
		return nil, fmt.Errorf("intl: currency names need plural rules, which are not implemented yet")
	}
	if opts.Style == StyleCurrency && opts.Currency == "" {
		return nil, fmt.Errorf("intl: a currency style needs a currency")
	}

	data, err := loadNumbers(src, loc)
	if err != nil {
		return nil, err
	}

	f := &NumberFormat{locale: loc, data: data, opts: opts}
	switch opts.Style {
	case StylePercent:
		f.pattern, err = parsePattern(data.PercentPattern)
	case StyleCurrency:
		f.pattern, err = parsePattern(data.CurrencyPattern)
	default:
		f.pattern, err = parsePattern(data.DecimalPattern)
	}
	if err != nil {
		return nil, fmt.Errorf("intl: %s: %w", loc, err)
	}

	if err := f.resolveDigits(src); err != nil {
		return nil, err
	}
	if err := f.resolveCurrency(); err != nil {
		return nil, err
	}
	if opts.Style == StyleCurrency {
		if f.beforeCurrency, err = compileSpacing(data.BeforeCurrency.CurrencyMatch,
			data.BeforeCurrency.SurroundingMatch, data.BeforeCurrency.InsertBetween); err != nil {
			return nil, fmt.Errorf("intl: %s: the currency spacing: %w", loc, err)
		}
		if f.afterCurrency, err = compileSpacing(data.AfterCurrency.CurrencyMatch,
			data.AfterCurrency.SurroundingMatch, data.AfterCurrency.InsertBetween); err != nil {
			return nil, fmt.Errorf("intl: %s: the currency spacing: %w", loc, err)
		}
	}

	if opts.Notation == NotationCompact {
		// The compact patterns are chosen by the plural category of the
		// divided amount, so a compact formatter carries the rules.
		if f.plurals, err = NewPluralRulesFrom(src, loc, PluralRulesOptions{}); err != nil {
			return nil, err
		}
	}

	f.grouping = opts.UseGrouping != GroupingNever && f.pattern.primaryGroup > 0
	f.minGrouping = data.MinimumGroupingDigits
	switch {
	case opts.UseGrouping == GroupingAlways:
		f.minGrouping = 1
	case opts.Notation == NotationCompact && opts.UseGrouping == GroupingAuto:
		// ECMA-402 groups a compact number only when the leading group has two
		// digits of its own, so 1235 thousand is "1235" and not "1,235". It
		// calls that "min2".
		f.minGrouping = max(2, f.minGrouping)
	}
	f.decimalSep, f.groupSep = data.Symbols.Decimal, data.Symbols.Group
	if opts.Style == StyleCurrency {
		if data.Symbols.CurrencyDecimal != "" {
			f.decimalSep = data.Symbols.CurrencyDecimal
		}
		if data.Symbols.CurrencyGroup != "" {
			f.groupSep = data.Symbols.CurrencyGroup
		}
	}
	return f, nil
}

// loadNumbers walks the fallback chain until a locale has number data. The
// root always does, so this always finds something or the data is broken.
func loadNumbers(src Source, loc Locale) (*numdata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.Chain(loc.Data())
	}
	for _, d := range chain {
		b, err := src.Open(MarkerNumbers, d)
		if err != nil {
			continue
		}
		l, err := numdata.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the number data for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no number data for %s: %w", loc, ErrNotFound)
}

// resolveDigits settles how many digits are written, by ECMA-402's rules
// rather than the pattern's. The pattern decides the layout; the option bag
// and the style decide the counts.
func (f *NumberFormat) resolveDigits(src Source) error {
	minFracDefault, maxFracDefault := 0, 3
	switch f.opts.Style {
	case StylePercent:
		minFracDefault, maxFracDefault = 0, 0
	case StyleCurrency:
		d, err := currencyDigits(src, f.opts.Currency)
		if err != nil {
			return err
		}
		minFracDefault, maxFracDefault = d, d
	}

	f.minInt = f.opts.MinimumIntegerDigits
	if f.minInt <= 0 {
		f.minInt = 1
	}
	f.minFrac = minFracDefault
	if f.opts.MinimumFractionDigits != nil {
		f.minFrac = *f.opts.MinimumFractionDigits
	}
	f.maxFrac = max(f.minFrac, maxFracDefault)
	if f.opts.MaximumFractionDigits != nil {
		f.maxFrac = *f.opts.MaximumFractionDigits
	}
	if f.minFrac > f.maxFrac {
		return fmt.Errorf("intl: at least %d decimals but at most %d",
			f.minFrac, f.maxFrac)
	}
	return nil
}

// resolveCurrency settles what is written where the pattern has its currency
// mark. A currency the locale has no symbol for is written as its code, which
// is what ICU does.
func (f *NumberFormat) resolveCurrency() error {
	if f.opts.Style != StyleCurrency {
		return nil
	}
	code := strings.ToUpper(f.opts.Currency)
	f.currencyText = code
	switch f.opts.CurrencyDisplay {
	case CurrencyCode:
	case CurrencyNarrowSymbol:
		if c, ok := f.data.Currency(code); ok {
			if c.Narrow != "" {
				f.currencyText = c.Narrow
			} else if c.Symbol != "" {
				f.currencyText = c.Symbol
			}
		}
	default:
		if c, ok := f.data.Currency(code); ok && c.Symbol != "" {
			f.currencyText = c.Symbol
		}
	}
	return nil
}

// A PartKind says what one piece of a formatted number is, matching the types
// Intl.NumberFormat.prototype.formatToParts gives.
type PartKind string

const (
	PartInteger     PartKind = "integer"
	PartGroup       PartKind = "group"
	PartDecimal     PartKind = "decimal"
	PartFraction    PartKind = "fraction"
	PartMinusSign   PartKind = "minusSign"
	PartPlusSign    PartKind = "plusSign"
	PartPercentSign PartKind = "percentSign"
	PartCurrency    PartKind = "currency"
	PartLiteral     PartKind = "literal"
	PartNaN         PartKind = "nan"
	PartInfinity    PartKind = "infinity"
)

// A Part is one piece of a formatted number.
type Part struct {
	Kind  PartKind
	Value string
}

// Format writes a number.
func (f *NumberFormat) Format(v float64) string {
	var b strings.Builder
	for _, p := range f.FormatToParts(v) {
		b.WriteString(p.Value)
	}
	return b.String()
}

// AppendFormat appends a number to dst, for a caller who would rather not
// allocate a string for every value.
func (f *NumberFormat) AppendFormat(dst []byte, v float64) []byte {
	for _, p := range f.FormatToParts(v) {
		dst = append(dst, p.Value...)
	}
	return dst
}

// FormatToParts writes a number as the pieces it is made of.
//
// This is not a convenience. Parts cannot be recovered from a finished string,
// so having to produce them is what keeps the formatter assembling a number
// out of a pattern rather than looking an answer up.
func (f *NumberFormat) FormatToParts(v float64) []Part {
	negative := math.Signbit(v)
	magnitude := math.Abs(v)
	if f.opts.Style == StylePercent {
		magnitude *= 100
	}

	var parts []Part
	add := func(kind PartKind, value string) {
		if value != "" {
			parts = append(parts, Part{kind, value})
		}
	}

	sign, showSign := f.signFor(v, negative)
	prefix := f.pattern.prefixFor(negative)
	suffix := f.pattern.suffixFor(negative)

	// A pattern with a negative form of its own already carries the sign in
	// its affixes, so one is not written twice.
	if showSign && !(negative && f.pattern.explicitNeg) {
		add(sign.kind, sign.text)
	}
	parts = append(parts, f.affixParts(prefix)...)

	switch {
	case math.IsNaN(v):
		add(PartNaN, f.data.Symbols.NaN)
	case math.IsInf(v, 0):
		add(PartInfinity, f.data.Symbols.Infinity)
	case f.opts.Notation == NotationCompact:
		parts = append(parts, f.compactParts(magnitude)...)
	default:
		parts = append(parts, f.numberParts(magnitude)...)
	}

	parts = append(parts, f.affixParts(suffix)...)
	return f.spaceCurrency(parts)
}

// spaceCurrency puts CLDR's space between the currency and the number where
// the two would otherwise run together: "USD1.00" becomes "USD 1.00",
// while "$1.00" is left alone because a dollar sign is a symbol.
//
// Only a currency directly against the number counts. Where the pattern
// already puts something between them there is nothing to separate.
func (f *NumberFormat) spaceCurrency(parts []Part) []Part {
	if f.opts.Style != StyleCurrency || len(parts) < 2 {
		return parts
	}
	for i := 0; i < len(parts)-1; i++ {
		left, right := parts[i], parts[i+1]
		var rule *spacingRule
		var currencyFacing, numberFacing rune
		var ok bool
		switch {
		case left.Kind == PartCurrency && isNumberPart(right.Kind):
			rule = f.afterCurrency
			if currencyFacing, ok = lastRune(left.Value); ok {
				numberFacing, ok = firstRune(right.Value)
			}
		case isNumberPart(left.Kind) && right.Kind == PartCurrency:
			rule = f.beforeCurrency
			if currencyFacing, ok = firstRune(right.Value); ok {
				numberFacing, ok = lastRune(left.Value)
			}
		default:
			continue
		}
		if !ok || !rule.applies(currencyFacing, numberFacing) {
			continue
		}
		parts = append(parts, Part{})
		copy(parts[i+2:], parts[i+1:])
		parts[i+1] = Part{PartLiteral, rule.insert}
		i++
	}
	return parts
}

// isNumberPart reports whether a piece is part of the number itself, as
// opposed to an affix or a sign.
func isNumberPart(k PartKind) bool {
	switch k {
	case PartInteger, PartFraction, PartDecimal, PartGroup, PartNaN, PartInfinity:
		return true
	}
	return false
}

type signPart struct {
	kind PartKind
	text string
}

// signFor decides whether a sign is written and which one.
func (f *NumberFormat) signFor(v float64, negative bool) (signPart, bool) {
	minus := signPart{PartMinusSign, f.data.Symbols.MinusSign}
	plus := signPart{PartPlusSign, f.data.Symbols.PlusSign}
	zero := v == 0 || math.IsNaN(v)

	switch f.opts.SignDisplay {
	case SignNever:
		return signPart{}, false
	case SignAlways:
		if negative {
			return minus, true
		}
		return plus, true
	case SignExceptZero:
		if zero {
			return signPart{}, false
		}
		if negative {
			return minus, true
		}
		return plus, true
	case SignNegative:
		if negative && !zero {
			return minus, true
		}
		return signPart{}, false
	default:
		if negative {
			return minus, true
		}
		return signPart{}, false
	}
}

// numberParts writes the digits themselves.
func (f *NumberFormat) numberParts(magnitude float64) []Part {
	integer, fraction := digitsOf(magnitude, f.maxFrac)
	fraction = trimTrailingZeros(fraction, f.minFrac)
	for len(fraction) < f.minFrac {
		fraction += "0"
	}
	integer = padInteger(integer, f.minInt)

	parts := f.groupedInteger(integer)

	if fraction != "" {
		parts = append(parts, Part{PartDecimal, f.decimalSep})
		parts = append(parts, Part{PartFraction, f.digits(fraction)})
	}
	return parts
}

// groupedInteger writes the integer digits with the locale's separators put
// where the pattern says.
func (f *NumberFormat) groupedInteger(integer string) []Part {
	var at []int
	if f.grouping {
		at = groupPositions(len(integer), f.pattern.primaryGroup,
			f.pattern.secondaryGroup, f.minGrouping)
	}
	var parts []Part
	last := 0
	for _, pos := range at {
		parts = append(parts, Part{PartInteger, f.digits(integer[last:pos])})
		parts = append(parts, Part{PartGroup, f.groupSep})
		last = pos
	}
	return append(parts, Part{PartInteger, f.digits(integer[last:])})
}

func (f *NumberFormat) digits(s string) string {
	return mapDigits(s, f.data.Digits)
}

// affixParts turns a pattern's prefix or suffix into pieces, replacing the
// marks that stand for something: the currency and the percent sign.
func (f *NumberFormat) affixParts(affix string) []Part {
	if affix == "" {
		return nil
	}
	var parts []Part
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			parts = append(parts, Part{PartLiteral, literal.String()})
			literal.Reset()
		}
	}
	for _, r := range affix {
		switch r {
		case '¤': // the currency mark
			flush()
			parts = append(parts, Part{PartCurrency, f.currencyText})
		case '%':
			flush()
			parts = append(parts, Part{PartPercentSign, f.data.Symbols.PercentSign})
		case '-':
			flush()
			parts = append(parts, Part{PartMinusSign, f.data.Symbols.MinusSign})
		case '+':
			flush()
			parts = append(parts, Part{PartPlusSign, f.data.Symbols.PlusSign})
		default:
			literal.WriteRune(r)
		}
	}
	flush()
	return parts
}

// ResolvedNumberFormat is what a formatter settled on, mirroring
// Intl.NumberFormat.prototype.resolvedOptions.
type ResolvedNumberFormat struct {
	Locale                string
	NumberingSystem       string
	Style                 Style
	Currency              string
	CurrencyDisplay       CurrencyDisplay
	MinimumIntegerDigits  int
	MinimumFractionDigits int
	MaximumFractionDigits int
	UseGrouping           bool
	SignDisplay           SignDisplay
	Notation              Notation
}

// ResolvedOptions returns what the formatter settled on.
func (f *NumberFormat) ResolvedOptions() ResolvedNumberFormat {
	return ResolvedNumberFormat{
		Locale:                f.locale.String(),
		NumberingSystem:       f.data.NumberingSystem,
		Style:                 f.opts.Style,
		Currency:              strings.ToUpper(f.opts.Currency),
		CurrencyDisplay:       f.opts.CurrencyDisplay,
		MinimumIntegerDigits:  f.minInt,
		MinimumFractionDigits: f.minFrac,
		MaximumFractionDigits: f.maxFrac,
		UseGrouping:           f.grouping,
		SignDisplay:           f.opts.SignDisplay,
		Notation:              f.opts.Notation,
	}
}

// currencyDigits is how many decimals a currency is written with: none for the
// yen, three for the dinar, two for most. It does not vary by locale, so it is
// one table rather than one per locale.
func currencyDigits(src Source, code string) (int, error) {
	b, err := src.Open(MarkerCurrencyDigits, DataLocale{})
	if err != nil {
		// Without the table every currency takes the usual two decimals, which
		// is right for most of them.
		return 2, nil
	}
	want := strings.ToUpper(code)
	for line := range strings.SplitSeq(strings.TrimRight(string(b), "\n"), "\n") {
		name, digits, ok := strings.Cut(line, ":")
		if !ok || name != want {
			continue
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			return 0, fmt.Errorf("intl: the digits for %s: %w", want, err)
		}
		return n, nil
	}
	return 2, nil
}
