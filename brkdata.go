package intl

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The data ICU's break iterators read, as segmentgen copies it out of
// ICU's own: rules compiled into state tables (RBBIDataHeader, format 6),
// the code point trie that classes characters for them (UCPTrie), and the
// dictionaries (BytesTrie and UCharsTrie).

// breakRules are one compiled rule set, RBBIDataWrapper.
type breakRules struct {
	catCount int
	forward  stateTable
	// statuses is the rule status table: groups of a count and values.
	statuses []int32
	trie     codePointTrie
}

// A stateTable is RBBIStateTable: rows of accepting, look-ahead and tag
// values, then the next state for each character category.
type stateTable struct {
	numStates     int
	rowLen        int
	dictStart     int
	lookAheadSize int
	flags         uint32
	data          []byte
	eight         bool // rows of 8-bit values rather than 16-bit
}

const (
	rbbiLookaheadHardBreak = 1
	rbbiBOFRequired        = 2
	rbbi8BitsRows          = 4
)

func (t *stateTable) field(state, i int) int {
	at := state * t.rowLen
	if t.eight {
		return int(t.data[at+i])
	}
	return int(binary.LittleEndian.Uint16(t.data[at+2*i:]))
}

func (t *stateTable) accepting(state int) int      { return t.field(state, 0) }
func (t *stateTable) lookAhead(state int) int      { return t.field(state, 1) }
func (t *stateTable) tagsIdx(state int) int        { return t.field(state, 2) }
func (t *stateTable) next(state, category int) int { return t.field(state, 3+category) }

func decodeBreakRules(b []byte) (*breakRules, error) {
	u32 := func(at int) int { return int(binary.LittleEndian.Uint32(b[at:])) }
	if len(b) < 80 || binary.LittleEndian.Uint32(b) != 0xb1a0 || b[4] != 6 {
		return nil, fmt.Errorf("not RBBI data of format 6")
	}
	if u32(8) > len(b) {
		return nil, fmt.Errorf("RBBI data of %d bytes claims %d", len(b), u32(8))
	}
	r := &breakRules{catCount: u32(12)}
	fTable, fLen := u32(16), u32(20)
	trie, trieLen := u32(32), u32(36)
	status, statusLen := u32(48), u32(52)
	if fTable+fLen > len(b) || trie+trieLen > len(b) || status+statusLen > len(b) || fLen < 20 {
		return nil, fmt.Errorf("RBBI sections out of range")
	}
	t := b[fTable : fTable+fLen]
	r.forward = stateTable{
		numStates:     int(binary.LittleEndian.Uint32(t)),
		rowLen:        int(binary.LittleEndian.Uint32(t[4:])),
		dictStart:     int(binary.LittleEndian.Uint32(t[8:])),
		lookAheadSize: int(binary.LittleEndian.Uint32(t[12:])),
		flags:         binary.LittleEndian.Uint32(t[16:]),
		data:          t[20:],
	}
	r.forward.eight = r.forward.flags&rbbi8BitsRows != 0
	if r.forward.numStates*r.forward.rowLen > len(r.forward.data) {
		return nil, fmt.Errorf("RBBI state table truncated")
	}
	for i := 0; i+4 <= statusLen; i += 4 {
		r.statuses = append(r.statuses, int32(binary.LittleEndian.Uint32(b[status+i:])))
	}
	var err error
	if r.trie, err = decodeCodePointTrie(b[trie : trie+trieLen]); err != nil {
		return nil, fmt.Errorf("RBBI trie: %w", err)
	}
	return r, nil
}

// ruleStatus is getRuleStatus: the largest value of a status group.
func (r *breakRules) ruleStatus(idx int) int {
	if idx < 0 || idx >= len(r.statuses) {
		return 0
	}
	n := int(r.statuses[idx])
	if idx+n >= len(r.statuses) {
		return 0
	}
	return int(r.statuses[idx+n])
}

// A codePointTrie is UCPTrie, of the fast type, with 8- or 16-bit values.
type codePointTrie struct {
	index      []uint16
	data8      []byte
	data16     []uint16
	dataLength int
	highStart  int
}

