// Package blob is the length-prefixed encoding the generated tables are
// written in.
//
// It is deliberately dull. The seam that matters is the Source the bytes
// arrive through, not a clever encoding, and a dull one can be read by eye
// when a generator and a reader disagree about what is in a file.
//
// Every table begins with a version byte, so data written by an older command
// is refused rather than misread.
package blob

import (
	"encoding/binary"
	"fmt"
)

// A Writer builds a table.
type Writer struct{ b []byte }

// NewWriter starts a table of the given version.
func NewWriter(version byte) *Writer { return &Writer{b: []byte{version}} }

// String appends a length-prefixed string.
func (w *Writer) String(s string) {
	w.b = binary.AppendUvarint(w.b, uint64(len(s)))
	w.b = append(w.b, s...)
}

// Uint appends a number.
func (w *Writer) Uint(v int) {
	if v < 0 {
		v = 0
	}
	w.b = binary.AppendUvarint(w.b, uint64(v))
}

// Bytes returns the finished table.
func (w *Writer) Bytes() []byte { return w.b }

// A Reader takes a table apart.
type Reader struct {
	b   []byte
	err error
}

// NewReader starts reading a table, checking its version.
func NewReader(b []byte, version byte) (*Reader, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("blob: the data is empty")
	}
	if b[0] != version {
		return nil, fmt.Errorf("blob: the data is version %d, not %d", b[0], version)
	}
	return &Reader{b: b[1:]}, nil
}

// String reads the next length-prefixed string. Once a read has failed every
// later one returns nothing, so a caller can read a whole table and check once
// at the end.
func (r *Reader) String() string {
	n := r.Uint()
	if r.err != nil {
		return ""
	}
	if n > len(r.b) {
		r.err = fmt.Errorf("blob: a string of %d bytes with %d left", n, len(r.b))
		return ""
	}
	s := string(r.b[:n])
	r.b = r.b[n:]
	return s
}

// Bytes reads the next length-prefixed string without copying it: what it
// returns shares the table's memory, so a table read this way is read where it
// lies, and the caller must not change it.
func (r *Reader) Bytes() []byte {
	n := r.Uint()
	if r.err != nil {
		return nil
	}
	if n > len(r.b) {
		r.err = fmt.Errorf("blob: a string of %d bytes with %d left", n, len(r.b))
		return nil
	}
	b := r.b[:n:n]
	r.b = r.b[n:]
	return b
}

// Uint reads the next number.
func (r *Reader) Uint() int {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.err = fmt.Errorf("blob: a number is cut short")
		return 0
	}
	r.b = r.b[n:]
	return int(v)
}

// Left reports how many bytes have not been read.
func (r *Reader) Left() int { return len(r.b) }

// Err returns the first failure, and reports anything left over as one, so
// that a table longer than the reader expected is noticed rather than ignored.
func (r *Reader) Err() error {
	if r.err != nil {
		return r.err
	}
	if len(r.b) != 0 {
		return fmt.Errorf("blob: %d bytes left over", len(r.b))
	}
	return nil
}
