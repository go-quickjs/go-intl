// Package unitdata is the model layer for measurements: how a locale writes a
// quantity of something.
//
// "987 km/h" is the same in German and "987公里/小時" in Chinese; "1 hour" is
// "1 Stunde" and "2 hours" is "2 Stunden". None of that follows from the name
// of the unit, so all of it is carried -- as patterns, one per plural
// category, and never as a formatted measurement.
package unitdata

import (
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 2

// The widths, in the order they are stored.
const (
	Long = iota
	Short
	Narrow
	Widths
)

// A CountedText is a wording and the plural category it belongs to.
type CountedText struct {
	Count string
	Text  string
}

// A Unit is how one measurement is written.
type Unit struct {
	// Name is ECMA-402's name for it, "meter", without CLDR's category.
	Name string
	// Patterns place the amount, "{0} meters", one per plural category.
	Patterns []CountedText
	// PerUnit writes the unit as a divisor, "{0}/m". Many units have none, in
	// which case the compound pattern joins them instead.
	PerUnit string
}

// Pattern returns the wording for a plural category, falling back to "other".
func (u Unit) Pattern(count string) string {
	var other string
	for _, p := range u.Patterns {
		if p.Count == count {
			return p.Text
		}
		if p.Count == "other" {
			other = p.Text
		}
	}
	return other
}

// A Width is everything a locale says at one level of abbreviation.
type Width struct {
	// Units is sorted by name.
	Units []Unit
	// Compound joins a unit to the one it is divided by, "{0} per {1}", for
	// the pairs CLDR has no wording of its own for.
	Compound string
}

// Unit finds one measurement.
func (w *Width) Unit(name string) (Unit, bool) {
	i := sort.Search(len(w.Units), func(i int) bool { return w.Units[i].Name >= name })
	if i < len(w.Units) && w.Units[i].Name == name {
		return w.Units[i], true
	}
	return Unit{}, false
}

// Locale holds every width.
type Locale struct {
	Widths [Widths]Width
}

// Width returns one level of abbreviation, falling back to the wider one where
// a locale gives none, which is what CLDR's own inheritance does.
func (l *Locale) Width(w int) *Width {
	for i := w; i > Long; i-- {
		if i < Widths && len(l.Widths[i].Units) > 0 {
			return &l.Widths[i]
		}
	}
	return &l.Widths[Long]
}

// Encode writes a locale's units, with what it shares with other locales --
// every string, each unit and each list -- in pool, which the generator
// writes beside the locales and Decode is given.
func Encode(l *Locale, pool *blob.Pool) []byte {
	b := blob.NewPooledWriter(Version, pool)
	for _, w := range l.Widths {
		b.SharedString(w.Compound)
		b.Shared(func(b *blob.Writer) {
			b.Uint(len(w.Units))
			for _, u := range w.Units {
				b.Shared(func(b *blob.Writer) {
					b.SharedString(u.Name)
					b.SharedString(u.PerUnit)
					b.Uint(len(u.Patterns))
					for _, p := range u.Patterns {
						b.SharedString(p.Count)
						b.SharedString(p.Text)
					}
				})
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
	for i := range l.Widths {
		w := &l.Widths[i]
		w.Compound = r.SharedString()
		r.Shared(func(r *blob.Reader) {
			n := r.Uint()
			if n < 0 || n > r.Left() {
				return
			}
			w.Units = make([]Unit, n)
			for j := range w.Units {
				u := &w.Units[j]
				r.Shared(func(r *blob.Reader) {
					u.Name = r.SharedString()
					u.PerUnit = r.SharedString()
					k := r.Uint()
					if k < 0 || k > r.Left() {
						return
					}
					u.Patterns = make([]CountedText, k)
					for m := range u.Patterns {
						u.Patterns[m] = CountedText{Count: r.SharedString(), Text: r.SharedString()}
					}
				})
			}
		})
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