func decodeCodePointTrie(b []byte) (codePointTrie, error) {
	var t codePointTrie
	if len(b) < 16 || binary.LittleEndian.Uint32(b) != 0x54726933 {
		return t, fmt.Errorf("not a code point trie")
	}
	options := int(binary.LittleEndian.Uint16(b[4:]))
	indexLength := int(binary.LittleEndian.Uint16(b[6:]))
	t.dataLength = (options&0xf000)<<4 | int(binary.LittleEndian.Uint16(b[8:]))
	t.highStart = int(binary.LittleEndian.Uint16(b[14:])) << 9
	if (options>>6)&3 != 0 {
		return t, fmt.Errorf("a trie not of the fast type")
	}
	p := 16
	if p+2*indexLength > len(b) {
		return t, fmt.Errorf("trie index truncated")
	}
	t.index = make([]uint16, indexLength)
	for i := range t.index {
		t.index[i] = binary.LittleEndian.Uint16(b[p+2*i:])
	}
	p += 2 * indexLength
	switch options & 7 {
	case 0: // 16-bit values
		if p+2*t.dataLength > len(b) {
			return t, fmt.Errorf("trie data truncated")
		}
		t.data16 = make([]uint16, t.dataLength)
		for i := range t.data16 {
			t.data16[i] = binary.LittleEndian.Uint16(b[p+2*i:])
		}
	case 2: // 8-bit values
		if p+t.dataLength > len(b) {
			return t, fmt.Errorf("trie data truncated")
		}
		t.data8 = b[p : p+t.dataLength]
	default:
		return t, fmt.Errorf("trie values of width %d", options&7)
	}
	return t, nil
}

// get is ucptrie_get: the value of a code point, and the error value for
// anything that is not one.
func (t *codePointTrie) get(c rune) int {
	var i int
	switch {
	case c >= 0 && c <= 0x7f:
		i = int(c)
	case c >= 0 && c <= 0xffff:
		i = int(t.index[c>>6]) + int(c&63)
	case c >= 0 && c <= 0x10ffff:
		if int(c) >= t.highStart {
			i = t.dataLength - 2
		} else {
			i = t.smallIndex(int(c))
		}
	default:
		i = t.dataLength - 1
	}
	if t.data8 != nil {
		return int(t.data8[i])
	}
	return int(t.data16[i])
}

// smallIndex is ucptrie_internalSmallIndex for a fast trie.
func (t *codePointTrie) smallIndex(c int) int {
	i1 := c>>14 + 0x10000>>6 - 0x10000>>14
	i3Block := int(t.index[int(t.index[i1])+(c>>9)&0x1f])
	i3 := (c >> 4) & 0x1f
	var dataBlock int
	if i3Block&0x8000 == 0 {
		dataBlock = int(t.index[i3Block+i3])
	} else {
		i3Block = (i3Block & 0x7fff) + (i3 &^ 7) + (i3 >> 3)
		i3 &= 7
		dataBlock = (int(t.index[i3Block]) << (2 + 2*i3)) & 0x30000
		dataBlock |= int(t.index[i3Block+1+i3])
	}
	return dataBlock + c&0xf
}

// A trieResult is UStringTrieResult.
type trieResult int

const (
	trieNoMatch trieResult = iota
	trieNoValue
	trieFinalValue
	trieIntermediateValue
)

func (r trieResult) hasValue() bool { return r >= trieFinalValue }

// A charsTrie is UCharsTrie, walked one unit at a time.
type charsTrie struct {
	units     []byte // little-endian UTF-16 units
	pos       int    // -1 once stopped
	remaining int
}

const (
	ucMaxBranchLinearSubNodeLength = 5
	ucMinLinearMatch               = 0x30
	ucMinValueLead                 = 0x40
	ucNodeTypeMask                 = 0x3f
	ucValueIsFinal                 = 0x8000
	ucMinTwoUnitValueLead          = 0x4000
	ucThreeUnitValueLead           = 0x7fff
	ucMinTwoUnitNodeValueLead      = 0x4040
	ucThreeUnitNodeValueLead       = 0x7fc0
	ucMinTwoUnitDeltaLead          = 0xfc00
	ucThreeUnitDeltaLead           = 0xffff
)

