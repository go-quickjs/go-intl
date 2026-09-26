package intl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/numdata"
	"github.com/go-quickjs/go-intl/internal/unitdata"
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
	// StyleUnit writes a measurement: "16 litres", "987 km/h".
	StyleUnit
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
	// GroupingMin2 writes a separator only where the leading group has at
	// least two digits: "1234" but "12,345". It is what compact notation
	// does by default.
	GroupingMin2
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

// CurrencySign is how a negative amount of money is written.
type CurrencySign int

const (
	// CurrencySignStandard signs it, and is the default.
	CurrencySignStandard CurrencySign = iota
	// CurrencySignAccounting brackets it, where the locale does that.
	CurrencySignAccounting
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
	Style Style
	// Unit is the measurement, for StyleUnit: "meter", or one over another,
	// "kilometer-per-hour".
	Unit            string
	UnitDisplay     UnitDisplay
	Currency        string
	CurrencyDisplay CurrencyDisplay
	Notation        Notation
	CompactDisplay  CompactDisplay
	SignDisplay     SignDisplay
	UseGrouping     Grouping

	// CurrencySign writes a loss the way accountants do, in brackets, where
	// the locale has a pattern for it.
	CurrencySign CurrencySign
	// RoundingMode, RoundingIncrement, RoundingPriority and
	// TrailingZeroDisplay are ECMA-402's controls over the last digit.
	RoundingMode        RoundingMode
	RoundingIncrement   int
	RoundingPriority    RoundingPriority
	TrailingZeroDisplay TrailingZeroDisplay

	// MinimumIntegerDigits is at least one; zero means the default.
	MinimumIntegerDigits int
	// MinimumFractionDigits and MaximumFractionDigits are pointers because
	// zero is a setting: asking for no decimals is not the same as not asking.
	MinimumFractionDigits *int
	MaximumFractionDigits *int
	// MinimumSignificantDigits and MaximumSignificantDigits count from the
	// first digit that is not a zero rather than from the point. Giving either
	// makes them decide the rounding unless a priority says otherwise.
	MinimumSignificantDigits *int
	MaximumSignificantDigits *int

	// NumberingSystem names the digits to write, "arab" or "deva". It
	// overrides -u-nu. Empty, or a name that is not a numeric system, means
	// the keyword's or else the locale's own.
	NumberingSystem string

	// Compat chooses between the standard and Node's observable behavior.
	Compat Compat
}

// A NumberFormat writes numbers in one locale.
type NumberFormat struct {
	locale  Locale
	data    *numdata.Locale
	pattern *pattern
	opts    NumberFormatOptions

	digitPlan
	currencyText   string
	grouping       bool
	primaryGroup   int
	secondaryGroup int
	decimalSep     string
	groupSep       string
	minGrouping    int
	beforeCurrency *spacingRule
	afterCurrency  *spacingRule
	plurals        *PluralRules
	units          *unitdata.Locale
	unitWidth      int
	// percentUnit is the unit "percent" at a short or narrow width and not
	// compact, which ICU writes with the locale's percent pattern, unscaled,
	// and the sign as the unit, rather than with the unit's own pattern
	// (number_formatimpl.cpp: isPercent, isCldrUnit).
	percentUnit bool
}

// NewNumberFormat builds a formatter from the data built into the package.
func NewNumberFormat(loc Locale, opts NumberFormatOptions) (*NumberFormat, error) {
	return NewNumberFormatFrom(Embedded, loc, opts)
}

// NewNumberFormatFrom builds a formatter from a source of the caller's own.
func NewNumberFormatFrom(src Source, loc Locale, opts NumberFormatOptions) (*NumberFormat, error) {
	if err := opts.check(); err != nil {
		return nil, err
	}
	s, err := loadNumberSources(src, loc, opts.NumberingSystem)
	if err != nil {
		return nil, err
	}
	return s.numberFormat(opts)
}

// numberSources are what a NumberFormat is built from, loaded once so that
// the several formatters one DurationFormat writes with share them: the
// locale's number data in its chosen numbering system, and, when a
// formatter first needs them, its units and plural rules. It is used while
// formatters are being built and never after, so filling it in as it goes
// breaks no formatter's immutability.
type numberSources struct {
	src     Source
	loc     Locale
	data    *numdata.Locale
	nu      string
	units   *unitdata.Locale
	plurals *PluralRules
}

