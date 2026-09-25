// Command segmentgen writes what the intl package's Segmenter reads, as ICU
// 78.3 has it, which is what Node's Intl.Segmenter runs.
//
//	go run ./internal/segmentgen <icu4c-78.3-sources.tgz> <icu4c-78.3-data.zip> <ucd-17.0.0 dir>
//
// ICU finds boundaries with its rules compiled into state tables, and
// divides runs of Thai, Lao, Khmer, Burmese, Chinese and Japanese with
// dictionaries. Both are in the ICU data Node carries, icudt78l.dat, which
// ICU's source release ships prebuilt (source/data/in); segmentgen copies
// them out of it as they stand, the way zoneinfo64 and the collation export
// are taken: the rules and word lists as ICU compiles them.
//
//	data/brkitr/char.bin, word.bin, word_POSIX.bin, sent.bin, sent_el.bin
//	data/brkitr/thaidict.bin, laodict.bin, khmerdict.bin,
//	    burmesedict.bin, cjdict.bin
//
// Each is ICU's item of that name, .brk or .dict, without ICU's data
// header: the RBBIDataHeader, or the dictionary's indexes, first.
//
// data/brkitr/boundaries.bin has which rules each bundle of ICU's brkitr
// tree names for the Segmenter's three granularities, and which dictionary
// the root names for each script, from the data archive's brkitr/*.txt:
//
//	boundaries root grapheme char.brk
//	boundaries en_US_POSIX word word_POSIX.brk
//	dictionary Thai thaidict.dict
//
// data/brkitr/sets.bin has the Unicode sets ICU's break engines build from
// property patterns, and the Script property they choose an engine by,
// from the Unicode Character Database 17.0.0 (Scripts.txt, LineBreak.txt,
// extracted/DerivedGeneralCategory.txt), as ranges of hexadecimal code
// points:
//
//	set thai 0E01-0E3A 0E40-0E4E       [[:Thai:]&[:LineBreak=SA:]]
//	set thaimarks 0E31 0E34-0E3A ...   [[:Thai:]&[:LineBreak=SA:]&[:M:]]
//	set cj 3041-3096 ...               [[:Han:][:Hiragana:][:Katakana:]] and U+30FC U+FF70 U+FF9E U+FF9F
//	script Thai 0E01-0E3A 0E40-0E5B
package main

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icudat"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// ucdSHA256 are the Unicode Character Database 17.0.0's files read.
var ucdSHA256 = map[string]string{
	"Scripts.txt":                "9f5e50d3abaee7d6ce09480f325c706f485ae3240912527e651954d2d6b035bf",
	"LineBreak.txt":              "e6a18fa91f8f6a6f8e534b1d3f128c21ada45bfe152eb6b1bcc5e15fd8ac92e6",
	"DerivedGeneralCategory.txt": "d62e5bab70ca74f099343f71224fa051cb1fdd61a1ab45c0488c44cfc0b6102e",
}

// items are the compiled rules and dictionaries copied from icudt78l.dat.
var items = []string{
	"char.brk", "word.brk", "word_POSIX.brk", "sent.brk", "sent_el.brk",
	"thaidict.dict", "laodict.dict", "khmerdict.dict", "burmesedict.dict", "cjdict.dict",
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/segmentgen <icu4c-78.3-sources.tgz> <icu4c-78.3-data.zip> <ucd-17.0.0 dir>")
		os.Exit(2)
	}
	files, err := build(os.Args[1], os.Args[2], os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, "segmentgen:", err)
		os.Exit(1)
	}
	if err := write(files); err != nil {
		fmt.Fprintln(os.Stderr, "segmentgen:", err)
		os.Exit(1)
	}
}

func build(sources, dataZip, ucdDir string) (map[string][]byte, error) {
	dat, err := icudat.Read(sources)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, name := range items {
		b, ok := dat["icudt78l/brkitr/"+name]
		if !ok {
			return nil, fmt.Errorf("icudt78l.dat has no brkitr/%s", name)
		}
		body, err := icudat.StripHeader(b)
		if err != nil {
			return nil, fmt.Errorf("brkitr/%s: %w", name, err)
		}
		files[strings.TrimSuffix(name, filepath.Ext(name))+".bin"] = body
	}
	boundaries, err := readBoundaries(dataZip)
	if err != nil {
		return nil, err
	}
	files["boundaries.bin"] = boundaries
	sets, err := buildSets(ucdDir)
	if err != nil {
		return nil, err
	}
	files["sets.bin"] = sets
	return files, nil
}

