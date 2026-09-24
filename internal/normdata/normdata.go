// Package normdata is the model layer for Unicode normalization: how a
// character comes apart and goes back together.
//
// Two strings can spell the same text in more than one way. An e with an acute
// accent is one character or two, and a run of accents can be written in more
// than one order. Normalizing settles both questions, which is what lets text
// from different places be compared -- and the collator sorts on the
// decomposed form, so everything it does rests on this.
//
// What is stored is the Unicode Character Database's own mappings. Hangul is
// not among them: its syllables come apart by arithmetic, which is smaller
// than the table would be.
package normdata

import (
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// A Decomposition is one character and what it comes apart into.
type Decomposition struct {
	Rune rune
	To   string
}

// A Combining is one character and its canonical combining class, which says
// how accents order among themselves. Only the characters with a class other
// than zero are stored.
type Combining struct {
	Rune  rune
	Class uint8
}

// A Composition is a pair of characters and the one they make together. It is
// Unicode's canonical mapping read backwards, keyed for looking up.
type Composition struct {
	First, Second, To rune
}

// Tables are everything normalization needs.
type Tables struct {
	// Canonical and Compatibility are sorted by rune. Canonical mappings are
	// stored fully worked out, so nothing has to be applied twice.
	Canonical     []Decomposition
	Compatibility []Decomposition
	// Classes is sorted by rune.
	Classes []Combining
	// Excluded are the characters that decompose but must not be put back
	// together, sorted.
	Excluded []rune
	// Compositions are the pairs that make one character, sorted.
	//
	// They come from the mappings as the database writes them, which are at
	// most two characters long, rather than from the worked-out ones: an
	// s-with-two-dots is written as an s-with-one-dot and the other dot, and
	// working that out into three characters loses the pair.
	Compositions []Composition
}

// Compose returns what two characters make together.
func (t *Tables) Compose(first, second rune) (rune, bool) {
	i := sort.Search(len(t.Compositions), func(i int) bool {
		c := t.Compositions[i]
		return c.First > first || (c.First == first && c.Second >= second)
	})
	if i < len(t.Compositions) &&
		t.Compositions[i].First == first && t.Compositions[i].Second == second {
		return t.Compositions[i].To, true
	}
	return 0, false
}

// Decompose finds a character's canonical decomposition.
func (t *Tables) Decompose(r rune) (string, bool) {
	return find(t.Canonical, r)
}

// DecomposeCompatibility finds a character's compatibility decomposition,
// which is the canonical one where it has no other.
func (t *Tables) DecomposeCompatibility(r rune) (string, bool) {
	if s, ok := find(t.Compatibility, r); ok {
		return s, true
	}
	return find(t.Canonical, r)
}

func find(set []Decomposition, r rune) (string, bool) {
	i := sort.Search(len(set), func(i int) bool { return set[i].Rune >= r })
	if i < len(set) && set[i].Rune == r {
		return set[i].To, true
	}
	return "", false
}

// Class returns a character's canonical combining class.
func (t *Tables) Class(r rune) uint8 {
	i := sort.Search(len(t.Classes), func(i int) bool { return t.Classes[i].Rune >= r })
	if i < len(t.Classes) && t.Classes[i].Rune == r {
		return t.Classes[i].Class
	}
	return 0
}

// IsExcluded reports whether a character must not be composed back together.
func (t *Tables) IsExcluded(r rune) bool {
	i := sort.Search(len(t.Excluded), func(i int) bool { return t.Excluded[i] >= r })
	return i < len(t.Excluded) && t.Excluded[i] == r
}

// Encode writes the tables.
func Encode(t *Tables) []byte {
	b := blob.NewWriter(Version)
	for _, set := range [][]Decomposition{t.Canonical, t.Compatibility} {
		b.Uint(len(set))
		for _, d := range set {
			b.Uint(int(d.Rune))
			b.String(d.To)
		}
	}
	b.Uint(len(t.Classes))
	for _, c := range t.Classes {
		b.Uint(int(c.Rune))
		b.Uint(int(c.Class))
	}
	b.Uint(len(t.Excluded))
	for _, r := range t.Excluded {
		b.Uint(int(r))
	}
	b.Uint(len(t.Compositions))
	for _, c := range t.Compositions {
		b.Uint(int(c.First))
		b.Uint(int(c.Second))
		b.Uint(int(c.To))
	}
	return b.Bytes()
}

// Decode reads what Encode wrote.
func Decode(data []byte) (*Tables, error) {
	r, err := blob.NewReader(data, Version)
	if err != nil {
		return nil, err
	}
	var t Tables
	for _, set := range []*[]Decomposition{&t.Canonical, &t.Compatibility} {
		n := r.Uint()
		if n < 0 || n > r.Left() {
			break
		}
		out := make([]Decomposition, 0, n)
		for i := 0; i < n; i++ {
			code := rune(r.Uint())
			to := r.String()
			out = append(out, Decomposition{Rune: code, To: to})
		}
		*set = out
	}
	if n := r.Uint(); n >= 0 && n <= r.Left() {
		t.Classes = make([]Combining, 0, n)
		for i := 0; i < n; i++ {
			code := rune(r.Uint())
			class := uint8(r.Uint())
			t.Classes = append(t.Classes, Combining{Rune: code, Class: class})
		}
	}
	if n := r.Uint(); n >= 0 && n <= r.Left() {
		t.Excluded = make([]rune, 0, n)
		for i := 0; i < n; i++ {
			t.Excluded = append(t.Excluded, rune(r.Uint()))
		}
	}
	if n := r.Uint(); n >= 0 && n <= r.Left() {
		t.Compositions = make([]Composition, 0, n)
		for i := 0; i < n; i++ {
			first := rune(r.Uint())
			second := rune(r.Uint())
			to := rune(r.Uint())
			t.Compositions = append(t.Compositions, Composition{first, second, to})
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &t, nil
}
