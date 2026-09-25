package icudat

import (
	"encoding/binary"
	"fmt"
)

// A UTrie2 is ICU's older code point trie, with 32-bit values, as compiled
// collation data holds it: a header, a 16-bit index, then the data.
type UTrie2 struct {
	index      []uint16
	data       []uint32
	highStart  rune
	highValue  uint32
	errorValue uint32
}

// The constants are ICU's, from utrie2.h.
const (
	u2Shift1             = 6 + 5
	u2Shift2             = 5
	u2IndexShift         = 2
	u2DataMask           = 1<<u2Shift2 - 1
	u2Index2Mask         = 1<<(u2Shift1-u2Shift2) - 1
	u2LSCPIndex2Offset   = 0x10000 >> u2Shift2
	u2Index2BMPLength    = u2LSCPIndex2Offset + 0x400>>u2Shift2
	u2UTF82BIndex2Length = 0x800 >> 6
	u2Index1Offset       = u2Index2BMPLength + u2UTF82BIndex2Length
	u2OmittedBMPIndex1   = 0x10000 >> u2Shift1
	u2BadUTF8DataOffset  = 0x80
	u2DataGranularity    = 1 << u2IndexShift
	u2Signature          = 0x54726932 // "Tri2"
	u2Header             = 16
)

// ReadUTrie2 reads a serialized UTrie2 of 32-bit values.
func ReadUTrie2(b []byte) (*UTrie2, error) {
	if len(b) < u2Header || binary.LittleEndian.Uint32(b) != u2Signature {
		return nil, fmt.Errorf("not a UTrie2")
	}
	options := binary.LittleEndian.Uint16(b[4:])
	if options&0xf != 1 {
		return nil, fmt.Errorf("a UTrie2 of %d-bit values", 16<<(options&0xf))
	}
	indexLength := int(binary.LittleEndian.Uint16(b[6:]))
	dataLength := int(binary.LittleEndian.Uint16(b[8:])) << u2IndexShift
	shiftedHighStart := int(binary.LittleEndian.Uint16(b[14:]))
	if u2Header+2*indexLength+4*dataLength > len(b) {
		return nil, fmt.Errorf("a UTrie2 past its end")
	}
	t := &UTrie2{
		index:     make([]uint16, indexLength),
		data:      make([]uint32, dataLength),
		highStart: rune(shiftedHighStart << u2Shift1),
	}
	for i := range t.index {
		t.index[i] = binary.LittleEndian.Uint16(b[u2Header+2*i:])
	}
	base := u2Header + 2*indexLength
	for i := range t.data {
		t.data[i] = binary.LittleEndian.Uint32(b[base+4*i:])
	}
	if dataLength <= u2BadUTF8DataOffset || dataLength < u2DataGranularity {
		return nil, fmt.Errorf("a UTrie2 with %d values", dataLength)
	}
	t.errorValue = t.data[u2BadUTF8DataOffset]
	t.highValue = t.data[dataLength-u2DataGranularity]
	return t, nil
}

// Get is utrie2_get32: a code point's value. A lead surrogate code point
// is read as a code point, not as the code unit ICU keeps apart.
func (t *UTrie2) Get(c rune) uint32 {
	i, ok := t.dataIndex(c)
	if !ok {
		return t.errorValue
	}
	return t.data[i]
}

// HighStart is where the values stop varying: every code point from here
// on has HighValue.
func (t *UTrie2) HighStart() rune { return t.highStart }

// HighValue is the value of every code point from HighStart on.
func (t *UTrie2) HighValue() uint32 { return t.highValue }

// ErrorValue is the value of what is not a code point.
func (t *UTrie2) ErrorValue() uint32 { return t.errorValue }

func (t *UTrie2) dataIndex(c rune) (int, bool) {
	raw := func(offset int) (int, bool) {
		i := offset + int(c>>u2Shift2)
		if i < 0 || i >= len(t.index) {
			return 0, false
		}
		d := int(t.index[i])<<u2IndexShift + int(c&u2DataMask)
		return d, d < len(t.data)
	}
	switch {
	case c < 0 || c > 0x10ffff:
		return 0, false
	case c < 0xd800:
		return raw(0)
	case c <= 0xffff:
		if c <= 0xdbff {
			return raw(u2LSCPIndex2Offset - 0xd800>>u2Shift2)
		}
		return raw(0)
	case c >= t.highStart:
		return len(t.data) - u2DataGranularity, true
	}
	i1 := u2Index1Offset - u2OmittedBMPIndex1 + int(c>>u2Shift1)
	if i1 >= len(t.index) {
		return 0, false
	}
	i2 := int(t.index[i1]) + int(c>>u2Shift2)&u2Index2Mask
	if i2 >= len(t.index) {
		return 0, false
	}
	d := int(t.index[i2])<<u2IndexShift + int(c&u2DataMask)
	return d, d < len(t.data)
}