func ucValueResult(node int) trieResult { return trieIntermediateValue - trieResult(node>>15) }

func (t *charsTrie) u(i int) int { return int(binary.LittleEndian.Uint16(t.units[2*i:])) }

func (t *charsTrie) first(c int) trieResult {
	t.remaining = -1
	return t.nextImpl(0, c)
}

func (t *charsTrie) next(c int) trieResult {
	pos := t.pos
	if pos < 0 {
		return trieNoMatch
	}
	if length := t.remaining; length >= 0 {
		if c == t.u(pos) {
			pos++
			length--
			t.remaining, t.pos = length, pos
			if length < 0 {
				if node := t.u(pos); node >= ucMinValueLead {
					return ucValueResult(node)
				}
			}
			return trieNoValue
		}
		t.pos = -1
		return trieNoMatch
	}
	return t.nextImpl(pos, c)
}

func (t *charsTrie) nextImpl(pos, c int) trieResult {
	node := t.u(pos)
	pos++
	for {
		if node < ucMinLinearMatch {
			return t.branchNext(pos, node, c)
		} else if node < ucMinValueLead {
			length := node - ucMinLinearMatch
			if c == t.u(pos) {
				pos++
				length--
				t.remaining, t.pos = length, pos
				if length < 0 {
					if n := t.u(pos); n >= ucMinValueLead {
						return ucValueResult(n)
					}
				}
				return trieNoValue
			}
			break
		} else if node&ucValueIsFinal != 0 {
			break
		} else {
			pos = ucSkipNodeValue(pos, node)
			node &= ucNodeTypeMask
		}
	}
	t.pos = -1
	return trieNoMatch
}

func ucSkipNodeValue(pos, lead int) int {
	if lead >= ucMinTwoUnitNodeValueLead {
		if lead < ucThreeUnitNodeValueLead {
			pos++
		} else {
			pos += 2
		}
	}
	return pos
}

func ucSkipValue(pos, lead int) int {
	if lead >= ucMinTwoUnitValueLead {
		if lead < ucThreeUnitValueLead {
			pos++
		} else {
			pos += 2
		}
	}
	return pos
}

func (t *charsTrie) jumpByDelta(pos int) int {
	delta := t.u(pos)
	pos++
	if delta >= ucMinTwoUnitDeltaLead {
		if delta == ucThreeUnitDeltaLead {
			delta = t.u(pos)<<16 | t.u(pos+1)
			pos += 2
		} else {
			delta = (delta-ucMinTwoUnitDeltaLead)<<16 | t.u(pos)
			pos++
		}
	}
	return pos + delta
}

func (t *charsTrie) skipDelta(pos int) int {
	delta := t.u(pos)
	pos++
	if delta >= ucMinTwoUnitDeltaLead {
		if delta == ucThreeUnitDeltaLead {
			pos += 2
		} else {
			pos++
		}
	}
	return pos
}

func (t *charsTrie) branchNext(pos, length, c int) trieResult {
	if length == 0 {
		length = t.u(pos)
		pos++
	}
	length++
	for length > ucMaxBranchLinearSubNodeLength {
		if c < t.u(pos) {
			pos++
			length >>= 1
			pos = t.jumpByDelta(pos)
		} else {
			pos++
			length = length - length>>1
			pos = t.skipDelta(pos)
		}
	}
	for {
		if c == t.u(pos) {
			pos++
			node := t.u(pos)
			var result trieResult
			if node&ucValueIsFinal != 0 {
				result = trieFinalValue
			} else {
				pos++
				var delta int
				switch {
				case node < ucMinTwoUnitValueLead:
					delta = node
				case node < ucThreeUnitValueLead:
					delta = (node-ucMinTwoUnitValueLead)<<16 | t.u(pos)
					pos++
				default:
					delta = t.u(pos)<<16 | t.u(pos+1)
					pos += 2
				}
				pos += delta
				node = t.u(pos)
				result = trieNoValue
				if node >= ucMinValueLead {
					result = ucValueResult(node)
				}
			}
			t.pos = pos
			return result
		}
		pos++
		length--
		pos = ucSkipValue(pos+1, t.u(pos)&0x7fff)
		if length <= 1 {
			break
		}
	}
	if c == t.u(pos) {
		pos++
		t.pos = pos
		if node := t.u(pos); node >= ucMinValueLead {
			return ucValueResult(node)
		}
		return trieNoValue
	}
	t.pos = -1
	return trieNoMatch
}

