package icudat

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// A Bundle is a compiled resource bundle, a .res item: a tree of tables,
// strings and binaries, each a 32-bit resource word whose top four bits
// are its type and the rest its offset.
//
// Only what the generators read is read: tables and binaries in a bundle
// that keeps its own keys. A bundle that shares a pool bundle's keys, as
// the locales tree's do, is refused rather than misread.
type Bundle struct {
	b []byte
	// keysLimit is where the bundle's own keys end, in bytes from its
	// start; a key offset past it would be the pool bundle's.
	keysLimit int
	// units16 is where the 16-bit units start, in bytes.
	units16 int
}

// A Res is a resource word.
type Res uint32

// The resource types used, from uresdata.h.
const (
	resBinary  = 1
	resTable   = 2
	resTable32 = 4
	resTable16 = 5
)

func (r Res) kind() int { return int(r >> 28) }

// IsTable reports whether the resource is a table.
func (r Res) IsTable() bool {
	k := r.kind()
	return k == resTable || k == resTable32 || k == resTable16
}
func (r Res) offset() int { return int(r & 0x0fffffff) }

// OpenBundle reads a .res item, its data header included.
func OpenBundle(item []byte) (*Bundle, error) {
	b, err := StripHeader(item)
	if err != nil {
		return nil, err
	}
	if len(b) < 8 || len(b)%4 != 0 {
		return nil, fmt.Errorf("a resource bundle of %d bytes", len(b))
	}
	n := int(binary.LittleEndian.Uint32(b[4:]) & 0xff)
	if n < 1 || 4+4*n > len(b) {
		return nil, fmt.Errorf("a resource bundle with %d indexes", n)
	}
	index := func(i int) int {
		if i >= n {
			return 0
		}
		return int(binary.LittleEndian.Uint32(b[4+4*i:]))
	}
	const indexKeysTop, indexAttributes, index16BitTop = 1, 5, 6
	const usesPoolBundle = 2
	if index(indexAttributes)&usesPoolBundle != 0 {
		return nil, fmt.Errorf("the bundle's keys are in a pool bundle")
	}
	keysTop := index(indexKeysTop)
	if top := index(index16BitTop); top > 0 && 4*top > len(b) || 4*keysTop > len(b) {
		return nil, fmt.Errorf("a resource bundle's parts run past its end")
	}
	return &Bundle{b: b, keysLimit: 4 * keysTop, units16: 4 * keysTop}, nil
}

// Root is the bundle's top table.
func (bd *Bundle) Root() Res { return Res(binary.LittleEndian.Uint32(bd.b)) }

func (bd *Bundle) u16(at int) (int, error) {
	if at < 0 || at+2 > len(bd.b) {
		return 0, fmt.Errorf("a resource past the bundle's end")
	}
	return int(binary.LittleEndian.Uint16(bd.b[at:])), nil
}

func (bd *Bundle) u32(at int) (int, error) {
	if at < 0 || at+4 > len(bd.b) {
		return 0, fmt.Errorf("a resource past the bundle's end")
	}
	return int(binary.LittleEndian.Uint32(bd.b[at:])), nil
}

func (bd *Bundle) key(offset int) (string, error) {
	if offset >= bd.keysLimit {
		return "", fmt.Errorf("a key in a pool bundle")
	}
	end := bytes.IndexByte(bd.b[offset:], 0)
	if end < 0 {
		return "", fmt.Errorf("an unterminated key")
	}
	return string(bd.b[offset : offset+end]), nil
}

// Table returns a table's items by key. Items of a 16-bit table are
// strings, which no generator reads, and are left out.
func (bd *Bundle) Table(r Res) (map[string]Res, error) {
	out := map[string]Res{}
	switch r.kind() {
	case resTable:
		if r.offset() == 0 {
			return out, nil
		}
		at := 4 * r.offset()
		n, err := bd.u16(at)
		if err != nil {
			return nil, err
		}
		values := at + 2 + 2*n
		if n%2 == 0 {
			// The 32-bit items are aligned after the 16-bit keys.
			values += 2
		}
		for i := 0; i < n; i++ {
			k, err := bd.u16(at + 2 + 2*i)
			if err != nil {
				return nil, err
			}
			name, err := bd.key(k)
			if err != nil {
				return nil, err
			}
			v, err := bd.u32(values + 4*i)
			if err != nil {
				return nil, err
			}
			out[name] = Res(v)
		}
	case resTable32:
		at := 4 * r.offset()
		n, err := bd.u32(at)
		if err != nil {
			return nil, err
		}
		for i := 0; i < n; i++ {
			k, err := bd.u32(at + 4 + 4*i)
			if err != nil {
				return nil, err
			}
			name, err := bd.key(k)
			if err != nil {
				return nil, err
			}
			v, err := bd.u32(at + 4 + 4*n + 4*i)
			if err != nil {
				return nil, err
			}
			out[name] = Res(v)
		}
	case resTable16:
		// Every item is a string.
	default:
		return nil, fmt.Errorf("a resource of type %d is not a table", r.kind())
	}
	return out, nil
}

// Binary returns a binary resource's bytes.
func (bd *Bundle) Binary(r Res) ([]byte, error) {
	if r.kind() != resBinary {
		return nil, fmt.Errorf("a resource of type %d is not a binary", r.kind())
	}
	at := 4 * r.offset()
	n, err := bd.u32(at)
	if err != nil {
		return nil, err
	}
	if at+4+n > len(bd.b) {
		return nil, fmt.Errorf("a binary past the bundle's end")
	}
	return bd.b[at+4 : at+4+n], nil
}

// Path follows keys down from the root.
func (bd *Bundle) Path(keys ...string) (Res, bool, error) {
	r := bd.Root()
	for _, k := range keys {
		t, err := bd.Table(r)
		if err != nil {
			return 0, false, err
		}
		next, ok := t[k]
		if !ok {
			return 0, false, nil
		}
		r = next
	}
	return r, true, nil
}
