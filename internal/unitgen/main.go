// Command unitgen writes the measurement tables the intl package carries.
//
// It reads ICU's unit bundles, data/unit/*.txt in the data archive
// SOURCES.md pins: each locale's units as ICU resolves them.
//
//	go run ./internal/unitgen [-seed pool.bin] icu4c-78.3-data.zip
//
// -seed numbers the pool's parts after an earlier pool's (datawrite.NewPool).
//
// Only the units ECMA-402 sanctions are written. CLDR carries 268 of them and
// ECMA-402 allows 45, chosen because every language has a name for each, so
// the rest would be carried for nothing. A unit may also be one of these
// divided by another, which is why the divisor patterns are kept too.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/unitdata"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

// sanctioned maps ECMA-402's name for a unit to CLDR's, which carries the
// category it belongs to: ICU keeps the unit under its category,
// units/length/foot. The mapping is written out rather than guessed from
// the suffix, because the suffix is ambiguous: "meter" ends the key of a
// length, an area, a volume and a torque.
var sanctioned = map[string]string{
	"acre": "area-acre", "bit": "digital-bit", "byte": "digital-byte",
	"celsius": "temperature-celsius", "centimeter": "length-centimeter",
	"day": "duration-day", "degree": "angle-degree",
	"fahrenheit": "temperature-fahrenheit", "fluid-ounce": "volume-fluid-ounce",
	"foot": "length-foot", "gallon": "volume-gallon",
	"gigabit": "digital-gigabit", "gigabyte": "digital-gigabyte",
	"gram": "mass-gram", "hectare": "area-hectare", "hour": "duration-hour",
	"inch": "length-inch", "kilobit": "digital-kilobit",
	"kilobyte": "digital-kilobyte", "kilogram": "mass-kilogram",
	"kilometer": "length-kilometer", "liter": "volume-liter",
	"megabit": "digital-megabit", "megabyte": "digital-megabyte",
	"meter": "length-meter", "microsecond": "duration-microsecond",
	"mile": "length-mile", "mile-scandinavian": "length-mile-scandinavian",
	"milliliter": "volume-milliliter", "millimeter": "length-millimeter",
	"millisecond": "duration-millisecond", "minute": "duration-minute",
	"month": "duration-month", "nanosecond": "duration-nanosecond",
	"ounce": "mass-ounce", "percent": "concentr-percent",
	"petabyte": "digital-petabyte", "pound": "mass-pound",
	"second": "duration-second", "stone": "mass-stone",
	"terabit": "digital-terabit", "terabyte": "digital-terabyte",
	"week": "duration-week", "yard": "length-yard", "year": "duration-year",

	// Five pairs have a wording of their own rather than one composed from
	// the two parts, and ICU prefers it. Chinese is where that shows: the
	// parts compose to "987公里/小时" but the wording it actually uses is
	// "987 km/h".
	"kilometer-per-hour":  "speed-kilometer-per-hour",
	"mile-per-hour":       "speed-mile-per-hour",
	"meter-per-second":    "speed-meter-per-second",
	"liter-per-kilometer": "consumption-liter-per-kilometer",
	"mile-per-gallon":     "consumption-mile-per-gallon",
}

// widthNames are ICU's tables of each width, in unitdata's order.
var widthNames = [unitdata.Widths]string{"units", "unitsShort", "unitsNarrow"}

// notCounts are the keys of a unit's table that are not a plural category's
// pattern: its display name, its per-unit pattern, and its gender.
var notCounts = map[string]bool{"dnam": true, "per": true, "gender": true}

func main() {
	seed := flag.String("seed", "", "a pool to number the parts after")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/unitgen [-seed pool.bin] <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *seed); err != nil {
		fmt.Fprintln(os.Stderr, "unitgen:", err)
		os.Exit(1)
	}
}

func run(zip, seed string) error {
	units, err := icusrc.OpenTree(zip, "unit")
	if err != nil {
		return err
	}
	defer units.Close()
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}
	// The locales share one pool, written beside them. EachLocale visits
	// the bundles in order, so the pool is the same every run.
	pool, err := datawrite.NewPool(unitdata.Version, seed)
	if err != nil {
		return err
	}
	built, err := icusrc.EachLocale(units, fb, func(v *icusrc.View) ([]byte, error) {
		l, err := read(v)
		if err != nil || l == nil {
			return nil, err
		}
		return unitdata.Encode(l, pool), nil
	})
	if err != nil {
		return err
	}

	set := writeset.New()
	if err := datawrite.AddLocales(set, filepath.Join("data", "units"), built); err != nil {
		return err
	}
	set.File(filepath.Join("data", "unitsshared.bin"), pool.Bytes())
	if err := set.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "unitgen: %d locales, %d units each\n", len(built), len(sanctioned))
	return nil
}

func read(v *icusrc.View) (*unitdata.Built, error) {
	var out unitdata.Built
	any := false
	for i, width := range widthNames {
		w := &out.Widths[i]
		per, err := v.Get(width, "compound", "per")
		if err != nil {
			return nil, err
		}
		if per != nil {
			w.Compound = per.Value
		}
		for short, key := range sanctioned {
			category, name, _ := strings.Cut(key, "-")
			fields, err := v.Get(width, category, name)
			if err != nil {
				return nil, err
			}
			if fields == nil {
				continue
			}
			u := unitdata.Unit{Name: short}
			if p := fields.Get("per"); p != nil {
				u.PerUnit = p.Value
			}
			for _, f := range fields.Children {
				if f.Table || notCounts[f.Key] || f.Value == "" {
					continue
				}
				u.Patterns = append(u.Patterns, unitdata.CountedText{Count: f.Key, Text: f.Value})
			}
			if len(u.Patterns) == 0 {
				continue
			}
			sort.Slice(u.Patterns, func(a, b int) bool {
				return u.Patterns[a].Count < u.Patterns[b].Count
			})
			w.Units = append(w.Units, u)
			any = true
		}
		sort.Slice(w.Units, func(a, b int) bool { return w.Units[a].Name < w.Units[b].Name })
	}
	if !any {
		return nil, nil
	}
	return &out, nil
}