func (t *charsTrie) value() int {
	pos := t.pos
	lead := t.u(pos)
	pos++
	if lead&ucValueIsFinal != 0 {
		lead &= 0x7fff
		switch {
		case lead < ucMinTwoUnitValueLead:
			return lead
		case lead < ucThreeUnitValueLead:
			return (lead-ucMinTwoUnitValueLead)<<16 | t.u(pos)
		}
		return t.u(pos)<<16 | t.u(pos+1)
	}
	switch {
	case lead < ucMinTwoUnitNodeValueLead:
		return lead>>6 - 1
	case lead < ucThreeUnitNodeValueLead:
		return ((lead&0x7fc0)-ucMinTwoUnitNodeValueLead)<<10 | t.u(pos)
	}
	return t.u(pos)<<16 | t.u(pos+1)
}

// A bytesTrie is BytesTrie, walked one byte at a time.
type bytesTrie struct {
	bytes     []byte
	pos       int // -1 once stopped
	remaining int
}

const (
	btMaxBranchLinearSubNodeLength = 5
	btMinLinearMatch               = 0x10
	btMinValueLead                 = 0x20
	btValueIsFinal                 = 1
	btMinOneByteValueLead          = 0x10
	btMinTwoByteValueLead          = 0x51
	btMinThreeByteValueLead        = 0x6c
	btFourByteValueLead            = 0x7e
	btMinTwoByteDeltaLead          = 0xc0
	btMinThreeByteDeltaLead        = 0xf0
	btFourByteDeltaLead            = 0xfe
)

func btValueResult(node int) trieResult {
	return trieIntermediateValue - trieResult(node&btValueIsFinal)
}

func (t *bytesTrie) b(i int) int { return int(t.bytes[i]) }

// first and next take a byte, a negative one being read as a byte, as
// BytesTrie does: -1 is 0xff.
func (t *bytesTrie) first(in int) trieResult {
	t.remaining = -1
	if in < 0 {
		in += 0x100
	}
	return t.nextImpl(0, in)
}

func (t *bytesTrie) next(in int) trieResult {
	pos := t.pos
	if pos < 0 {
		return trieNoMatch
	}
	if in < 0 {
		in += 0x100
	}
	if length := t.remaining; length >= 0 {
		if in == t.b(pos) {
			pos++
			length--
			t.remaining, t.pos = length, pos
			if length < 0 {
				if node := t.b(pos); node >= btMinValueLead {
					return btValueResult(node)
				}
			}
			return trieNoValue
		}
		t.pos = -1
		return trieNoMatch
	}
	return t.nextImpl(pos, in)
}

func (t *bytesTrie) nextImpl(pos, in int) trieResult {
	for {
		node := t.b(pos)
		pos++
		if node < btMinLinearMatch {
			return t.branchNext(pos, node, in)
		} else if node < btMinValueLead {
			length := node - btMinLinearMatch
			if in == t.b(pos) {
				pos++
				length--
				t.remaining, t.pos = length, pos
				if length < 0 {
					if n := t.b(pos); n >= btMinValueLead {
						return btValueResult(n)
					}
				}
				return trieNoValue
			}
			break
		} else if node&btValueIsFinal != 0 {
			break
		} else {
			pos = btSkipValue(pos, node)
		}
	}
	t.pos = -1
	return trieNoMatch
}

func btSkipValue(pos, lead int) int {
	if lead >= btMinTwoByteValueLead<<1 {
		switch {
		case lead < btMinThreeByteValueLead<<1:
			pos++
		case lead < btFourByteValueLead<<1:
			pos += 2
		default:
			pos += 3 + (lead>>1)&1
		}
	}
	return pos
}

