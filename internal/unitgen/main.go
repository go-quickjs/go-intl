// Command unitgen writes the measurement tables the intl package carries.
//
// It reads CLDR's cldr-units-full package, which is not vendored: SOURCES.md
// pins the release and its checksum, and the path to an unpacked copy is given
// here.
//
//	curl -sLO https://registry.npmjs.org/cldr-units-full/-/cldr-units-full-48.0.0.tgz
//	tar xzf cldr-units-full-48.0.0.tgz
//	go run ./internal/unitgen package
//
// Only the units ECMA-402 sanctions are written. CLDR carries 268 of them and
// ECMA-402 allows 45, chosen because every language has a name for each, so
// the rest would be carried for nothing. A unit may also be one of these
// divided by another, which is why the divisor patterns are kept too.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/unitdata"
)

// sanctioned maps ECMA-402's name for a unit to CLDR's, which carries the
// category it belongs to. The mapping is written out rather than guessed from
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

var widthNames = [unitdata.Widths]string{"long", "short", "narrow"}

type file struct {
	Main map[string]struct {
		Units map[string]json.RawMessage `json:"units"`
	} `json:"main"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/unitgen <cldr-units-full/package>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "unitgen:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	main := filepath.Join(root, "main")
	entries, err := os.ReadDir(main)
	if err != nil {
		return fmt.Errorf("reading %s: %w", main, err)
	}

	built := map[string][]byte{}
	// The locales share one pool, written beside them. ReadDir's order is
	// sorted, so the pool is the same every run.
	pool := blob.NewPool(unitdata.Version)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		l, err := read(main, e.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if l == nil {
			continue
		}
		built[e.Name()] = unitdata.Encode(l, pool)
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}

	out := filepath.Join("data", "units")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := datawrite.Locales(out, built); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "unitsshared.bin"), pool.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "unitgen: %d locales, %d units each\n", len(built), len(sanctioned))
	return nil
}

func read(main, name string) (*unitdata.Built, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, "units.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("units.json: %w", err)
	}
	entry, ok := f.Main[name]
	if !ok {
		return nil, nil
	}

	var out unitdata.Built
	any := false
	for i, width := range widthNames {
		body, ok := entry.Units[width]
		if !ok {
			continue
		}
		var byUnit map[string]map[string]string
		if err := json.Unmarshal(body, &byUnit); err != nil {
			return nil, fmt.Errorf("%s: %w", width, err)
		}
		w := &out.Widths[i]
		if per, ok := byUnit["per"]; ok {
			w.Compound = per["compoundUnitPattern"]
		}
		for short, key := range sanctioned {
			fields, ok := byUnit[key]
			if !ok {
				continue
			}
			u := unitdata.Unit{Name: short, PerUnit: fields["perUnitPattern"]}
			for field, text := range fields {
				count, ok := strings.CutPrefix(field, "unitPattern-count-")
				if !ok || text == "" {
					continue
				}
				u.Patterns = append(u.Patterns,
					unitdata.CountedText{Count: count, Text: text})
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
