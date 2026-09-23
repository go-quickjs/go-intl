package intl

import (
	"fmt"
	"strings"

	"github.com/go-quickjs/go-intl/internal/listdata"
)

// Intl.ListFormat.
//
// Joining a list is not a matter of putting a separator between the items.
// English writes "a and b" for two and "a, b, and c" for three, which is not
// the same join repeated, and other languages differ again. CLDR gives four
// patterns for each kind of list and this applies them.

// ListType is what kind of list is being written.
type ListType int

const (
	// Conjunction is "a, b, and c", and the default.
	Conjunction ListType = iota
	// Disjunction is "a, b, or c".
	Disjunction
	// UnitList is "a, b, c", for a run of measurements.
	UnitList
)

// ListStyle is how much room the joins take.
type ListStyle int

const (
	// ListLong is the default.
	ListLong ListStyle = iota
	ListShort
	ListNarrow
)

// ListFormatOptions mirrors the option bag of Intl.ListFormat. Its zero value
// is ECMA-402's default in every field.
type ListFormatOptions struct {
	Type  ListType
	Style ListStyle
}

// A ListFormat joins lists in one locale. It never changes after it is built
// and is safe for any number of goroutines to share.
type ListFormat struct {
	locale   Locale
	opts     ListFormatOptions
	patterns listdata.Patterns
}

// NewListFormat builds a formatter from the data built into the package.
func NewListFormat(loc Locale, opts ListFormatOptions) (*ListFormat, error) {
	return NewListFormatFrom(Embedded, loc, opts)
}

// NewListFormatFrom builds a formatter from a source of the caller's own.
func NewListFormatFrom(src Source, loc Locale, opts ListFormatOptions) (*ListFormat, error) {
	data, err := loadLists(src, loc)
	if err != nil {
		return nil, err
	}
	kind := listdata.Standard
	switch opts.Type {
	case Disjunction:
		kind = listdata.Or
	case UnitList:
		kind = listdata.Unit
	}
	width := listdata.Long
	switch opts.Style {
	case ListShort:
		width = listdata.Short
	case ListNarrow:
		width = listdata.Narrow
	}
	p := data.Get(kind, width)
	if p.Empty() {
		return nil, fmt.Errorf("intl: %s has no list patterns: %w", loc, ErrNotFound)
	}
	return &ListFormat{locale: loc, opts: opts, patterns: p}, nil
}

func loadLists(src Source, loc Locale) (*listdata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.Chain(loc.Data())
	}
	for _, d := range chain {
		b, err := src.Open(MarkerLists, d)
		if err != nil {
			continue
		}
		l, err := listdata.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the list patterns for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no list patterns for %s: %w", loc, ErrNotFound)
}

// Format joins a list.
func (f *ListFormat) Format(items []string) string {
	var b strings.Builder
	for _, p := range f.FormatToParts(items) {
		b.WriteString(p.Value)
	}
	return b.String()
}

// ListPartKind says whether a piece of a joined list is one of the items or
// the text between them.
type ListPartKind string

const (
	ListElement ListPartKind = "element"
	ListLiteral ListPartKind = "literal"
)

// A ListPart is one piece of a joined list.
type ListPart struct {
	Kind  ListPartKind
	Value string
}

// FormatToParts joins a list and says which pieces are the items.
//
// The list is built from the end: the last two items are joined with the end
// pattern, each earlier item is folded in with the middle pattern, and the
// first with the start pattern. That is what makes "a, b, and c" rather than
// one separator repeated.
func (f *ListFormat) FormatToParts(items []string) []ListPart {
	switch len(items) {
	case 0:
		return nil
	case 1:
		return []ListPart{{ListElement, items[0]}}
	case 2:
		return applyListPattern(f.patterns.Two,
			[]ListPart{{ListElement, items[0]}},
			[]ListPart{{ListElement, items[1]}})
	}

	parts := []ListPart{{ListElement, items[len(items)-1]}}
	for i := len(items) - 2; i >= 1; i-- {
		p := f.patterns.Middle
		if i == len(items)-2 {
			p = f.patterns.End
		}
		parts = applyListPattern(p, []ListPart{{ListElement, items[i]}}, parts)
	}
	return applyListPattern(f.patterns.Start,
		[]ListPart{{ListElement, items[0]}}, parts)
}

// applyListPattern fills a pattern's {0} and {1}, keeping what came from the
// items apart from what came from the pattern.
func applyListPattern(pattern string, first, second []ListPart) []ListPart {
	var out []ListPart
	literal := func(s string) {
		if s != "" {
			out = append(out, ListPart{ListLiteral, s})
		}
	}
	rest := pattern
	for {
		at := strings.IndexByte(rest, '{')
		if at < 0 || at+2 >= len(rest) || rest[at+2] != '}' {
			break
		}
		literal(rest[:at])
		switch rest[at+1] {
		case '0':
			out = append(out, first...)
		case '1':
			out = append(out, second...)
		default:
			literal(rest[at : at+3])
		}
		rest = rest[at+3:]
	}
	literal(rest)
	return out
}

// ResolvedListFormat is what a ListFormat settled on.
type ResolvedListFormat struct {
	Locale string
	Type   ListType
	Style  ListStyle
}

// ResolvedOptions returns what the formatter settled on.
func (f *ListFormat) ResolvedOptions() ResolvedListFormat {
	return ResolvedListFormat{
		Locale: f.locale.String(),
		Type:   f.opts.Type,
		Style:  f.opts.Style,
	}
}
