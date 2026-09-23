// Package namedata is the model layer for display names: what a locale calls a
// language, a region, a script, a currency, a calendar or a part of a date.
//
// This is the largest data set in the library by a wide margin -- seven
// hundred languages and three hundred regions in each of seven hundred
// locales -- and the reason it is not a problem is the Source it arrives
// through. A program that never asks what French is called in Japanese never
// reads a byte of it.
package namedata

import (
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// The kinds of name, in the order they are stored.
const (
	Language = iota
	Region
	Script
	Calendar
	DateTimeField
	Kinds
)

// The widths, in the order they are stored.
const (
	Long = iota
	Short
	Narrow
	Widths
)

// An Entry is one code and what the locale calls it.
type Entry struct {
	Code string
	Name string
}

// A Set is every name of one kind at one width, sorted by code.
type Set struct {
	Entries []Entry
}

// Name finds one.
func (s *Set) Name(code string) (string, bool) {
	i := sort.Search(len(s.Entries), func(i int) bool { return s.Entries[i].Code >= code })
	if i < len(s.Entries) && s.Entries[i].Code == code {
		return s.Entries[i].Name, s.Entries[i].Name != ""
	}
	return "", false
}

// Locale holds every kind at every width, plus the patterns that join a
// language to the region or script it is qualified by.
type Locale struct {
	Sets [Kinds * Widths]Set

	// Pattern puts a qualifier beside a language, "{0} ({1})", and Separator
	// joins two qualifiers, "{0}, {1}".
	Pattern   string
	Separator string
}

// Set returns one kind at one width, falling back to the wider forms, which is
// what CLDR's own inheritance does: a locale gives a short name only where it
// differs from the long one.
func (l *Locale) Set(kind, width int) *Set {
	if kind < 0 || kind >= Kinds {
		kind = Language
	}
	for w := width; w > Long; w-- {
		if s := &l.Sets[kind*Widths+w]; len(s.Entries) > 0 {
			return s
		}
	}
	return &l.Sets[kind*Widths+Long]
}

// Lookup finds a name at a width, falling back through the narrower forms to
// the long one, entry by entry rather than set by set.
func (l *Locale) Lookup(kind, width int, code string) (string, bool) {
	for w := width; w >= Long; w-- {
		if name, ok := l.Sets[kind*Widths+w].Name(code); ok {
			return name, true
		}
	}
	return "", false
}

// Encode writes a locale's names.
func Encode(l *Locale) []byte {
	b := blob.NewWriter(Version)
	b.String(l.Pattern)
	b.String(l.Separator)
	for _, s := range l.Sets {
		b.Uint(len(s.Entries))
		for _, e := range s.Entries {
			b.String(e.Code)
			b.String(e.Name)
		}
	}
	return b.Bytes()
}

// Decode reads what Encode wrote.
func Decode(data []byte) (*Locale, error) {
	r, err := blob.NewReader(data, Version)
	if err != nil {
		return nil, err
	}
	var l Locale
	l.Pattern = r.String()
	l.Separator = r.String()
	for i := range l.Sets {
		n := r.Uint()
		if n < 0 || n > r.Left() {
			break
		}
		entries := make([]Entry, 0, n)
		for j := 0; j < n; j++ {
			code := r.String()
			name := r.String()
			entries = append(entries, Entry{code, name})
		}
		l.Sets[i].Entries = entries
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
