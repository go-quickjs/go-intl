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
const Version = 2

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

// Encode writes a locale's names, with what it shares with other locales --
// every string and each set -- in pool, which the generator writes beside
// the locales and Decode is given.
func Encode(l *Locale, pool *blob.Pool) []byte {
	b := blob.NewPooledWriter(Version, pool)
	b.SharedString(l.Pattern)
	b.SharedString(l.Separator)
	for _, s := range l.Sets {
		b.Shared(func(b *blob.Writer) {
			b.Uint(len(s.Entries))
			for _, e := range s.Entries {
				b.SharedString(e.Code)
				b.SharedString(e.Name)
			}
		})
	}
	return b.Bytes()
}

// Decode reads what Encode wrote, with the pool it wrote into.
func Decode(data []byte, pool blob.Shared) (*Locale, error) {
	r, err := blob.NewPooledReader(data, Version, pool)
	if err != nil {
		return nil, err
	}
	var l Locale
	l.Pattern = r.SharedString()
	l.Separator = r.SharedString()
	for i := range l.Sets {
		r.Shared(func(r *blob.Reader) {
			n := r.Uint()
			if n < 0 || n > r.Left() {
				return
			}
			entries := make([]Entry, n)
			for j := range entries {
				entries[j] = Entry{Code: r.SharedString(), Name: r.SharedString()}
			}
			l.Sets[i].Entries = entries
		})
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
