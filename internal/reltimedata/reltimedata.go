// Package reltimedata is the model layer for relative times: how a locale says
// that something happened a while ago or will happen in a while.
//
// A language says this two ways. It has patterns that take a number -- "in {0}
// days", "{0} days ago" -- one per plural category, and it has wordings for
// the offsets it has a name for: yesterday, today, tomorrow. Both are stored,
// and which is used is the caller's choice.
package reltimedata

import (
	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// The units, in the order they are stored. These are the ones ECMA-402 allows.
const (
	Year = iota
	Quarter
	Month
	Week
	Day
	Hour
	Minute
	Second
	Units
)

// Names are the units as ECMA-402 writes them, in storage order.
var Names = [Units]string{
	"year", "quarter", "month", "week", "day", "hour", "minute", "second",
}

// The widths, in the order they are stored.
const (
	Long = iota
	Short
	Narrow
	Widths
)

// WidthNames are CLDR's suffixes for the widths, in storage order. The long
// one has no suffix.
var WidthNames = [Widths]string{"", "-short", "-narrow"}

// A CountedText is a wording and the plural category it belongs to.
type CountedText struct {
	Count string
	Text  string
}

// A Named is a wording for one offset: -1 is "yesterday".
type Named struct {
	Offset int
	Text   string
}

// A Field is everything a locale says about one unit at one width.
type Field struct {
	// Named are the offsets the locale has a word for, sorted by offset. Most
	// units have -1, 0 and 1; a second usually has only 0, "now".
	Named []Named
	// Future and Past place a count, "in {0} days" and "{0} days ago".
	Future []CountedText
	Past   []CountedText
}

// Word returns the locale's wording for an offset, if it has one.
func (f *Field) Word(offset int) (string, bool) {
	for _, n := range f.Named {
		if n.Offset == offset {
			return n.Text, n.Text != ""
		}
	}
	return "", false
}

// Pattern returns the counted pattern for a direction and plural category.
func (f *Field) Pattern(future bool, count string) string {
	set := f.Past
	if future {
		set = f.Future
	}
	var other string
	for _, p := range set {
		if p.Count == count {
			return p.Text
		}
		if p.Count == "other" {
			other = p.Text
		}
	}
	return other
}

// Locale holds every unit at every width.
type Locale struct {
	Fields [Units * Widths]Field
}

// Field returns one unit at one width, falling back to the wider forms where a
// locale gives none, which is what CLDR's own inheritance does.
func (l *Locale) Field(unit, width int) *Field {
	if unit < 0 || unit >= Units {
		unit = Year
	}
	for w := width; w > Long; w-- {
		if f := &l.Fields[unit*Widths+w]; len(f.Future) > 0 || len(f.Named) > 0 {
			return f
		}
	}
	return &l.Fields[unit*Widths+Long]
}

// Encode writes a locale's fields.
func Encode(l *Locale) []byte {
	b := blob.NewWriter(Version)
	for _, f := range l.Fields {
		b.Uint(len(f.Named))
		for _, n := range f.Named {
			// The offsets run from -1 upwards in practice, and the writer
			// takes no negative numbers, so they are shifted.
			b.Uint(n.Offset + 8)
			b.String(n.Text)
		}
		for _, set := range [][]CountedText{f.Future, f.Past} {
			b.Uint(len(set))
			for _, p := range set {
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
	for i := range l.Fields {
		f := &l.Fields[i]
		n := r.Uint()
		if n < 0 || n > r.Left() {
			break
		}
		f.Named = make([]Named, 0, n)
		for j := 0; j < n; j++ {
			offset := r.Uint() - 8
			text := r.String()
			f.Named = append(f.Named, Named{offset, text})
		}
		for _, set := range []*[]CountedText{&f.Future, &f.Past} {
			m := r.Uint()
			if m < 0 || m > r.Left() {
				break
			}
			out := make([]CountedText, 0, m)
			for j := 0; j < m; j++ {
				count := r.String()
				text := r.String()
				out = append(out, CountedText{count, text})
			}
			*set = out
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
