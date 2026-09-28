package intl_test

import (
	"strings"
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// TestParseExactDecimal pins that a number too large for a float is written
// in full when read as an exact decimal, as V8 writes a BigInt, and as an
// infinity when read as a string is, as ECMA-402 has it.
func TestParseExactDecimal(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	nf, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	big := "1" + strings.Repeat("0", 400)
	want := "10" + strings.Repeat(",000", 400/3)
	if got := nf.FormatDecimal(intl.ParseExactDecimal(big)); got != want {
		t.Errorf("exact = %d characters, want %d", len(got), len(want))
	}
	if got := nf.FormatDecimal(intl.ParseExactDecimal("-" + big)); got != "-"+want {
		t.Errorf("negative exact = %.20q", got)
	}
	if got := nf.FormatDecimal(intl.ParseDecimal(big)); got != "∞" {
		t.Errorf("string = %q, want ∞", got)
	}
}
