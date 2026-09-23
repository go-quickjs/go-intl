package intl_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/internal/corpus"
)

// The corpus is what ICU answered. This runs the NumberFormat part of it
// through go-intl and counts what agrees.
//
// Every NumberFormat case in the corpus is in.
func TestNumberFormatMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "NumberFormat" || c.Method != "format" {
			continue
		}
		opts, err := numberOptions(c.Options)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		v, ok := c.Number()
		if !ok {
			t.Errorf("%s: no number to format", c.Source)
			continue
		}

		ran++
		nf, err := intl.NewNumberFormat(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		got := nf.Format(v)
		if got == c.Want {
			matched++
			continue
		}
		differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
			c.Source, got, c.Want))
	}

	if ran == 0 {
		t.Fatal("no NumberFormat cases ran")
	}
	t.Logf("%d of %d NumberFormat cases match ICU exactly (%.2f%%)",
		matched, ran, 100*float64(matched)/float64(ran))

	if matched != ran {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 {
			shown = shown[:25]
		}
		t.Errorf("%d of %d cases differ:\n%s", ran-matched, ran,
			strings.Join(shown, "\n"))
	}
}

// numberOptions turns the corpus's option bag into the Go one. It refuses an
// option it does not know rather than quietly formatting without it, which
// would score a pass for a case that was never really run.
func numberOptions(in map[string]any) (intl.NumberFormatOptions, error) {
	var out intl.NumberFormatOptions
	for key, value := range in {
		switch key {
		case "style":
			switch value {
			case "decimal":
				out.Style = intl.StyleDecimal
			case "percent":
				out.Style = intl.StylePercent
			case "currency":
				out.Style = intl.StyleCurrency
			default:
				return out, fmt.Errorf("style %v", value)
			}
		case "currency":
			out.Currency, _ = value.(string)
		case "currencyDisplay":
			switch value {
			case "symbol":
				out.CurrencyDisplay = intl.CurrencySymbol
			case "narrowSymbol":
				out.CurrencyDisplay = intl.CurrencyNarrowSymbol
			case "code":
				out.CurrencyDisplay = intl.CurrencyCode
			case "name":
				out.CurrencyDisplay = intl.CurrencyName
			default:
				return out, fmt.Errorf("currencyDisplay %v", value)
			}
		case "signDisplay":
			switch value {
			case "auto":
				out.SignDisplay = intl.SignAuto
			case "always":
				out.SignDisplay = intl.SignAlways
			case "exceptZero":
				out.SignDisplay = intl.SignExceptZero
			case "never":
				out.SignDisplay = intl.SignNever
			case "negative":
				out.SignDisplay = intl.SignNegative
			default:
				return out, fmt.Errorf("signDisplay %v", value)
			}
		case "useGrouping":
			if on, ok := value.(bool); ok && !on {
				out.UseGrouping = intl.GroupingNever
			}
		case "notation":
			switch value {
			case "standard":
				out.Notation = intl.NotationStandard
			case "compact":
				out.Notation = intl.NotationCompact
			default:
				return out, fmt.Errorf("notation %v", value)
			}
		case "compactDisplay":
			switch value {
			case "short":
				out.CompactDisplay = intl.CompactShort
			case "long":
				out.CompactDisplay = intl.CompactLong
			default:
				return out, fmt.Errorf("compactDisplay %v", value)
			}
		case "minimumIntegerDigits":
			out.MinimumIntegerDigits = int(value.(float64))
		case "minimumFractionDigits":
			out.MinimumFractionDigits = intl.Digits(int(value.(float64)))
		case "maximumFractionDigits":
			out.MaximumFractionDigits = intl.Digits(int(value.(float64)))
		default:
			return out, fmt.Errorf("unknown option %q", key)
		}
	}
	return out, nil
}
