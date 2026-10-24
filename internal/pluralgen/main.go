// Command pluralgen writes the plural-rule tables the intl package carries.
//
// It reads ICU's plural rules and plural ranges, misc/plurals.txt and
// misc/pluralRanges.txt in the data archive SOURCES.md pins.
//
// Usage, from the repository root:
//
//	go run ./internal/pluralgen icu4c-78.3-data.zip
//
// A CLDR rule carries its samples after the condition -- "i = 1 and v = 0
// @integer 1" -- which are there to be read and tested against rather than
// evaluated. They are dropped here. What is kept is the condition itself, as
// CLDR writes it: the rule is the input, and which category a number falls in
// is an answer, which no table holds.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/plurdata"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

// categories are tried in this order, which is the order CLDR lists them and
// the order the rules assume: the first condition that holds wins, and "other"
// is last because it always holds.
var categories = []string{"zero", "one", "two", "few", "many", "other"}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/pluralgen <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "pluralgen:", err)
		os.Exit(1)
	}
}

func run(zip string) error {
	icu, err := icusrc.OpenLocales(zip)
	if err != nil {
		return err
	}
	defer icu.Close()
	plurals, err := icu.Misc("plurals")
	if err != nil {
		return err
	}
	metadata, err := icu.Misc("metadata")
	if err != nil {
		return err
	}
	languages := metadata.Get("alias", "language")
	if languages == nil {
		return fmt.Errorf("metadata.txt has no language aliases")
	}
	cardinal, err := read(plurals, "locales", languages)
	if err != nil {
		return err
	}
	ordinal, err := read(plurals, "locales_ordinals", languages)
	if err != nil {
		return err
	}
	rangeFile, err := icu.Misc("pluralRanges")
	if err != nil {
		return err
	}
	ranges, err := readRanges(rangeFile, languages)
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

	set := writeset.New()
	if err := datawrite.AddLocales(set, filepath.Join("data", "plurals"), built); err != nil {
		return err
	}
	if err := set.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "pluralgen: %d locales\n", len(built))
	return nil
}

// readRanges reads the plural ranges, by language, sorted: each language's
// set, a list of the categories of a range's start and end and the
// category of the range.
func readRanges(file *icutxt.Node, languages *icutxt.Node) (map[string][]plurdata.Range, error) {
	locales, rules := file.Get("locales"), file.Get("rules")
	if locales == nil || rules == nil {
		return nil, fmt.Errorf("pluralRanges.txt has no locales or rules")
	}
	out := map[string][]plurdata.Range{}
	for _, l := range locales.Children {
		if replaced(l.Key, locales, languages) {
			continue
		}
		set := rules.Get(l.Value)
		if set == nil {
			return nil, fmt.Errorf("pluralRanges.txt: %s's set %s is missing", l.Key, l.Value)
		}
		var list []plurdata.Range
		for _, r := range set.Children {
			if len(r.Values) != 3 {
				return nil, fmt.Errorf("pluralRanges.txt: %s: a range of %d", l.Value, len(r.Values))
			}
			list = append(list, plurdata.Range{Start: r.Values[0], End: r.Values[1], Result: r.Values[2]})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Start != list[j].Start {
				return list[i].Start < list[j].Start
			}
			return list[i].End < list[j].End
		})
		out[tag(l.Key)] = list
	}
	return out, nil
}

// read reads one kind of rules, a table of each locale's rule set, by
// locale.
func read(file *icutxt.Node, key string, languages *icutxt.Node) (map[string][]plurdata.Rule, error) {
	locales, rules := file.Get(key), file.Get("rules")
	if locales == nil || rules == nil {
		return nil, fmt.Errorf("plurals.txt has no %s or rules", key)
	}
	out := map[string][]plurdata.Rule{}
	for _, l := range locales.Children {
		if replaced(l.Key, locales, languages) {
			continue
		}
		set := rules.Get(l.Value)
		if set == nil {
			return nil, fmt.Errorf("plurals.txt: %s's set %s is missing", l.Key, l.Value)
		}
		var list []plurdata.Rule
		for _, category := range categories {
			if r := set.Get(category); r != nil {
				list = append(list, plurdata.Rule{Category: category, Condition: condition(r.Value)})
			}
		}
		if len(list) > 0 {
			out[tag(l.Key)] = list
		}
	}
	return out, nil
}

// replaced reports whether ICU lists a deprecated language code beside the
// one that replaces it, "in" beside "id": a tag is canonicalized before its
// data is looked up, so the deprecated one is never asked for.
func replaced(name string, locales, languages *icutxt.Node) bool {
	r := languages.Get(name, "replacement")
	return r != nil && locales.Get(r.Value) != nil
}

// tag is an ICU locale name as BCP 47 writes it: "pt_PT" is "pt-PT", and
// the root "und".
func tag(name string) string {
	if name == "root" {
		return "und"
	}
	return strings.ReplaceAll(name, "_", "-")
}

// condition strips the samples, which begin at the first "@".
func condition(rule string) string {
	if at := strings.IndexByte(rule, '@'); at >= 0 {
		rule = rule[:at]
	}
	return strings.TrimSpace(rule)
}
