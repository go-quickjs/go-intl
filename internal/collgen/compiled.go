package main

import (
	"encoding/binary"
	"fmt"

	"github.com/go-quickjs/go-intl/internal/colldata"
	"github.com/go-quickjs/go-intl/internal/icudat"
)

// The tailorings taken from ICU's compiled data rather than the export.
//
// The export leaves the conjoining Hangul jamo, U+1100 to U+11FF, out of
// every tailoring's trie and ships only the root's jamo, because ICU4X
// reckons with those alone. A tailoring that gives jamo elements of their
// own loses them: the search collations make a trailing consonant weigh as
// the leading one, and Korean's "searchjl" gives the leading consonants
// prefix contexts and weights of their own. Node's ICU has them all.
//
// Those weights are allocated by ICU's rule compiler, so they are taken as
// it compiled them: the collation type's %%CollationBin in icudt78l.dat,
// whole -- its trie, turned into the code point trie the collator reads,
// and the arrays the trie points into, which the export renumbers and so
// cannot be mixed with. The type's settings, script reordering and
// diacritics still come from the export. The layout is ICU's, from
// collationdatareader.h.

// The indexes of a compiled collation's parts.
const (
	ixIndexesLength = 0
	ixTrieOffset    = 7
	ixCEsOffset     = 9
	ixCE32sOffset   = 11
	ixContexts      = 13
)

const fallbackCE32 = 0xc0

// compiledCollations reads a locale's collation types from ICU's compiled
// data, keeping those whose trie tailors a conjoining jamo.
func compiledCollations(dat icudat.Dat, name string) (map[string]*colldata.Data, error) {
	item, ok := dat["icudt78l/coll/"+name+".res"]
	if !ok {
		return nil, nil
	}
	bundle, err := icudat.OpenBundle(item)
	if err != nil {
		return nil, fmt.Errorf("coll/%s.res: %w", name, err)
	}
	colls, ok, err := bundle.Path("collations")
	if err != nil || !ok {
		return nil, err
	}
	kinds, err := bundle.Table(colls)
	if err != nil {
		return nil, err
	}
	out := map[string]*colldata.Data{}
	for kind, r := range kinds {
		if !r.IsTable() {
			// "default", which names a type.
			continue
		}
		res, ok, err := bundle.Path("collations", kind, "%%CollationBin")
		if err != nil {
			return nil, fmt.Errorf("coll/%s.res %s: %w", name, kind, err)
		}
		if !ok {
			// An alias, or "default".
			continue
		}
		bin, err := bundle.Binary(res)
		if err != nil {
			return nil, fmt.Errorf("coll/%s.res %s: %w", name, kind, err)
		}
		d, err := readCompiled(bin)
		if err != nil {
			return nil, fmt.Errorf("coll/%s.res %s: %w", name, kind, err)
		}
		if d != nil {
			out[kind] = d
		}
	}
	return out, nil
}

// readCompiled reads a compiled collation's table, or nothing when the
// type has no table of its own or tailors no conjoining jamo.
func readCompiled(bin []byte) (*colldata.Data, error) {
	b, err := icudat.StripHeader(bin)
	if err != nil {
		return nil, err
	}
	if len(b) < 4 {
		return nil, fmt.Errorf("a compiled collation of %d bytes", len(b))
	}
	n := int(int32(binary.LittleEndian.Uint32(b)))
	if n < ixIndexesLength+1 || 4*n > len(b) {
		return nil, fmt.Errorf("a compiled collation with %d indexes", n)
	}
	index := func(i int) int {
		if i >= n {
			return 0
		}
		return int(int32(binary.LittleEndian.Uint32(b[4*i:])))
	}
	part := func(i int) ([]byte, error) {
		start, limit := index(i), index(i+1)
		if limit <= start {
			return nil, nil
		}
		if limit > len(b) {
			return nil, fmt.Errorf("part %d runs past the end", i)
		}
		return b[start:limit], nil
	}
	trieBytes, err := part(ixTrieOffset)
	if err != nil || len(trieBytes) < 8 {
		// Only settings are tailored.
		return nil, err
	}
	trie, err := icudat.ReadUTrie2(trieBytes)
	if err != nil {
		return nil, err
	}
	jamo := false
	for c := rune(0x1100); c < 0x1200; c++ {
		if trie.Get(c) != fallbackCE32 {
			jamo = true
		}
	}
	if !jamo {
		return nil, nil
	}
	built, err := colldata.BuildTrie(trie.Get, trie.ErrorValue())
	if err != nil {
		return nil, err
	}
	for c := rune(0); c <= 0x10ffff; c++ {
		if built.Get(c) != trie.Get(c) {
			return nil, fmt.Errorf("the trie built for U+%04X gives %#x, ICU's %#x", c, built.Get(c), trie.Get(c))
		}
	}
	d := &colldata.Data{Trie: built}
	ces, err := part(ixCEsOffset)
	if err != nil {
		return nil, err
	}
	ce32s, err := part(ixCE32sOffset)
	if err != nil {
		return nil, err
	}
	contexts, err := part(ixContexts)
	if err != nil {
		return nil, err
	}
	if len(ces)%8 != 0 || len(ce32s)%4 != 0 || len(contexts)%2 != 0 {
		return nil, fmt.Errorf("arrays of %d, %d and %d bytes", len(ces), len(ce32s), len(contexts))
	}
	// The arrays are little-endian, as the collator reads them.
	d.CEs = colldata.U64s(append([]byte(nil), ces...))
	d.CE32s = colldata.U32s(append([]byte(nil), ce32s...))
	d.Contexts = colldata.U16s(append([]byte(nil), contexts...))
	return d, nil
}
