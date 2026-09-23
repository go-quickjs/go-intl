package intl

import (
	"fmt"
	"strings"
	"unicode"
)

// The small part of UnicodeSet that CLDR's currency spacing needs.
//
// The spacing rules match with expressions like "[[:^S:]&[:^Z:]]" and
// "[:digit:]". A general UnicodeSet is a language of its own and nothing here
// needs one, so this reads the few forms CLDR actually writes and refuses
// anything else.
//
// Refusing matters more than covering. A matcher that quietly said no to an
// expression it did not understand would put the spaces in the wrong places in
// some locales and nowhere else, which is the kind of wrong that is found
// years later. This fails when the formatter is built, naming the expression.

type runeSet func(rune) bool

// parseRuneSet reads one of the set expressions CLDR uses.
func parseRuneSet(src string) (runeSet, error) {
	s := strings.TrimSpace(src)
	if s == "" {
		return func(rune) bool { return false }, nil
	}
	// An intersection of two sets: [[:^S:]&[:^Z:]].
	if strings.HasPrefix(s, "[[") && strings.HasSuffix(s, "]]") {
		inner := s[1 : len(s)-1]
		parts := strings.Split(inner, "&")
		if len(parts) < 2 {
			return nil, fmt.Errorf("the set %q is not an intersection", src)
		}
		sets := make([]runeSet, 0, len(parts))
		for _, p := range parts {
			set, err := parsePosixClass(strings.TrimSpace(p), src)
			if err != nil {
				return nil, err
			}
			sets = append(sets, set)
		}
		return func(r rune) bool {
			for _, in := range sets {
				if !in(r) {
					return false
				}
			}
			return true
		}, nil
	}
	return parsePosixClass(s, src)
}

// parsePosixClass reads [:name:] and [:^name:].
func parsePosixClass(s, whole string) (runeSet, error) {
	if !strings.HasPrefix(s, "[:") || !strings.HasSuffix(s, ":]") {
		return nil, fmt.Errorf("the set %q has a part this does not read: %q", whole, s)
	}
	name := s[2 : len(s)-2]
	negated := strings.HasPrefix(name, "^")
	name = strings.TrimPrefix(name, "^")

	var table *unicode.RangeTable
	switch name {
	case "S":
		table = unicode.S
	case "Z":
		table = unicode.Z
	case "L":
		table = unicode.L
	case "N":
		table = unicode.N
	case "digit", "Nd":
		table = unicode.Nd
	default:
		return nil, fmt.Errorf("the set %q names a class this does not know: %q",
			whole, name)
	}
	if negated {
		return func(r rune) bool { return !unicode.Is(table, r) }, nil
	}
	return func(r rune) bool { return unicode.Is(table, r) }, nil
}

// spacingRule is one of CLDR's currency-spacing rules, compiled.
type spacingRule struct {
	currency    runeSet
	surrounding runeSet
	insert      string
}

func compileSpacing(currencyMatch, surroundingMatch, insert string) (*spacingRule, error) {
	if insert == "" {
		return nil, nil
	}
	cur, err := parseRuneSet(currencyMatch)
	if err != nil {
		return nil, err
	}
	sur, err := parseRuneSet(surroundingMatch)
	if err != nil {
		return nil, err
	}
	return &spacingRule{currency: cur, surrounding: sur, insert: insert}, nil
}

// applies reports whether a space belongs between the two characters facing
// each other across the join.
func (r *spacingRule) applies(currencyFacing, numberFacing rune) bool {
	if r == nil {
		return false
	}
	return r.currency(currencyFacing) && r.surrounding(numberFacing)
}

func firstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}

func lastRune(s string) (rune, bool) {
	var last rune
	var ok bool
	for _, r := range s {
		last, ok = r, true
	}
	return last, ok
}
