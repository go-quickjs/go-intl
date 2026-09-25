package blob

import (
	"math"
	"strconv"
	"testing"
)

// A number wider than 32 bits -- the end ICU gives a metazone use that has
// not ended, 9999-12-31 in minutes, or a collation's 32-bit primary -- reads
// back whole through Uint64 on every platform.
func TestUint64RoundTrip(t *testing.T) {
	values := []int64{0, 1, 1<<31 - 1, 1 << 31, 4223371680, 0xFE000000, math.MaxInt64}
	w := NewWriter(1)
	for _, v := range values {
		w.Uint64(v)
	}
	r, err := NewReader(w.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range values {
		if got := r.Uint64(); got != want {
			t.Errorf("Uint64 = %d, want %d", got, want)
		}
	}
	if err := r.Err(); err != nil {
		t.Error(err)
	}
}

// Uint refuses a number an int cannot hold rather than wrapping it round,
// which is what hid two 32-bit bugs.
func TestUintRefusesWhatAnIntCannotHold(t *testing.T) {
	w := NewWriter(1)
	w.Uint64(4223371680)
	r, err := NewReader(w.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Uint()
	if strconv.IntSize == 64 {
		if int64(got) != 4223371680 || r.Err() != nil {
			t.Errorf("Uint = %d, %v on a 64-bit platform", got, r.Err())
		}
		return
	}
	if got != 0 || r.Err() == nil {
		t.Errorf("Uint = %d, %v on a 32-bit platform, want a failure", got, r.Err())
	}
}
