package main

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/go-quickjs/go-intl/internal/colldata"
	"github.com/go-quickjs/go-intl/internal/icudat"
)

// The unsafe-backward sets: the characters before which ICU's comparison
// may not start, when it compares two strings from after the prefix they
// share (RuleBasedCollator::doCompare). They are ICU's data, read as
// CollationDataReader reads them: the root's is the set ICU builds into
// itself, collunsafe.h, with the ranges the root table adds; a tailoring's
// is the root's with the ranges its compiled table adds, of which only the
// added ranges are kept.

// ixUnsafeBackwardOffset is the index of a compiled table's set, from
// collationdatareader.h.
const ixUnsafeBackwardOffset = 14

const collunsafePath = "icu/source/i18n/collunsafe.h"

var collunsafeArray = regexp.MustCompile(`(?s)unsafe_serializedData\[(\d+)\] = \{(.*?)\};`)
var hexWord = regexp.MustCompile(`0x[0-9A-Fa-f]{4}`)

// rootUnsafe is the root's set: collunsafe.h's, with ucadata.icu's ranges.
func rootUnsafe(sources string, dat icudat.Dat) (colldata.U32s, error) {
	header, err := icudat.ReadSourceFile(sources, collunsafePath)
	if err != nil {
		return nil, err
	}
	m := collunsafeArray.FindSubmatch(header)
	if m == nil {
		return nil, fmt.Errorf("%s has no unsafe_serializedData", collunsafePath)
	}
	var words []uint16
	for _, w := range hexWord.FindAll(m[2], -1) {
		v, err := strconv.ParseUint(string(w[2:]), 16, 16)
		if err != nil {
			return nil, err
		}
		words = append(words, uint16(v))
	}
	if n, _ := strconv.Atoi(string(m[1])); n != len(words) {
		return nil, fmt.Errorf("%s declares %d words and has %d", collunsafePath, n, len(words))
	}
	builtIn, err := serializedSet(words)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", collunsafePath, err)
	}
	item, ok := dat["icudt78l/coll/ucadata.icu"]
	if !ok {
		return nil, fmt.Errorf("icudt78l.dat has no coll/ucadata.icu")
	}
	b, err := icudat.StripHeader(item)
	if err != nil {
		return nil, err
	}
	own, err := compiledUnsafe(b)
	if err != nil {
		return nil, fmt.Errorf("coll/ucadata.icu: %w", err)
	}
	if own == nil {
		return nil, fmt.Errorf("coll/ucadata.icu has no unsafe-backward set")
	}
	return colldata.MakeU32s(union(builtIn, own)), nil
}

// compiledUnsafe is the set a compiled table carries, without its header,
// or nil where it has none.
func compiledUnsafe(b []byte) ([]uint32, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("a compiled collation of %d bytes", len(b))
	}
	n := int(int32(binary.LittleEndian.Uint32(b)))
	if n <= ixUnsafeBackwardOffset+1 || 4*n > len(b) {
		return nil, nil
	}
	start := int(int32(binary.LittleEndian.Uint32(b[4*ixUnsafeBackwardOffset:])))
	limit := int(int32(binary.LittleEndian.Uint32(b[4*(ixUnsafeBackwardOffset+1):])))
	// CollationDataReader reads a set of at least one 16-bit word.
	if limit-start < 2 {
		return nil, nil
	}
	if start < 0 || limit > len(b) || (limit-start)%2 != 0 {
		return nil, fmt.Errorf("an unsafe-backward set at %d to %d of %d bytes", start, limit, len(b))
	}
	words := make([]uint16, (limit-start)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(b[start+2*i:])
	}
	return serializedSet(words)
}

// serializedSet decodes a USerializedSet (uset.h) into an inversion list:
// a length word, with its top bit set when a word giving how many values
// are below U+10000 follows; those values, a word each; then the rest, two
// words each, the high first. A list of odd length runs to U+10FFFF.
func serializedSet(words []uint16) ([]uint32, error) {
	if len(words) == 0 {
		return nil, fmt.Errorf("an empty serialized set")
	}
	length, bmp, at := int(words[0]&0x7fff), int(words[0]&0x7fff), 1
	if words[0]&0x8000 != 0 {
		if len(words) < 2 {
			return nil, fmt.Errorf("a serialized set without its BMP length")
		}
		bmp, at = int(words[1]), 2
	}
	if bmp > length || (length-bmp)%2 != 0 || at+length > len(words) {
		return nil, fmt.Errorf("a serialized set of %d words, %d below U+10000, in %d", length, bmp, len(words))
	}
	var list []uint32
	for _, w := range words[at : at+bmp] {
		list = append(list, uint32(w))
	}
	for i := at + bmp; i < at+length; i += 2 {
		list = append(list, uint32(words[i])<<16|uint32(words[i+1]))
	}
	if len(list)%2 != 0 {
		list = append(list, 0x110000)
	}
	for i := 1; i < len(list); i++ {
		if list[i] <= list[i-1] {
			return nil, fmt.Errorf("a serialized set out of order at %#x", list[i])
		}
	}
	return list, nil
}

// union is the union of two inversion lists.
func union(a, b []uint32) []uint32 {
	type span struct{ start, limit uint32 }
	var spans []span
	for _, l := range [][]uint32{a, b} {
		for i := 0; i+1 < len(l); i += 2 {
			spans = append(spans, span{l[i], l[i+1]})
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].start < spans[j].start || spans[i].start == spans[j].start && spans[i].limit < spans[j].limit
	})
	var out []uint32
	for _, s := range spans {
		if n := len(out); n > 0 && s.start <= out[n-1] {
			out[n-1] = max(out[n-1], s.limit)
			continue
		}
		out = append(out, s.start, s.limit)
	}
	return out
}