func loadNumberSources(src Source, loc Locale, numberingSystem string) (*numberSources, error) {
	data, err := loadNumbers(src, loc)
	if err != nil {
		return nil, err
	}
	data, nu, err := selectNumberingSystem(src, data, loc, numberingSystem)
	if err != nil {
		return nil, err
	}
	return &numberSources{src: src, loc: loc, data: data, nu: nu}, nil
}

// check refuses the options no formatter can be built for.
func (opts *NumberFormatOptions) check() error {
	switch opts.Notation {
	case NotationStandard, NotationCompact, NotationScientific, NotationEngineering:
	default:
		return fmt.Errorf("intl: %d is not a notation", opts.Notation)
	}
	if opts.Style == StyleCurrency && opts.Currency == "" {
		return fmt.Errorf("intl: a currency style needs a currency")
	}
	if opts.Style == StyleUnit && !HasUnit(opts.Unit) {
		return fmt.Errorf("intl: %q is not a unit a number may be written in",
			opts.Unit)
	}
	return nil
}

// numberFormat builds a formatter from loaded sources. The options' own
// numbering system is not consulted: the sources were loaded in one.
func (s *numberSources) numberFormat(opts NumberFormatOptions) (*NumberFormat, error) {
	if err := opts.check(); err != nil {
		return nil, err
	}
	src, loc, data := s.src, s.loc, s.data
	var err error

	// Of the Unicode extension, NumberFormat uses only the numbering system.
	f := &NumberFormat{locale: loc.onlyKeywords().withKeyword("nu", s.nu), data: data, opts: opts}
	f.percentUnit = opts.Style == StyleUnit && opts.Unit == "percent" && opts.UnitDisplay != UnitLong &&
		opts.Notation != NotationCompact
	switch {
	case opts.Style == StylePercent || f.percentUnit:
		f.pattern, err = parsePattern(data.PercentPattern)
	case opts.Style == StyleCurrency:
		p := data.CurrencyPattern
		switch {
		case opts.CurrencyDisplay == CurrencyName:
			// A spelled-out name is not an affix on the number: the amount is
			// written plainly and then joined to the name by a pattern of its
			// own, which is why the currency pattern is not used here.
			p = data.DecimalPattern
		case opts.CurrencySign == CurrencySignAccounting && data.AccountingPattern != "":
			p = data.AccountingPattern
		}
		f.pattern, err = parsePattern(p)
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

	if opts.Style == StyleUnit && !f.percentUnit {
		if s.units == nil {
			if s.units, err = loadUnits(src, loc); err != nil {
				return nil, err
			}
		}
		f.units = s.units
		switch opts.UnitDisplay {
		case UnitLong:
			f.unitWidth = unitdata.Long
		case UnitNarrow:
			f.unitWidth = unitdata.Narrow
		default:
			f.unitWidth = unitdata.Short
		}
	}

	if opts.Notation == NotationCompact || opts.Style == StyleUnit ||
		(opts.Style == StyleCurrency && opts.CurrencyDisplay == CurrencyName) {
		// Both the compact patterns and the spelled-out currency names are
		// chosen by the plural category of the amount, so the formatter
		// carries the rules.
		if s.plurals == nil {
			if s.plurals, err = NewPluralRulesFrom(src, loc, PluralRulesOptions{}); err != nil {
				return nil, err
			}
		}
		f.plurals = s.plurals
	}

	f.primaryGroup, f.secondaryGroup = f.pattern.primaryGroup, f.pattern.secondaryGroup
	if opts.UseGrouping == GroupingAlways && f.primaryGroup <= 0 {
		// A pattern that does not group is grouped by threes when grouping
		// is asked for always, as ICU's Grouper does for ON_ALIGNED: POSIX's
		// "0.######".
		f.primaryGroup, f.secondaryGroup = 3, 3
	}
	f.grouping = opts.UseGrouping != GroupingNever && f.primaryGroup > 0
	f.minGrouping = data.MinimumGroupingDigits
	switch {
	case opts.UseGrouping == GroupingAlways:
		f.minGrouping = 1
	case opts.UseGrouping == GroupingMin2,
		opts.Notation == NotationCompact && opts.UseGrouping == GroupingAuto:
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
//
// The currency names are ICU's currency tree's, which sends a locale or two
// elsewhere than the number symbols' tree does: "sr-Cyrl-ME" writes Latin
// currency names. Where it does, they are read from there.
func loadNumbers(src Source, loc Locale) (*numdata.Locale, error) {
	f, err := NewFallbacker(src)
	if err != nil {
		return loadNumbersAlong(src, loc, loc.Fallback())
	}
	data, err := loadNumbersAlong(src, loc, f.ChainIn(treeLocales, loc.Data()))
	if err != nil {
		return nil, err
	}
	if curr := f.ChainIn(treeCurr, loc.Data()); curr[0] != f.ChainIn(treeLocales, loc.Data())[0] {
		names, err := loadNumbersAlong(src, loc, curr)
		if err != nil {
			return nil, err
		}
		data.TakeCurrencies(names)
	}
	return data, nil
}

func loadNumbersAlong(src Source, loc Locale, chain []DataLocale) (*numdata.Locale, error) {
	pool, err := openShared(src, MarkerNumbersShared, numdata.Version)
	if err != nil {
		return nil, err
	}
	for _, d := range chain {
		b, err := src.Open(MarkerNumbers, d)
		if err != nil {
			continue
		}
		l, err := numdata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the number data for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no number data for %s: %w", loc, ErrNotFound)
}

// resolveDigits builds the digit plan from the option bag and the style.
func (f *NumberFormat) resolveDigits(src Source) error {
	// InitializeNumberFormat: a currency's own number of decimals only in
	// standard notation; 0 to 3 otherwise, and none at all for a percentage.
	minFracDefault, maxFracDefault := 0, 3
	switch {
	case f.opts.Style == StylePercent:
		minFracDefault, maxFracDefault = 0, 0
	case f.opts.Style == StyleCurrency && f.opts.Notation == NotationStandard:
		d, err := currencyDigits(src, f.opts.Currency)
		if err != nil {
			return err
		}
		minFracDefault, maxFracDefault = d, d
	}
	plan, err := digitRequest{
		minInt:         f.opts.MinimumIntegerDigits,
		minFrac:        f.opts.MinimumFractionDigits,
		maxFrac:        f.opts.MaximumFractionDigits,
		minSig:         f.opts.MinimumSignificantDigits,
		maxSig:         f.opts.MaximumSignificantDigits,
		priority:       f.opts.RoundingPriority,
		mode:           f.opts.RoundingMode,
		increment:      f.opts.RoundingIncrement,
		trailingZero:   f.opts.TrailingZeroDisplay,
		compact:        f.opts.Notation == NotationCompact,
		minFracDefault: minFracDefault,
		maxFracDefault: maxFracDefault,
	}.resolve()
	if err != nil {
		return err
	}
	f.digitPlan = plan
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
	// The pieces of a scientific or engineering exponent.
	PartExponentSeparator PartKind = "exponentSeparator"
	PartExponentMinusSign PartKind = "exponentMinusSign"
	PartExponentInteger   PartKind = "exponentInteger"
	// PartCompact is the word or letter compact notation writes for a
	// magnitude: "K", "million".
	PartCompact PartKind = "compact"
	// PartUnit is a measurement's name or symbol: "km", "kilometers".
	PartUnit PartKind = "unit"
	// PartApproximatelySign marks a range whose ends are written alike.
	PartApproximatelySign PartKind = "approximatelySign"

	// The pieces of a date or a time, named as ECMA-402 names them.
	PartEra  PartKind = "era"
	PartYear PartKind = "year"
	// PartYearName is the name of a year in the Chinese calendars' cycle,
	// "jia-chen", and PartRelatedYear the Gregorian year a year starts in.
	PartYearName         PartKind = "yearName"
	PartRelatedYear      PartKind = "relatedYear"
	PartMonth            PartKind = "month"
	PartDay              PartKind = "day"
	PartWeekday          PartKind = "weekday"
	PartDayPeriod        PartKind = "dayPeriod"
	PartHour             PartKind = "hour"
	PartMinute           PartKind = "minute"
	PartSecond           PartKind = "second"
	PartFractionalSecond PartKind = "fractionalSecond"
	PartTimeZoneName     PartKind = "timeZoneName"
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
	return f.FormatDecimalToParts(DecimalFromFloat(v))
}

// FormatDecimal writes a number given exactly: a string or a BigInt, as
// ECMA-402's formatters take them, with as many digits as it has.
func (f *NumberFormat) FormatDecimal(d Decimal) string {
	var b strings.Builder
	for _, p := range f.FormatDecimalToParts(d) {
		b.WriteString(p.Value)
	}
	return b.String()
}

// FormatDecimalToParts writes a number given exactly as the pieces it is
// made of.
func (f *NumberFormat) FormatDecimalToParts(d Decimal) []Part {
	return mergeParts(trimParts(f.assemble(f.layers(d, false))))
}

// numberLayers is a number written, in the layers ICU's number formatter
// builds it from: the digits (body); the exponent of scientific notation
// (inner); the pattern's affixes with the sign, the currency, the percent
// sign and compact notation's word (middle, pre and post); and the unit or
// currency name wrapped round the lot (outer). A range compares its two ends
// layer by layer and writes a layer both share once.
type numberLayers struct {
	pre, body, inner, post []Part
	// outer says whether a unit or currency name is wrapped round the
	// number, and count is the plural category it is chosen by.
	outer bool
	count string
	// rounded is the number as written, sign and digits, which decides
	// whether a range's two ends are the same.
	rounded string
}

// layers writes a number in its layers. Approximately puts the approximately
// sign where the sign goes, as ICU does for a range whose ends are written
// alike.
func (f *NumberFormat) layers(d Decimal, approximately bool) numberLayers {
	negative := d.neg
	magnitude := d.m
	if magnitude.integer == "" {
		magnitude.integer = "0"
	}
	if f.opts.Style == StylePercent {
		magnitude = magnitude.shift(2)
	}

	var l numberLayers
	// Whether the number is zero is asked of it as written: 0.0001 at two
	// decimals is "0", which signDisplay "exceptZero" writes without a sign.
	zero := d.kind == decimalNaN || d.kind == decimalFinite && f.roundsToZero(magnitude, negative)
	sign, showSign := f.signFor(zero, negative)

	// PatternStringUtils::patternInfoToStringBuilder: the sign goes where
	// the pattern's negative form puts its minus -- the plus and the
	// approximately sign too, when there is such a form -- and otherwise
	// before everything. A number shown without a sign is written with the
	// positive form, whatever its sign.
	minusShown := showSign && sign.kind == PartMinusSign
	plusShown := showSign && sign.kind == PartPlusSign
	var symbols []Part
	if approximately {
		symbols = append(symbols, Part{PartApproximatelySign, f.approximatelySign()})
	}
	switch {
	case plusShown:
		symbols = append(symbols, Part{PartPlusSign, f.data.Symbols.PlusSign})
	case minusShown || !approximately:
		symbols = append(symbols, Part{PartMinusSign, f.data.Symbols.MinusSign})
	}
	negativeHasMinus := f.pattern.explicitNeg &&
		strings.ContainsRune(f.pattern.negPrefix+f.pattern.negSuffix, '-')
	useNegative := f.pattern.explicitNeg &&
		(minusShown || negativeHasMinus && (plusShown || approximately))
	prefix, suffix := f.pattern.posPrefix, f.pattern.posSuffix
	if useNegative {
		prefix, suffix = f.pattern.negPrefix, f.pattern.negSuffix
	}
	// A compact pattern with a negative form of its own places the sign
	// likewise, in place of the pattern's.
	var form compactForm
	compactNegative := false
	if d.kind == decimalFinite && f.opts.Notation == NotationCompact {
		form = f.compactForm(magnitude, negative)
		compactNegative = form.hasNeg && (minusShown ||
			strings.ContainsRune(form.negPrefix+form.negSuffix, '-') && (plusShown || approximately))
	}
	if !useNegative && !compactNegative && (minusShown || plusShown || approximately) {
		l.pre = append(l.pre, symbols...)
	}
	l.pre = append(l.pre, f.affixParts(prefix, symbols)...)

	switch {
	case d.kind == decimalNaN:
		l.body = []Part{{PartNaN, f.data.Symbols.NaN}}
	case d.kind == decimalInfinite:
		l.body = []Part{{PartInfinity, f.data.Symbols.Infinity}}
	case f.opts.Notation == NotationCompact:
		pre, body, post := f.compactPieces(form, magnitude, negative, compactNegative, symbols)
		l.pre = append(l.pre, pre...)
		l.body = body
		l.post = post
	case f.opts.Notation == NotationScientific, f.opts.Notation == NotationEngineering:
		l.body, l.inner = f.scientificPieces(magnitude, negative)
	default:
		l.body = f.numberParts(magnitude, negative)
	}
	l.post = append(l.post, f.affixParts(suffix, symbols)...)

	// NaN and the infinities are "other" in every language, as ICU's plural
	// rules answer for them.
	finite := d.kind == decimalFinite
	if f.opts.Style == StyleCurrency && f.opts.CurrencyDisplay == CurrencyName || f.opts.Style == StyleUnit && !f.percentUnit {
		l.outer = true
		l.count = f.outerCount(magnitude, finite)
	}
	var b strings.Builder
	if negative {
		b.WriteByte('-')
	}
	for _, p := range l.body {
		b.WriteString(p.Value)
	}
	for _, p := range l.inner {
		b.WriteString(p.Value)
	}
	l.rounded = b.String()
	return l
}

// approximatelySign is the locale's, or ICU's default.
func (f *NumberFormat) approximatelySign() string {
	if s := f.data.Symbols.ApproximatelySign; s != "" {
		return s
	}
	return "~"
}

// assemble puts a number's layers together.
func (f *NumberFormat) assemble(l numberLayers) []Part {
	parts := make([]Part, 0, len(l.pre)+len(l.body)+len(l.inner)+len(l.post))
	parts = append(parts, l.pre...)
	parts = append(parts, l.body...)
	parts = append(parts, l.inner...)
	parts = append(parts, l.post...)
	if l.outer {
		return f.wrapOuter(parts, l.count)
	}
	return f.spaceCurrency(parts)
}

// outerCount is the plural category a unit or currency name is chosen by.
// It comes from the digits the formatter writes, not from the value: money
// is written with two decimals, so one dollar is "1.00", which English calls
// "dollars" rather than "dollar".
func (f *NumberFormat) outerCount(magnitude mag, finite bool) string {
	if f.plurals == nil || !finite {
		return string(PluralOther)
	}
	integer, fraction := f.round(magnitude, false)
	o := operandsFor(padInteger(integer, f.minInt), fraction, 0)
	return string(f.plurals.selectOperands(&o))
}

// wrapOuter wraps a number in its unit or currency name.
func (f *NumberFormat) wrapOuter(parts []Part, count string) []Part {
	if f.opts.Style == StyleUnit {
		return f.applyUnit(parts, count)
	}
	return f.joinCurrencyName(parts, count)
}

// joinCurrencyName puts the amount and the spelled-out name together, by the
// locale's unit pattern and the plural category of the amount.
func (f *NumberFormat) joinCurrencyName(parts []Part, count string) []Part {

	name := strings.ToUpper(f.opts.Currency)
	if c, ok := f.data.Currency(name); ok {
		if text := c.Name(count); text != "" {
			name = text
		}
	}
	pattern := "{0} {1}"
	var other string
	for _, u := range f.data.UnitPatterns {
		if u.Count == count {
			other = u.Text
			break
		}
		if u.Count == "other" {
			other = u.Text
		}
	}
	if other != "" {
		pattern = other
	}

	var out []Part
	rest := pattern
	for {
		at := strings.IndexByte(rest, '{')
		if at < 0 || at+2 >= len(rest) || rest[at+2] != '}' {
			break
		}
		if at > 0 {
			out = append(out, Part{PartLiteral, rest[:at]})
		}
		switch rest[at+1] {
		case '0':
			out = append(out, parts...)
		case '1':
			out = append(out, Part{PartCurrency, name})
		}
		rest = rest[at+3:]
	}
	if rest != "" {
		out = append(out, Part{PartLiteral, rest})
	}
	return out
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
		insert, ok := f.currencySpace(parts[i], parts[i+1])
		if !ok {
			continue
		}
		parts = append(parts, Part{})
		copy(parts[i+2:], parts[i+1:])
		parts[i+1] = Part{PartLiteral, insert}
		i++
	}
	return parts
}

// currencySpace is the space to put between two adjacent parts, where one
// is the currency and the other the number and CLDR's rule says so.
func (f *NumberFormat) currencySpace(left, right Part) (string, bool) {
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
		return "", false
	}
	if !ok || !rule.applies(currencyFacing, numberFacing) {
		return "", false
	}
	return rule.insert, true
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

// roundsToZero reports whether a magnitude is written as zero. A number in
// scientific notation never is unless it is zero, and one large enough to
// compact is not.
func (f *NumberFormat) roundsToZero(m mag, negative bool) bool {
	if m.isZero() {
		return true
	}
	switch f.opts.Notation {
	case NotationScientific, NotationEngineering:
		return false
	}
	integer, fraction := f.round(m, negative)
	return strings.Trim(integer, "0") == "" && strings.Trim(fraction, "0") == ""
}

// signFor decides whether a sign is written and which one.
func (f *NumberFormat) signFor(zero, negative bool) (signPart, bool) {
	minus := signPart{PartMinusSign, f.data.Symbols.MinusSign}
	plus := signPart{PartPlusSign, f.data.Symbols.PlusSign}

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
func (f *NumberFormat) numberParts(magnitude mag, negative bool) []Part {
	integer, fraction := f.round(magnitude, negative)
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
		at = groupPositions(len(integer), f.primaryGroup, f.secondaryGroup, f.minGrouping)
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
func (f *NumberFormat) affixParts(affix string, signSymbols []Part) []Part {
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
			kind := PartPercentSign
			if f.percentUnit {
				kind = PartUnit
			}
			parts = append(parts, Part{kind, f.data.Symbols.PercentSign})
		case '-':
			// The pattern's sign, which is whatever the number shows there.
			flush()
			parts = append(parts, signSymbols...)
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
	Locale          string
	NumberingSystem string
	Style           Style
	// Currency, CurrencyDisplay and CurrencySign are meaningful only in the
	// currency style, and Unit and UnitDisplay in the unit style, the only
	// ones resolvedOptions reports them for.
	Currency        string
	CurrencyDisplay CurrencyDisplay
	CurrencySign    CurrencySign
	Unit            string
	UnitDisplay     UnitDisplay
	ResolvedDigits
	// UseGrouping is the grouping settled on: GroupingMin2 for a compact
	// number that asked for none.
	UseGrouping Grouping
	Notation    Notation
	// CompactDisplay is meaningful only in compact notation, which is the
	// only one resolvedOptions reports it for.
	CompactDisplay CompactDisplay
	SignDisplay    SignDisplay
}

// ResolvedOptions returns what the formatter settled on.
func (f *NumberFormat) ResolvedOptions() ResolvedNumberFormat {
	r := ResolvedNumberFormat{
		Locale:          f.locale.String(),
		NumberingSystem: f.data.NumberingSystem,
		Style:           f.opts.Style,
		Currency:        strings.ToUpper(f.opts.Currency),
		CurrencyDisplay: f.opts.CurrencyDisplay,
		CurrencySign:    f.opts.CurrencySign,
		Unit:            f.opts.Unit,
		UnitDisplay:     f.opts.UnitDisplay,
		ResolvedDigits:  f.digitPlan.resolved(),
		UseGrouping:     f.opts.UseGrouping,
		Notation:        f.opts.Notation,
		CompactDisplay:  f.opts.CompactDisplay,
		SignDisplay:     f.opts.SignDisplay,
	}
	if r.UseGrouping == GroupingAuto && r.Notation == NotationCompact {
		r.UseGrouping = GroupingMin2
	}
	return r
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

// CurrencyDigits is ECMA-402's CurrencyDigits: how many decimals a currency
// is written with by default, from ISO 4217 as CLDR has it, two for a code it
// does not list.
func CurrencyDigits(src Source, code string) (int, error) { return currencyDigits(src, code) }
