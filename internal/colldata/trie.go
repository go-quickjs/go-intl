package colldata

// A Trie maps every code point to a 32-bit value. It is ICU's code point trie
// (UCPTrie), read as ICU writes it: collation data is exported in this form,
// and reading it where it lies is less to get wrong than converting it.
//
// Most code points share a value with their neighbours, so the trie stores
// blocks of values and an index of blocks, and the blocks that repeat are
// stored once. A lookup is two or three array reads.
type Trie struct {
	Index U16s
	Data  U32s
	// HighStart is where the values stop varying: every code point from here
	// on has the value at the end of Data.
	HighStart rune
	// Fast tries index the whole of the Basic Multilingual Plane directly;
	// small ones only its first 4,096 code points.
	Fast bool
}

// The constants are ICU's, from ucptrie.h and ucptrie_impl.h.
const (
	fastShift          = 6
	fastDataMask       = 1<<fastShift - 1
	smallMax           = 0xfff
	bmpIndexLength     = 0x10000 >> fastShift
	smallIndexLength   = 0x1000 >> fastShift
	shift3             = 4
	shift2             = shift3 + 5
	shift1             = shift2 + 5
	omittedBMPIndex1   = 0x10000 >> shift1
	index2Mask         = 1<<(shift1-shift2) - 1
	index3Mask         = 1<<(shift2-shift3) - 1
	smallDataMask      = 1<<shift3 - 1
	errorValueNegative = 1
	highValueNegative  = 2
)

// Get returns a code point's value.
func (t *Trie) Get(c rune) uint32 {
	i := t.dataIndex(c)
	if i < 0 || i >= t.Data.Len() {
		return t.Data.At(t.Data.Len() - errorValueNegative)
	}
	return t.Data.At(i)
}

func (t *Trie) dataIndex(c rune) int {
	fastMax := rune(smallMax)
	if t.Fast {
		fastMax = 0xffff
	}
	switch {
	case c < 0 || c > 0x10ffff:
		return t.Data.Len() - errorValueNegative
	case c <= fastMax:
		return int(t.Index.At(int(c>>fastShift))) + int(c&fastDataMask)
	case c >= t.HighStart:
		return t.Data.Len() - highValueNegative
	}
	return t.smallIndex(c)
}

func (t *Trie) smallIndex(c rune) int {
	i1 := int(c >> shift1)
	if t.Fast {
		i1 += bmpIndexLength - omittedBMPIndex1
	} else {
		i1 += smallIndexLength
	}
	i3Block := int(t.Index.At(int(t.Index.At(i1)) + int((c>>shift2)&index2Mask)))
	i3 := int((c >> shift3) & index3Mask)
	var dataBlock int
	if i3Block&0x8000 == 0 {
		// Sixteen-bit indexes.
		dataBlock = int(t.Index.At(i3Block + i3))
	} else {
		// Eighteen-bit indexes, stored as groups of nine units for eight
		// indexes: one unit of high bits, then the eight low halves.
		i3Block = (i3Block & 0x7fff) + (i3 &^ 7) + (i3 >> 3)
		i3 &= 7
		dataBlock = (int(t.Index.At(i3Block)) << (2 + 2*i3)) & 0x30000
		dataBlock |= int(t.Index.At(i3Block + 1 + i3))
	}
	return dataBlock + int(c&smallDataMask)
}