func (t *bytesTrie) readValue(pos, lead int) int {
	switch {
	case lead < btMinTwoByteValueLead:
		return lead - btMinOneByteValueLead
	case lead < btMinThreeByteValueLead:
		return (lead-btMinTwoByteValueLead)<<8 | t.b(pos)
	case lead < btFourByteValueLead:
		return (lead-btMinThreeByteValueLead)<<16 | t.b(pos)<<8 | t.b(pos+1)
	case lead == btFourByteValueLead:
		return t.b(pos)<<16 | t.b(pos+1)<<8 | t.b(pos+2)
	}
	return int(int32(uint32(t.b(pos))<<24 | uint32(t.b(pos+1))<<16 | uint32(t.b(pos+2))<<8 | uint32(t.b(pos+3))))
}

// valueLength is how many bytes readValue reads after a lead.
func btValueLength(lead int) int {
	switch {
	case lead < btMinTwoByteValueLead:
		return 0
	case lead < btMinThreeByteValueLead:
		return 1
	case lead < btFourByteValueLead:
		return 2
	case lead == btFourByteValueLead:
		return 3
	}
	return 4
}

func (t *bytesTrie) jumpByDelta(pos int) int {
	delta := t.b(pos)
	pos++
	switch {
	case delta < btMinTwoByteDeltaLead:
	case delta < btMinThreeByteDeltaLead:
		delta = (delta-btMinTwoByteDeltaLead)<<8 | t.b(pos)
		pos++
	case delta < btFourByteDeltaLead:
		delta = (delta-btMinThreeByteDeltaLead)<<16 | t.b(pos)<<8 | t.b(pos+1)
		pos += 2
	case delta == btFourByteDeltaLead:
		delta = t.b(pos)<<16 | t.b(pos+1)<<8 | t.b(pos+2)
		pos += 3
	default:
		delta = int(int32(uint32(t.b(pos))<<24 | uint32(t.b(pos+1))<<16 | uint32(t.b(pos+2))<<8 | uint32(t.b(pos+3))))
		pos += 4
	}
	return pos + delta
}

func (t *bytesTrie) skipDelta(pos int) int {
	delta := t.b(pos)
	pos++
	if delta >= btMinTwoByteDeltaLead {
		switch {
		case delta < btMinThreeByteDeltaLead:
			pos++
		case delta < btFourByteDeltaLead:
			pos += 2
		default:
			pos += 3 + delta&1
		}
	}
	return pos
}

func (t *bytesTrie) branchNext(pos, length, in int) trieResult {
	if length == 0 {
		length = t.b(pos)
		pos++
	}
	length++
	for length > btMaxBranchLinearSubNodeLength {
		if in < t.b(pos) {
			pos++
			length >>= 1
			pos = t.jumpByDelta(pos)
		} else {
			pos++
			length = length - length>>1
			pos = t.skipDelta(pos)
		}
	}
	for {
		if in == t.b(pos) {
			pos++
			node := t.b(pos)
			var result trieResult
			if node&btValueIsFinal != 0 {
				result = trieFinalValue
			} else {
				pos++
				node >>= 1
				delta := t.readValue(pos, node)
				pos += btValueLength(node)
				pos += delta
				node = t.b(pos)
				result = trieNoValue
				if node >= btMinValueLead {
					result = btValueResult(node)
				}
			}
			t.pos = pos
			return result
		}
		pos++
		length--
		pos = btSkipValue(pos+1, t.b(pos))
		if length <= 1 {
			break
		}
	}
	if in == t.b(pos) {
		pos++
		t.pos = pos
		if node := t.b(pos); node >= btMinValueLead {
			return btValueResult(node)
		}
		return trieNoValue
	}
	t.pos = -1
	return trieNoMatch
}

func (t *bytesTrie) value() int {
	lead := t.b(t.pos)
	return t.readValue(t.pos+1, lead>>1)
}

// A breakDictionary is ICU's DictionaryMatcher over a .dict file: a
// UCharsTrie, or a BytesTrie of characters offset into bytes.
type breakDictionary struct {
	chars     []byte // a UCharsTrie's little-endian units
	bytes     []byte
	transform int
}

