package intl

import (
	"fmt"
	"strings"

	"github.com/go-quickjs/go-intl/internal/namedata"
	"github.com/go-quickjs/go-intl/internal/numdata"
)

// Intl.DisplayNames.
//
// What a locale calls a language, a region, a script, a currency, a calendar
// or a part of a date. This is the largest data set here by a wide margin, and
// the reason that is not a problem is the Source it arrives through: a program
// that never asks what French is called in Japanese never reads a byte of it.

// DisplayKind is what sort of thing is being named.
type DisplayKind int

const (
	DisplayLanguage DisplayKind = iota
	DisplayRegion
	DisplayScript
	DisplayCurrency
	DisplayCalendar
	DisplayDateTimeField
)

// ParseDisplayKind reads a kind as ECMA-402 names it.
func ParseDisplayKind(name string) (DisplayKind, bool) {
	switch name {
	case "language":
		return DisplayLanguage, true
	case "region":
		return DisplayRegion, true
	case "script":
		return DisplayScript, true
	case "currency":
		return DisplayCurrency, true
	case "calendar":
		return DisplayCalendar, true
	case "dateTimeField":
		return DisplayDateTimeField, true
	}
	return 0, false
}

// DisplayStyle is how much room a name takes.
type DisplayStyle int

const (
	DisplayLong DisplayStyle = iota
	DisplayShort
	DisplayNarrow
)

// DisplayFallback is what to answer when a code has no name.
type DisplayFallback int

const (
	// FallbackCode answers with the code itself, and is the default.
	FallbackCode DisplayFallback = iota
	// FallbackNone answers with nothing.
	FallbackNone
)

// LanguageDisplay is how a language with a region or script is named.
type LanguageDisplay int

const (
	// LanguageDialect uses the name the locale has for the whole thing where
	// it has one -- "British English" -- and is the default.
	LanguageDialect LanguageDisplay = iota
	// LanguageStandard always builds the name from its parts: "English
	// (United Kingdom)".
	LanguageStandard
)

// DisplayNamesOptions mirrors the option bag of Intl.DisplayNames. Its zero
// value is ECMA-402's default in every field except Kind, which has no
// default: ECMA-402 requires it.
type DisplayNamesOptions struct {
	Kind            DisplayKind
	Style           DisplayStyle
	Fallback        DisplayFallback
	LanguageDisplay LanguageDisplay
}

// A DisplayNames answers what things are called in one locale. It never
// changes after it is built and is safe for any number of goroutines to share.
type DisplayNames struct {
	locale   Locale
	opts     DisplayNamesOptions
	data     *namedata.Locale
	width    int
	currency *numdata.Locale
}

// NewDisplayNames builds one from the data built into the package.
func NewDisplayNames(loc Locale, opts DisplayNamesOptions) (*DisplayNames, error) {
	return NewDisplayNamesFrom(Embedded, loc, opts)
}

