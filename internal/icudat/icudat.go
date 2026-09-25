// Package icudat reads ICU's compiled data: icudt78l.dat as ICU 78.3's
// source release ships it prebuilt (source/data/in), which is the data Node
// carries.
//
// Generators copy what they need out of it as ICU compiled it, where
// compiling it again would be ICU's own compilers to port: the break rules
// and dictionaries, and the collation tailorings the ICU4X export leaves
// incomplete. Nothing here runs at run time.
//
// The formats are ICU's: the common data file (ucmndata.h), resource
// bundles (uresdata.h) and UTrie2 (utrie2.h, utrie2_impl.h).
package icudat

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// SourcesSHA256 is icu4c-78.3-sources.tgz, as SOURCES.md pins it.
const SourcesSHA256 = "3a2e7a47604ba702f345878308e6fefeca612ee895cf4a5f222e7955fabfe0c0"

const datPath = "icu/source/data/in/icudt78l.dat"

// Dat is the items of icudt78l.dat by name, "icudt78l/coll/ko.res", each
// with its data header.
type Dat map[string][]byte

// Read reads icudt78l.dat out of the source release, after checking the
// release's checksum.
func Read(sources string) (Dat, error) {
	raw, err := os.ReadFile(sources)
	if err != nil {
		return nil, err
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != SourcesSHA256 {
		return nil, fmt.Errorf("%s has checksum %s, want %s", sources, got, SourcesSHA256)
	}
	z, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	r := tar.NewReader(z)
	var dat []byte
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Name == datPath {
			if dat, err = io.ReadAll(r); err != nil {
				return nil, err
			}
			break
		}
	}
	if dat == nil {
		return nil, fmt.Errorf("%s has no %s", sources, datPath)
	}
	return parseDat(dat)
}

// parseDat splits a common data file: its header, then a table of contents
// of name and data offsets from the table's start.
func parseDat(dat []byte) (Dat, error) {
	toc := int(binary.LittleEndian.Uint16(dat))
	if toc+4 > len(dat) {
		return nil, fmt.Errorf("icudt78l.dat is truncated")
	}
	n := int(binary.LittleEndian.Uint32(dat[toc:]))
	type entry struct {
		name  string
		start int
	}
	entries := make([]entry, n)
	for i := 0; i < n; i++ {
		at := toc + 4 + 8*i
		if at+8 > len(dat) {
			return nil, fmt.Errorf("icudt78l.dat's table of contents is broken")
		}
		nameOff := toc + int(binary.LittleEndian.Uint32(dat[at:]))
		dataOff := toc + int(binary.LittleEndian.Uint32(dat[at+4:]))
		if nameOff >= len(dat) || dataOff > len(dat) {
			return nil, fmt.Errorf("icudt78l.dat's table of contents is broken")
		}
		end := bytes.IndexByte(dat[nameOff:], 0)
		if end < 0 {
			return nil, fmt.Errorf("icudt78l.dat's table of contents is broken")
		}
		entries[i] = entry{string(dat[nameOff : nameOff+end]), dataOff}
	}
	out := Dat{}
	for i, e := range entries {
		limit := len(dat)
		if i+1 < n {
			limit = entries[i+1].start
		}
		if limit < e.start {
			return nil, fmt.Errorf("icudt78l.dat's %s ends before it starts", e.name)
		}
		out[e.name] = dat[e.start:limit]
	}
	return out, nil
}

// StripHeader drops ICU's data header, whose first two bytes are its size
// and next two its magic number.
func StripHeader(b []byte) ([]byte, error) {
	if len(b) < 4 || b[2] != 0xda || b[3] != 0x27 {
		return nil, fmt.Errorf("not ICU data")
	}
	size := int(binary.LittleEndian.Uint16(b))
	if size > len(b) {
		return nil, fmt.Errorf("a header of %d bytes in %d", size, len(b))
	}
	return b[size:], nil
}
