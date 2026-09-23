// Command pluralgen writes the plural-rule tables the intl package carries.
//
// It reads CLDR's plurals.json and ordinals.json, vendored beside it, being a
// hundred kilobytes between them.
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

	"github.com/go-quickjs/go-intl/internal/plurdata"
)

//go:embed plurals.json
var cardinalJSON []byte

//go:embed ordinals.json
var ordinalJSON []byte

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

	built := map[string][]byte{}
	for name := range locales {
		built[name] = plurdata.Encode(&plurdata.Locale{
			Cardinal: cardinal[name],
			Ordinal:  ordinal[name],
		})
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found")
	}

	out := filepath.Join("data", "plurals")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(out, "*.bin"))
	for _, name := range old {
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(built))
	for name := range built {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(out, name+".bin"), built[name], 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "pluralgen: %d locales\n", len(names))
	return nil
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