func decodeBreakDictionary(b []byte) (*breakDictionary, error) {
	if len(b) < 32 {
		return nil, fmt.Errorf("dictionary truncated")
	}
	idx := func(i int) int { return int(int32(binary.LittleEndian.Uint32(b[4*i:]))) }
	offset, total, trieType, transform := idx(0), idx(3), idx(4)&7, idx(5)
	if offset < 32 || offset > len(b) || total > len(b) {
		return nil, fmt.Errorf("dictionary indexes out of range")
	}
	d := &breakDictionary{transform: transform}
	switch trieType {
	case 0:
		d.bytes = b[offset:total]
	case 1:
		d.chars = b[offset:total]
	default:
		return nil, fmt.Errorf("dictionary of trie type %d", trieType)
	}
	return d, nil
}

// transformChar is BytesDictionaryMatcher::transform.
func (d *breakDictionary) transformChar(c rune) int {
	if d.transform&0x7f000000 == 0x1000000 {
		switch c {
		case 0x200d:
			return 0xff
		case 0x200c:
			return 0xfe
		}
		delta := int(c) - d.transform&0x1fffff
		if delta < 0 || 0xfd < delta {
			return -1
		}
		return delta
	}
	return int(c)
}

// matches is DictionaryMatcher::matches: the dictionary words the text
// starts with, from the iterator's position, up to maxLength code units
// and limit words. lengths are in code units, cpLengths in code points;
// either, and values, may be nil. prefix is the code points matched.
func (d *breakDictionary) matches(t *utext, maxLength, limit int, lengths, cpLengths, values []int) (count, prefix int) {
	start := t.index()
	var ct charsTrie
	var bt bytesTrie
	ct.units, bt.bytes = d.chars, d.bytes
	matched := 0
	for c := t.next32(); c >= 0; c = t.next32() {
		var result trieResult
		if d.chars != nil {
			// The code point is taken as one unit, so a supplementary
			// character matches nothing.
			if matched == 0 {
				result = ct.first(int(c))
			} else {
				result = ct.next(int(c))
			}
		} else {
			if matched == 0 {
				result = bt.first(d.transformChar(c))
			} else {
				result = bt.next(d.transformChar(c))
			}
		}
		length := t.index() - start
		matched++
		if result.hasValue() {
			if count < limit {
				if values != nil {
					if d.chars != nil {
						values[count] = ct.value()
					} else {
						values[count] = bt.value()
					}
				}
				if lengths != nil {
					lengths[count] = length
				}
				if cpLengths != nil {
					cpLengths[count] = matched
				}
				count++
			}
			if result == trieFinalValue {
				break
			}
		} else if result == trieNoMatch {
			break
		}
		if length >= maxLength {
			break
		}
	}
	return count, matched
}

// A codeRanges is a set of code points as sorted ranges.
type codeRanges [][2]rune

func (s codeRanges) contains(c rune) bool {
	i := sort.Search(len(s), func(i int) bool { return s[i][1] >= c })
	return i < len(s) && s[i][0] <= c
}

// with is the set with a range added; without, with one taken out.
func (s codeRanges) with(lo, hi rune) codeRanges {
	out := append(codeRanges{}, s...)
	out = append(out, [2]rune{lo, hi})
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	merged := codeRanges{}
	for _, r := range out {
		if n := len(merged); n > 0 && r[0] <= merged[n-1][1]+1 {
			if r[1] > merged[n-1][1] {
				merged[n-1][1] = r[1]
			}
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

func (s codeRanges) without(lo, hi rune) codeRanges {
	var out codeRanges
	for _, r := range s {
		if r[1] < lo || r[0] > hi {
			out = append(out, r)
			continue
		}
		if r[0] < lo {
			out = append(out, [2]rune{r[0], lo - 1})
		}
		if r[1] > hi {
			out = append(out, [2]rune{hi + 1, r[1]})
		}
	}
	return out
}

func parseCodeRanges(fields []string) (codeRanges, error) {
	var s codeRanges
	for _, f := range fields {
		lo, hi, ok := strings.Cut(f, "-")
		if !ok {
			hi = lo
		}
		a, err1 := strconv.ParseUint(lo, 16, 32)
		z, err2 := strconv.ParseUint(hi, 16, 32)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("range %q", f)
		}
		s = append(s, [2]rune{rune(a), rune(z)})
	}
	return s, nil
}
