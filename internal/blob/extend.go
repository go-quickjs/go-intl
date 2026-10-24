package blob

import (
	"encoding/binary"
	"fmt"
)

// Pool extensions: a pool that holds only the parts it adds to another.
//
// A data set made from slightly different sources -- a browser's ICU over
// the one this package carries -- shares nearly all its parts with the
// set it differs from. Its pool is that set's pool and a few parts more:
// the generator seeds its pool with the other's parts under their numbers
// (SeedPool), so that every part they share keeps its number and every
// table that did not change is the same bytes, and writes only the parts
// it added (Extension). A reader reads the extension over the pool it
// extends (Extend): a part number below the base's count is the base's
// part, and the rest are the extension's. An extension can itself be
// extended.
//
// An extension is the version byte, a part count no pool can have,
// 0xFFFFFFFF, then the base's part count and the length of its parts,
// which a reader checks the base against, and then the added parts as a
// pool lays them out: their count, where each ends, and the parts.

const extensionMark = 0xffffffff

// SeedPool starts a pool that numbers parts after base's: the parts base
// has keep their numbers, and parts it lacks are numbered after its last.
func SeedPool(version byte, base Shared) *Pool {
	p := NewPool(version)
	n := base.Len()
	p.parts = make([]string, 0, n)
	for i := 0; i < n; i++ {
		part, _ := base.part(i)
		s := string(part)
		if _, dup := p.index[s]; !dup {
			p.index[s] = i
		}
		p.parts = append(p.parts, s)
	}
	p.baseCount = n
	p.baseLen = base.dataLen()
	p.seeded = true
	return p
}

// Extension returns the parts a seeded pool added to its base, laid out
// as an extension of it. A pool that was not seeded has no base to extend.
func (p *Pool) Extension() ([]byte, error) {
	if !p.seeded {
		return nil, fmt.Errorf("blob: an extension of a pool that extends nothing")
	}
	added := p.parts[p.baseCount:]
	size := 0
	for _, s := range added {
		size += len(s)
	}
	out := make([]byte, 0, 1+4+4+8+4+4*len(added)+size)
	out = append(out, p.version)
	out = binary.LittleEndian.AppendUint32(out, extensionMark)
	out = binary.LittleEndian.AppendUint32(out, uint32(p.baseCount))
	out = binary.LittleEndian.AppendUint64(out, uint64(p.baseLen))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(added)))
	end := 0
	for _, s := range added {
		end += len(s)
		out = binary.LittleEndian.AppendUint32(out, uint32(end))
	}
	for _, s := range added {
		out = append(out, s...)
	}
	return out, nil
}

// ExtensionOf is the extension of base that a pool written from a pool
// seeded with it holds: its parts after base's, which must be base's
// parts, as SeedPool numbers them, before them.
func ExtensionOf(base, full Shared, version byte) ([]byte, error) {
	n := base.Len()
	if full.Len() < n {
		return nil, fmt.Errorf("blob: a pool of %d parts over a base of %d", full.Len(), n)
	}
	for i := 0; i < n; i++ {
		part, _ := full.part(i)
		if b, _ := base.part(i); string(b) != string(part) {
			return nil, fmt.Errorf("blob: part %d is not the base's", i)
		}
	}
	// The pool of the parts after base's alone, seeded as if with base.
	p := NewPool(version)
	p.parts = make([]string, n, full.Len())
	for i := n; i < full.Len(); i++ {
		part, _ := full.part(i)
		p.parts = append(p.parts, string(part))
	}
	p.baseCount, p.baseLen, p.seeded = n, base.dataLen(), true
	return p.Extension()
}

// IsExtension reports whether b is a pool extension rather than a pool.
func IsExtension(b []byte) bool {
	return len(b) >= 5 && binary.LittleEndian.Uint32(b[1:]) == extensionMark
}

// Extend reads an extension over the pool it extends, which it checks:
// the base must have the part count and the length the extension was
// made over.
func Extend(base Shared, b []byte, version byte) (Shared, error) {
	if len(b) < 1+4+4+8 || !IsExtension(b) {
		return Shared{}, fmt.Errorf("blob: not a pool extension")
	}
	if b[0] != version {
		return Shared{}, fmt.Errorf("blob: the pool extension is version %d, not %d", b[0], version)
	}
	count := int(binary.LittleEndian.Uint32(b[5:]))
	length := binary.LittleEndian.Uint64(b[9:])
	if count != base.Len() || length != uint64(base.dataLen()) {
		return Shared{}, fmt.Errorf("blob: a pool extension of a pool of %d parts in %d bytes, over one of %d in %d",
			count, length, base.Len(), base.dataLen())
	}
	// The rest is a pool's layout without its version byte.
	added, err := readParts(b[17:])
	if err != nil {
		return Shared{}, fmt.Errorf("blob: the pool extension: %w", err)
	}
	baseCopy := base
	added.base = &baseCopy
	added.baseCount = count
	return added, nil
}

// Len is the number of parts, the base's included.
func (s Shared) Len() int { return s.baseCount + len(s.ends)/4 }

// dataLen is the length of the parts, the base's included.
func (s Shared) dataLen() int {
	n := len(s.data)
	if s.base != nil {
		n += s.base.dataLen()
	}
	return n
}
