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
const Version = 1

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

// Encode writes a locale's units.
func Encode(l *Locale) []byte {
	b := blob.NewWriter(Version)
	for _, w := range l.Widths {
		b.String(w.Compound)
		b.Uint(len(w.Units))
		for _, u := range w.Units {
			b.String(u.Name)
			b.String(u.PerUnit)
			b.Uint(len(u.Patterns))
			for _, p := range u.Patterns {
				b.String(p.Count)
				b.String(p.Text)
			}
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
	for i := range l.Widths {
		w := &l.Widths[i]
		w.Compound = r.String()
		n := r.Uint()
		if n < 0 || n > r.Left() {
			break
		}
		w.Units = make([]Unit, 0, n)
		for j := 0; j < n; j++ {
			var u Unit
			u.Name = r.String()
			u.PerUnit = r.String()
			patterns := r.Uint()
			if patterns < 0 || patterns > r.Left() {
				break
			}
			u.Patterns = make([]CountedText, 0, patterns)
			for k := 0; k < patterns; k++ {
				count := r.String()
				text := r.String()
				u.Patterns = append(u.Patterns, CountedText{count, text})
			}
			w.Units = append(w.Units, u)
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
