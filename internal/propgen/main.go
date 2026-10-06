// Command propgen writes the Unicode properties formatting reads, from the
// Unicode Character Database 17.0.0, the version ICU 78.3 and Node carry:
//
//	go run ./internal/propgen <ucd-17.0.0 dir>
//
// The directory holds extracted/DerivedGeneralCategory.txt, PropList.txt and
// Scripts.txt, as regen lays them out, each checked against the checksum
// SOURCES.md pins. Go's unicode package would answer the same questions in
// whatever Unicode the toolchain building the program carries -- 15.0 in
// Go 1.24 -- so a character Unicode added since, a Garay digit, had no
// category, and en-u-nu-gara lost the space before a currency.
//
// data/properties.bin is an index (blob.Index), read where it lies, of the
// sets, each its ranges of code points, two little-endian uint32s apiece,
// the first and the last:
//
//	gc S, gc Z, gc L, gc N, gc Nd, gc Zs   general categories, for the
//	                                       currency spacing's sets and
//	                                       ICU's default ignorables
//	Bidi_Control, Variation_Selector       the rest of the ignorables
//	sc Hebrew                              the Script property's Hebrew,
//	                                       which Hebrew lists test
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

// ucdSHA256 are the files read, as SOURCES.md pins them.
var ucdSHA256 = map[string]string{
	"DerivedGeneralCategory.txt": "d62e5bab70ca74f099343f71224fa051cb1fdd61a1ab45c0488c44cfc0b6102e",
	"PropList.txt":               "130dcddcaadaf071008bdfce1e7743e04fdfbc910886f017d9f9ac931d8c64dd",
	"Scripts.txt":                "9f5e50d3abaee7d6ce09480f325c706f485ae3240912527e651954d2d6b035bf",
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/propgen <ucd-17.0.0 dir>")
		os.Exit(2)
	}
	out, err := build(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "propgen:", err)
		os.Exit(1)
	}
	if err := writeset.WriteFile(filepath.Join("data", "properties.bin"), out); err != nil {
		fmt.Fprintln(os.Stderr, "propgen:", err)
		os.Exit(1)
	}
}

// An entry is one line of a UCD file: a range and its value.
type entry struct {
	lo, hi rune
	value  string
}

func build(dir string) ([]byte, error) {
	gc, err := readUCD(dir, "DerivedGeneralCategory.txt")
	if err != nil {
		return nil, err
	}
	props, err := readUCD(dir, "PropList.txt")
	if err != nil {
		return nil, err
	}
	scripts, err := readUCD(dir, "Scripts.txt")
	if err != nil {
		return nil, err
	}
	// A general category's major class is its first letter.
	category := func(in func(string) bool) [][2]rune { return collect(gc, in) }
	major := func(c byte) func(string) bool { return func(v string) bool { return v[0] == c } }
	is := func(want string) func(string) bool { return func(v string) bool { return v == want } }
	sets := map[string][][2]rune{
		"gc S":               category(major('S')),
		"gc Z":               category(major('Z')),
		"gc L":               category(major('L')),
		"gc N":               category(major('N')),
		"gc Nd":              category(is("Nd")),
		"gc Zs":              category(is("Zs")),
		"Bidi_Control":       collect(props, is("Bidi_Control")),
		"Variation_Selector": collect(props, is("Variation_Selector")),
		"sc Hebrew":          collect(scripts, is("Hebrew")),
	}
	records := map[string][]byte{}
	for name, set := range sets {
		if len(set) == 0 {
			return nil, fmt.Errorf("the set %s is empty", name)
		}
		var b []byte
		for _, r := range set {
			b = binary.LittleEndian.AppendUint32(b, uint32(r[0]))
			b = binary.LittleEndian.AppendUint32(b, uint32(r[1]))
		}
		records[name] = b
	}
	return blob.BuildIndex(records)
}

// collect is the ranges of the entries whose value is in, sorted, with
// adjacent ones joined.
func collect(entries []entry, in func(string) bool) [][2]rune {
	var out [][2]rune
	for _, e := range entries {
		if in(e.value) {
			out = append(out, [2]rune{e.lo, e.hi})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	joined := out[:0]
	for _, r := range out {
		if n := len(joined); n > 0 && r[0] <= joined[n-1][1]+1 {
			joined[n-1][1] = max(joined[n-1][1], r[1])
			continue
		}
		joined = append(joined, r)
	}
	return joined
}

// readUCD reads a UCD file's lines, "0041..005A ; Lu # ...".
func readUCD(dir, name string) ([]entry, error) {
	path := filepath.Join(dir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != ucdSHA256[name] {
		return nil, fmt.Errorf("%s has checksum %s, want %s", path, got, ucdSHA256[name])
	}
	var out []entry
	for _, line := range strings.Split(string(b), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		codes, value, ok := strings.Cut(line, ";")
		if !ok {
			continue
		}
		codes, value = strings.TrimSpace(codes), strings.TrimSpace(value)
		lo, hi, _ := strings.Cut(codes, "..")
		if hi == "" {
			hi = lo
		}
		a, err1 := strconv.ParseUint(lo, 16, 32)
		z, err2 := strconv.ParseUint(hi, 16, 32)
		if err1 != nil || err2 != nil || a > z || z > 0x10ffff || value == "" {
			return nil, fmt.Errorf("%s: %q", name, line)
		}
		out = append(out, entry{rune(a), rune(z), value})
	}
	return out, nil
}
