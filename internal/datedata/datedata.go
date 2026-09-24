// Package datedata is the model layer for date and time formatting: the names
// a locale writes dates with and the patterns it writes them in.
//
// Everything here is what CLDR says, kept in CLDR's own shape. The patterns
// stay patterns -- "EEEE, MMMM d, y" -- and are taken apart when a formatter
// is built. A field holding a formatted date would mean the layering had been
// violated.
package datedata

import (
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// The widths a name may be written at, in the order they are stored.
const (
	Wide = iota
	Abbreviated
	Narrow
	// Short is a fourth width that only weekday names have.
	Short
	Widths
)

// The two contexts a name may appear in. A month inside a date may take a
// different form from one standing on its own, which is what the distinction
// is for: "1 de enero" against "enero".
const (
	Format = iota
	StandAlone
	Contexts
)

// The lengths a whole date or time may be written at.
const (
	Full = iota
	Long
	Medium
	ShortLength
	Lengths
)

// LengthNames are CLDR's names for the lengths, in storage order.
var LengthNames = [Lengths]string{"full", "long", "medium", "short"}

// A Skeleton is one of CLDR's available formats: a request for a set of
// fields, and the pattern that answers it.
type Skeleton struct {
	// ID is the skeleton as CLDR writes it, "yMMMd".
	ID string
	// Pattern is what it resolves to, "MMM d, y".
	Pattern string
}

// Names holds one set of names, indexed by the thing they name: months by
// number less one, weekdays by day of the week with Sunday first.
type Names struct {
	Text []string
}

// Calendar is everything a locale says about writing dates in one calendar.
type Calendar struct {
	// Months and Days are indexed [context*Widths + width].
	Months [Contexts * Widths]Names
	Days   [Contexts * Widths]Names
	// DayPeriods are indexed by width and hold the two halves of the day.
	AM [Widths]string
	PM [Widths]string
	// Eras are indexed by width.
	Eras [Widths]Names

	// DateFormats, TimeFormats and DateTimeFormats are indexed by length.
	// DateTimeFormats is the glue that puts a date and a time together,
	// "{1}, {0}", where {1} is the date.
	DateFormats     [Lengths]string
	TimeFormats     [Lengths]string
	DateTimeFormats [Lengths]string
	// AtTimeFormats is the glue used when a whole date and a whole time were
	// both asked for by length. It differs from the plain one: English joins
	// them with "at" there and with a comma otherwise.
	AtTimeFormats [Lengths]string

	// Available are the skeletons the locale answers, sorted by id.
	Available []Skeleton
}

// Month returns a month's name, counting from one, falling back from a width
// the locale does not give to the wide one.
func (c *Calendar) Month(context, width, month int) string {
	return pick(c.Months[:], context, width, month-1)
}

// Day returns a weekday's name, with Sunday as zero.
func (c *Calendar) Day(context, width, day int) string {
	return pick(c.Days[:], context, width, day)
}

// Era returns an era's name: zero is before the epoch, one after it.
func (c *Calendar) Era(width, era int) string {
	for w := width; w >= Wide; w-- {
		if names := c.Eras[w]; era >= 0 && era < len(names.Text) && names.Text[era] != "" {
			return names.Text[era]
		}
		if w == Wide {
			break
		}
	}
	return ""
}

func pick(sets []Names, context, width, index int) string {
	for _, ctx := range [...]int{context, Format} {
		for w := width; w >= Wide; w-- {
			names := sets[ctx*Widths+w]
			if index >= 0 && index < len(names.Text) && names.Text[index] != "" {
				return names.Text[index]
			}
			if w == Wide {
				break
			}
		}
	}
	return ""
}

// Skeleton finds the pattern for an exact skeleton.
func (c *Calendar) Skeleton(id string) (string, bool) {
	i := sort.Search(len(c.Available), func(i int) bool { return c.Available[i].ID >= id })
	if i < len(c.Available) && c.Available[i].ID == id {
		return c.Available[i].Pattern, true
	}
	return "", false
}

// Locale holds the calendars a locale has data for. Only the Gregorian one is
// carried so far; the field is keyed so the rest can arrive without the
// encoding changing shape.
type Locale struct {
	Calendars []NamedCalendar
}

// A NamedCalendar is one calendar and which one it is, "gregory".
type NamedCalendar struct {
	Name     string
	Calendar Calendar
}

// Calendar finds one by name.
func (l *Locale) Calendar(name string) (*Calendar, bool) {
	for i := range l.Calendars {
		if l.Calendars[i].Name == name {
			return &l.Calendars[i].Calendar, true
		}
	}
	return nil, false
}

// Encode writes a locale's calendars.
func Encode(l *Locale) []byte {
	b := blob.NewWriter(Version)
	b.Uint(len(l.Calendars))
	for i := range l.Calendars {
		b.String(l.Calendars[i].Name)
		encodeCalendar(b, &l.Calendars[i].Calendar)
	}
	return b.Bytes()
}

func encodeCalendar(b *blob.Writer, c *Calendar) {
	for _, sets := range [][]Names{c.Months[:], c.Days[:]} {
		for _, n := range sets {
			b.Uint(len(n.Text))
			for _, s := range n.Text {
				b.String(s)
			}
		}
	}
	for w := 0; w < Widths; w++ {
		b.String(c.AM[w])
		b.String(c.PM[w])
	}
	for _, n := range c.Eras {
		b.Uint(len(n.Text))
		for _, s := range n.Text {
			b.String(s)
		}
	}
	for _, set := range [][Lengths]string{c.DateFormats, c.TimeFormats,
		c.DateTimeFormats, c.AtTimeFormats} {
		for _, s := range set {
			b.String(s)
		}
	}
	b.Uint(len(c.Available))
	for _, s := range c.Available {
		b.String(s.ID)
		b.String(s.Pattern)
	}
}

// Decode reads what Encode wrote.
func Decode(data []byte) (*Locale, error) {
	r, err := blob.NewReader(data, Version)
	if err != nil {
		return nil, err
	}
	var l Locale
	n := r.Uint()
	if n < 0 || n > r.Left() {
		n = 0
	}
	for i := 0; i < n; i++ {
		name := r.String()
		var c Calendar
		decodeCalendar(r, &c)
		l.Calendars = append(l.Calendars, NamedCalendar{Name: name, Calendar: c})
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}

func decodeCalendar(r *blob.Reader, c *Calendar) {
	for _, sets := range []*[Contexts * Widths]Names{&c.Months, &c.Days} {
		for i := range sets {
			n := r.Uint()
			if n < 0 || n > r.Left() {
				return
			}
			text := make([]string, 0, n)
			for j := 0; j < n; j++ {
				text = append(text, r.String())
			}
			sets[i].Text = text
		}
	}
	for w := 0; w < Widths; w++ {
		c.AM[w] = r.String()
		c.PM[w] = r.String()
	}
	for i := range c.Eras {
		n := r.Uint()
		if n < 0 || n > r.Left() {
			return
		}
		text := make([]string, 0, n)
		for j := 0; j < n; j++ {
			text = append(text, r.String())
		}
		c.Eras[i].Text = text
	}
	for _, set := range []*[Lengths]string{&c.DateFormats, &c.TimeFormats,
		&c.DateTimeFormats, &c.AtTimeFormats} {
		for i := range set {
			set[i] = r.String()
		}
	}
	n := r.Uint()
	if n < 0 || n > r.Left() {
		return
	}
	c.Available = make([]Skeleton, 0, n)
	for i := 0; i < n; i++ {
		id := r.String()
		pattern := r.String()
		c.Available = append(c.Available, Skeleton{ID: id, Pattern: pattern})
	}
}
