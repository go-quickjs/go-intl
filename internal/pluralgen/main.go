// Command pluralgen writes the plural-rule tables the intl package carries.
//
// It reads CLDR's plurals.json, ordinals.json and pluralRanges.json, vendored
// beside it from cldr-core, being a hundred and thirty kilobytes between them.
//
// Usage, from the repository root:
//
//	go run ./internal/pluralgen
//
// A CLDR rule carries its samples after the condition -- "i = 1 and v = 0
// @integer 1" -- which are there to be read and tested against rather than
// evaluated. They are dropped here. What is kept is the condition itself, as
// CLDR writes it: the rule is the input, and which category a number falls in
// is an answer, which no table holds.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/plurdata"
)

//go:embed plurals.json
var cardinalJSON []byte

//go:embed ordinals.json
var ordinalJSON []byte

//go:embed pluralRanges.json
var rangesJSON []byte

// categories are tried in this order, which is the order CLDR lists them and
// the order the rules assume: the first condition that holds wins, and "other"
// is last because it always holds.
var categories = []string{"zero", "one", "two", "few", "many", "other"}

type resource struct {
	Supplemental map[string]json.RawMessage `json:"supplemental"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pluralgen:", err)
		os.Exit(1)
	}
}

func run() error {
	cardinal, err := read(cardinalJSON, "plurals-type-cardinal")
	if err != nil {
		return err
	}
	ordinal, err := read(ordinalJSON, "plurals-type-ordinal")
	if err != nil {
		return err
	}

	locales := map[string]bool{}
	for name := range cardinal {
		locales[name] = true
	}
	for name := range ordinal {
		locales[name] = true
	}

	ranges, err := readRanges()
	if err != nil {
		return err
	}

	built := map[string][]byte{}
	for name := range locales {
		// ICU looks a locale's plural ranges up by its language alone.
		language, _, _ := strings.Cut(name, "-")
		built[name] = plurdata.Encode(&plurdata.Locale{
			Cardinal: cardinal[name],
			Ordinal:  ordinal[name],
			Ranges:   ranges[language],
		})
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found")
	}

	out := filepath.Join("data", "plurals")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := datawrite.Locales(out, built); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "pluralgen: %d locales\n", len(built))
	return nil
}

// readRanges reads the plural ranges, by language, sorted.
func readRanges() (map[string][]plurdata.Range, error) {
	var res struct {
		Supplemental struct {
			Plurals map[string]map[string]string `json:"plurals"`
		} `json:"supplemental"`
	}
	if err := json.Unmarshal(rangesJSON, &res); err != nil {
		return nil, fmt.Errorf("pluralRanges.json: %w", err)
	}
	out := map[string][]plurdata.Range{}
	for language, pairs := range res.Supplemental.Plurals {
		var list []plurdata.Range
		for key, result := range pairs {
			ends, ok := strings.CutPrefix(key, "pluralRange-start-")
			start, end, ok2 := strings.Cut(ends, "-end-")
			if !ok || !ok2 {
				return nil, fmt.Errorf("pluralRanges.json: %s: a key %q", language, key)
			}
			list = append(list, plurdata.Range{Start: start, End: end, Result: result})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Start != list[j].Start {
				return list[i].Start < list[j].Start
			}
			return list[i].End < list[j].End
		})
		out[language] = list
	}
	return out, nil
}

// read pulls one kind of rules out of a supplemental file.
func read(raw []byte, key string) (map[string][]plurdata.Rule, error) {
	var res resource
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	body, ok := res.Supplemental[key]
	if !ok {
		return nil, fmt.Errorf("no %q", key)
	}
	var byLocale map[string]map[string]string
	if err := json.Unmarshal(body, &byLocale); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}

	out := make(map[string][]plurdata.Rule, len(byLocale))
	for locale, rules := range byLocale {
		var set []plurdata.Rule
		for _, category := range categories {
			rule, ok := rules["pluralRule-count-"+category]
			if !ok {
				continue
			}
			set = append(set, plurdata.Rule{
				Category:  category,
				Condition: condition(rule),
			})
		}
		if len(set) == 0 {
			continue
		}
		out[locale] = set
	}
	return out, nil
}

// condition strips the samples, which begin at the first "@".
func condition(rule string) string {
	if at := strings.IndexByte(rule, '@'); at >= 0 {
		rule = rule[:at]
	}
	return strings.TrimSpace(rule)
}