// NewDisplayNamesFrom builds one from a source of the caller's own.
func NewDisplayNamesFrom(src Source, loc Locale, opts DisplayNamesOptions) (*DisplayNames, error) {
	// ICU keeps language, script and region names in trees of their own,
	// and the redirects differ between them only a little.
	tree := treeLang
	if opts.Kind == DisplayRegion {
		tree = treeRegion
	}
	data, err := loadNames(src, loc, tree)
	if err != nil {
		return nil, err
	}
	// DisplayNames uses nothing of the Unicode extension.
	d := &DisplayNames{locale: loc.onlyKeywords(), opts: opts, data: data}
	switch opts.Style {
	case DisplayShort:
		d.width = namedata.Short
	case DisplayNarrow:
		d.width = namedata.Narrow
	default:
		d.width = namedata.Long
	}
	if opts.Kind == DisplayCurrency {
		// A currency's name lives with the number data, since that is where it
		// is also used to write an amount out in words.
		if d.currency, err = loadNumbers(src, loc); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func loadNames(src Source, loc Locale, tree string) (*namedata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.ChainIn(tree, loc.Data())
	}
	pool, err := openShared(src, MarkerNamesShared, namedata.Version)
	if err != nil {
		return nil, err
	}
	for _, d := range chain {
		b, err := src.Open(MarkerNames, d)
		if err != nil {
			continue
		}
		l, err := namedata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the display names for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no display names for %s: %w", loc, ErrNotFound)
}

// Of returns what the locale calls a code.
//
// The second result says whether a name was found. When none was, the first is
// the code itself or empty, by the fallback option.
func (d *DisplayNames) Of(code string) (string, bool) {
	name, ok := d.lookup(code)
	if ok {
		return name, true
	}
	if d.opts.Fallback == FallbackNone {
		return "", false
	}
	return code, false
}

func (d *DisplayNames) lookup(code string) (string, bool) {
	switch d.opts.Kind {
	case DisplayCurrency:
		if d.currency == nil {
			return "", false
		}
		c, ok := d.currency.Currency(strings.ToUpper(code))
		if !ok {
			return "", false
		}
		// The plain display name, not one of the counted wordings: "US Dollar"
		// rather than "US dollars", which is what goes beside an amount.
		if c.DisplayName != "" {
			return c.DisplayName, true
		}
		if name := c.Name("other"); name != "" {
			return name, true
		}
		return "", false
	case DisplayLanguage:
		return d.language(code)
	case DisplayRegion:
		return d.data.Lookup(namedata.Region, d.width, code)
	case DisplayScript:
		return d.data.Lookup(namedata.Script, d.width, code)
	case DisplayCalendar:
		return d.data.Lookup(namedata.Calendar, d.width, code)
	case DisplayDateTimeField:
		return d.data.Lookup(namedata.DateTimeField, d.width, code)
	}
	return "", false
}

// language names a language, which is the only kind with a shape of its own: a
// language qualified by a script or a region may have a name for the whole
// thing, and where it does not the parts are put together.
func (d *DisplayNames) language(code string) (string, bool) {
	loc, err := ParseLocale(code)
	if err != nil {
		return "", false
	}
	canonical := loc.Data().String()

	if d.opts.LanguageDisplay == LanguageDialect {
		if name, ok := d.data.Lookup(namedata.Language, d.width, canonical); ok {
			return name, true
		}
	}

	base, ok := d.data.Lookup(namedata.Language, d.width, loc.Language.String())
	if !ok {
		// Standard display still falls back to a whole-name match rather than
		// answering nothing.
		return d.data.Lookup(namedata.Language, d.width, canonical)
	}
	var qualifiers []string
	if !loc.Script.IsZero() {
		if name, ok := d.data.Lookup(namedata.Script, d.width, loc.Script.String()); ok {
			qualifiers = append(qualifiers, name)
		}
	}
	if !loc.Region.IsZero() {
		if name, ok := d.data.Lookup(namedata.Region, d.width, loc.Region.String()); ok {
			qualifiers = append(qualifiers, name)
		}
	}
	if len(qualifiers) == 0 {
		return base, true
	}

	separator := d.data.Separator
	if separator == "" {
		separator = "{0}, {1}"
	}
	joined := qualifiers[0]
	for _, q := range qualifiers[1:] {
		joined = fill2(separator, joined, q)
	}
	pattern := d.data.Pattern
	if pattern == "" {
		pattern = "{0} ({1})"
	}
	return fill2(pattern, base, joined), true
}

// fill2 substitutes a two-placeholder pattern.
func fill2(pattern, first, second string) string {
	out := strings.ReplaceAll(pattern, "{0}", first)
	return strings.ReplaceAll(out, "{1}", second)
}

// ResolvedDisplayNames is what a DisplayNames settled on.
type ResolvedDisplayNames struct {
	Locale          string
	Kind            DisplayKind
	Style           DisplayStyle
	Fallback        DisplayFallback
	LanguageDisplay LanguageDisplay
}

// ResolvedOptions returns what it settled on.
func (d *DisplayNames) ResolvedOptions() ResolvedDisplayNames {
	return ResolvedDisplayNames{
		Locale:          d.locale.String(),
		Kind:            d.opts.Kind,
		Style:           d.opts.Style,
		Fallback:        d.opts.Fallback,
		LanguageDisplay: d.opts.LanguageDisplay,
	}
}
