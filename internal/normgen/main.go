// Command normgen writes the normalization tables the intl package carries.
//
// It reads the Unicode Character Database, vendored beside it: UnicodeData.txt
// for the decompositions and combining classes, and CompositionExclusions.txt
// for the characters that come apart but must not be put back together.
//
//	go run ./internal/normgen
//
// The database is the primary source. ICU's own normalizer tables are exported
// in icuexportdata, but in ICU4X's trie encoding, which would have to be
// implemented to read; the database they are built from is line-based text and
// says the same thing.
//
// Canonical decompositions are worked out fully here rather than at run time:
// a mapping may name a character that itself decomposes, and doing it once in
// the generator is cheaper than doing it on every string.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/normdata"
)

//go:embed UnicodeData.txt
var unicodeData string

//go:embed CompositionExclusions.txt
var compositionExclusions string

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "normgen:", err)
		os.Exit(1)
	}
}

func run() error {
	canonical := map[rune]string{}
	compatibility := map[rune]string{}
	classes := map[rune]uint8{}

	for line := range strings.SplitSeq(unicodeData, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), ";")
		if len(fields) < 6 {
			continue
		}
		code, err := strconv.ParseUint(fields[0], 16, 32)
		if err != nil {
			continue
		}
		r := rune(code)

		if class, err := strconv.Atoi(fields[3]); err == nil && class != 0 {
			classes[r] = uint8(class)
		}

		mapping := strings.TrimSpace(fields[5])
		if mapping == "" {
			continue
		}
		// A mapping that opens with a tag in angle brackets is a
		// compatibility one; the rest are canonical.
		compat := strings.HasPrefix(mapping, "<")
		if compat {
			if at := strings.IndexByte(mapping, '>'); at >= 0 {
				mapping = strings.TrimSpace(mapping[at+1:])
			}
		}
		var to []rune
		for _, part := range strings.Fields(mapping) {
			v, err := strconv.ParseUint(part, 16, 32)
			if err != nil {
				return fmt.Errorf("U+%04X maps to %q, which is not characters", r, part)
			}
			to = append(to, rune(v))
		}
		if len(to) == 0 {
			continue
		}
		if compat {
			compatibility[r] = string(to)
		} else {
			canonical[r] = string(to)
		}
	}
	if len(canonical) == 0 {
		return fmt.Errorf("UnicodeData.txt has no canonical decompositions")
	}

	// Worked out fully: a mapping may name a character that decomposes in
	// turn, and this is the one place to follow it.
	full := make(map[rune]string, len(canonical))
	for r := range canonical {
		full[r] = expand(r, canonical, nil)
	}
	// Every character that decomposes either way needs a compatibility form,
	// not only those with a compatibility mapping of their own: the long s
	// with a dot above decomposes canonically to a long s, and it is the long
	// s that has the compatibility mapping to a plain one.
	fullCompat := make(map[rune]string, len(compatibility)+len(canonical))
	for r := range compatibility {
		fullCompat[r] = expandCompat(r, canonical, compatibility, nil)
	}
	for r := range canonical {
		if _, seen := fullCompat[r]; !seen {
			fullCompat[r] = expandCompat(r, canonical, compatibility, nil)
		}
	}

	excluded := map[rune]bool{}
	for line := range strings.SplitSeq(compositionExclusions, "\n") {
		if at := strings.IndexByte(line, '#'); at >= 0 {
			line = line[:at]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		v, err := strconv.ParseUint(line, 16, 32)
		if err != nil {
			continue
		}
		excluded[rune(v)] = true
	}
	// A character whose canonical decomposition is a single character, or
	// whose first character has a combining class, is excluded by the standard
	// itself rather than by the list.
	//
	// This is judged on the mapping as the database writes it, not on the
	// worked-out one. The angstrom sign maps to an A-with-ring, which is a
	// single character and so excluded; working it out first turns that into
	// two characters and loses the reason.
	for r, to := range canonical {
		runes := []rune(to)
		if len(runes) == 1 || classes[runes[0]] != 0 {
			excluded[r] = true
		}
	}

	var tables normdata.Built
	tables.Canonical = sortedDecompositions(full)
	tables.Compatibility = sortedDecompositions(fullCompat)
	for r, class := range classes {
		tables.Classes = append(tables.Classes, normdata.Combining{Rune: r, Class: class})
	}
	sort.Slice(tables.Classes, func(i, j int) bool {
		return tables.Classes[i].Rune < tables.Classes[j].Rune
	})
	for r := range excluded {
		tables.Excluded = append(tables.Excluded, r)
	}
	sort.Slice(tables.Excluded, func(i, j int) bool {
		return tables.Excluded[i] < tables.Excluded[j]
	})

	// The pairs come from the mappings as written, which are at most two
	// characters, rather than from the worked-out ones.
	for r, to := range canonical {
		if excluded[r] {
			continue
		}
		runes := []rune(to)
		if len(runes) != 2 {
			continue
		}
		tables.Compositions = append(tables.Compositions, normdata.Composition{
			First: runes[0], Second: runes[1], To: r,
		})
	}
	sort.Slice(tables.Compositions, func(i, j int) bool {
		a, b := tables.Compositions[i], tables.Compositions[j]
		if a.First != b.First {
			return a.First < b.First
		}
		return a.Second < b.Second
	})

	out, err := normdata.Encode(&tables)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "normalization.bin"), out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr,
		"normgen: %d canonical, %d compatibility, %d classes, %d excluded, %.0f KB\n",
		len(tables.Canonical), len(tables.Compatibility), len(tables.Classes),
		len(tables.Excluded), float64(len(out))/1024)
	return nil
}

// expand follows a canonical mapping until nothing decomposes further.
func expand(r rune, canonical map[rune]string, seen []rune) string {
	to, ok := canonical[r]
	if !ok {
		return string(r)
	}
	for _, s := range seen {
		if s == r {
			// A mapping that leads back to itself would not terminate. The
			// database has none, and this is here so that a future one is a
			// wrong answer rather than a hang.
			return string(r)
		}
	}
	seen = append(seen, r)
	var b strings.Builder
	for _, c := range to {
		b.WriteString(expand(c, canonical, seen))
	}
	return b.String()
}

// expandCompat does the same for a compatibility mapping, which may name
// characters with either kind.
func expandCompat(r rune, canonical, compatibility map[rune]string, seen []rune) string {
	to, ok := compatibility[r]
	if !ok {
		if to, ok = canonical[r]; !ok {
			return string(r)
		}
	}
	for _, s := range seen {
		if s == r {
			return string(r)
		}
	}
	seen = append(seen, r)
	var b strings.Builder
	for _, c := range to {
		b.WriteString(expandCompat(c, canonical, compatibility, seen))
	}
	return b.String()
}

func sortedDecompositions(in map[rune]string) []normdata.Decomposition {
	out := make([]normdata.Decomposition, 0, len(in))
	for r, to := range in {
		if to == string(r) {
			continue
		}
		out = append(out, normdata.Decomposition{Rune: r, To: to})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rune < out[j].Rune })
	return out
}
