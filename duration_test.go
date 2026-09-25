package intl_test

import (
	"math"
	"testing"

	"github.com/go-quickjs/go-intl"
)

// Valid is ECMA-402's IsValidDuration: whole numbers of one sign, the
// calendar units below 2**32, the rest below 2**53 seconds in all.
func TestDurationValid(t *testing.T) {
	for _, c := range []struct {
		d    intl.Duration
		want bool
	}{
		{intl.Duration{}, true},
		{intl.Duration{intl.DurationHours: 1, intl.DurationMinutes: 46}, true},
		{intl.Duration{intl.DurationHours: -1, intl.DurationMinutes: -46}, true},
		{intl.Duration{intl.DurationHours: -1, intl.DurationMinutes: 46}, false},
		{intl.Duration{intl.DurationSeconds: 1.5}, false},
		{intl.Duration{intl.DurationDays: math.NaN()}, false},
		{intl.Duration{intl.DurationDays: math.Inf(1)}, false},
		{intl.Duration{intl.DurationYears: 1<<32 - 1}, true},
		{intl.Duration{intl.DurationYears: 1 << 32}, false},
		{intl.Duration{intl.DurationSeconds: 1<<53 - 1}, true},
		{intl.Duration{intl.DurationSeconds: 1 << 53}, false},
		// 2**53 seconds less a nanosecond, counted exactly.
		{intl.Duration{intl.DurationSeconds: 1<<53 - 1, intl.DurationNanoseconds: 999999999}, true},
		{intl.Duration{intl.DurationSeconds: 1<<53 - 1, intl.DurationNanoseconds: 1e9}, false},
	} {
		if got := c.d.Valid(); got != c.want {
			t.Errorf("%v.Valid() = %v, want %v", c.d, got, c.want)
		}
	}
}

// An invalid duration is an error rather than a guess.
func TestDurationFormatRefusesInvalid(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	f, err := intl.NewDurationFormat(loc, intl.DurationFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Format(intl.Duration{intl.DurationHours: 1, intl.DurationMinutes: -1}); err == nil {
		t.Error("a duration of mixed signs was formatted")
	}
	got, err := f.Format(intl.Duration{intl.DurationHours: 1, intl.DurationMinutes: 46, intl.DurationSeconds: 40})
	if err != nil || got != "1 hr, 46 min, 40 sec" {
		t.Errorf("Format = %q, %v", got, err)
	}
}

func BenchmarkNewDurationFormat(b *testing.B) {
	loc, _ := intl.ParseLocale("de")
	for i := 0; i < b.N; i++ {
		if _, err := intl.NewDurationFormat(loc, intl.DurationFormatOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