// readBoundaries reads which rules each brkitr bundle names, and which
// dictionary the root names for each script.
func readBoundaries(dataZip string) ([]byte, error) {
	z, err := icusrc.Open(dataZip, icusrc.DataSHA256)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	var lines []string
	for _, f := range z.File {
		dir, name := filepath.ToSlash(filepath.Dir(f.Name)), filepath.Base(f.Name)
		if dir != "data/brkitr" || !strings.HasSuffix(name, ".txt") {
			continue
		}
		b, err := readZip(f)
		if err != nil {
			return nil, err
		}
		n, err := icutxt.Parse(strings.TrimPrefix(string(b), string(rune(0xfeff))))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		bundle := strings.TrimSuffix(name, ".txt")
		if b := n.Get("boundaries"); b != nil {
			for _, c := range b.Children {
				kind, _, _ := strings.Cut(c.Key, ":")
				switch kind {
				case "grapheme", "word", "sentence":
					lines = append(lines, fmt.Sprintf("boundaries %s %s %s", bundle, kind, c.Value))
				}
			}
		}
		if bundle == "root" {
			if d := n.Get("dictionaries"); d != nil {
				for _, c := range d.Children {
					script, _, _ := strings.Cut(c.Key, ":")
					lines = append(lines, fmt.Sprintf("dictionary %s %s", script, c.Value))
				}
			}
		}
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("the data archive has no brkitr boundaries")
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func readZip(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// A ucdProperty is a property's values by code point.
type ucdProperty map[rune]string

// readUCD reads a property file of "range ; value" lines.
func readUCD(dir, name string) (ucdProperty, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != ucdSHA256[name] {
		return nil, fmt.Errorf("%s has checksum %s, want %s", name, got, ucdSHA256[name])
	}
	p := ucdProperty{}
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
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%s: %q", name, line)
		}
		for r := rune(a); r <= rune(z); r++ {
			p[r] = value
		}
	}
	return p, nil
}

// The long names Scripts.txt writes of the scripts ICU's patterns name.
const (
	thai     = "Thai"
	lao      = "Lao"
	khmer    = "Khmer"
	myanmar  = "Myanmar"
	han      = "Han"
	hiragana = "Hiragana"
	katakana = "Katakana"
)

func buildSets(dir string) ([]byte, error) {
	scripts, err := readUCD(dir, "Scripts.txt")
	if err != nil {
		return nil, err
	}
	lb, err := readUCD(dir, "LineBreak.txt")
	if err != nil {
		return nil, err
	}
	gc, err := readUCD(dir, "DerivedGeneralCategory.txt")
	if err != nil {
		return nil, err
	}
	mark := func(r rune) bool { v := gc[r]; return v == "Mn" || v == "Mc" || v == "Me" }
	var out strings.Builder
	set := func(name string, in func(r rune) bool) {
		out.WriteString("set " + name + ranges(in) + "\n")
	}
	for _, s := range []struct{ name, script string }{
		{"thai", thai}, {"lao", lao}, {"khmer", khmer}, {"myanmar", myanmar},
	} {
		script := s.script
		set(s.name, func(r rune) bool { return scripts[r] == script && lb[r] == "SA" })
		set(s.name+"marks", func(r rune) bool { return scripts[r] == script && lb[r] == "SA" && mark(r) })
	}
	set("cj", func(r rune) bool {
		switch scripts[r] {
		case han, hiragana, katakana:
			return true
		}
		return r == 0x30fc || r == 0xff70 || r == 0xff9e || r == 0xff9f
	})
	// The Script property, which chooses a break engine and which ICU's
	// engine for scripts without one skips by.
	names := map[string]bool{}
	for _, v := range scripts {
		names[v] = true
	}
	var sorted []string
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		out.WriteString("script " + n + ranges(func(r rune) bool { return scripts[r] == n }) + "\n")
	}
	return []byte(out.String()), nil
}

// ranges writes the code points a predicate holds for as " lo-hi" ranges.
func ranges(in func(r rune) bool) string {
	var b strings.Builder
	for r := rune(0); r <= 0x10ffff; r++ {
		if !in(r) {
			continue
		}
		lo := r
		for r+1 <= 0x10ffff && in(r+1) {
			r++
		}
		if lo == r {
			fmt.Fprintf(&b, " %04X", lo)
		} else {
			fmt.Fprintf(&b, " %04X-%04X", lo, r)
		}
	}
	return b.String()
}

// write replaces data/brkitr, all built before any is written.
func write(files map[string][]byte) error {
	tmp := filepath.Join("data", "brkitr.tmp")
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), b, 0o644); err != nil {
			return err
		}
	}
	final := filepath.Join("data", "brkitr")
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}
