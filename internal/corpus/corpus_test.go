package corpus_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-quickjs/go-intl/internal/corpus"
)

const goldenPath = "../../testdata/intl_golden.txt"

func load(t *testing.T) *corpus.File {
	t.Helper()
	f, err := corpus.Load(filepath.FromSlash(goldenPath))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}
	return f
}

// Every line has to come apart. A line that does not is not a case that can be
// skipped: it is a shape the corpus uses and go-intl would never be held to.
func TestEveryCaseParses(t *testing.T) {
	f := load(t)
	if got, want := len(f.Cases), 7949; got != want {
		t.Errorf("parsed %d cases, want %d", got, want)
	}
	if got, want := f.ICU, "78.3"; got != want {
		t.Errorf("the corpus was produced by ICU %s, want %s -- the anchor in "+
			"SOURCES.md and the corpus have to be the same ICU", got, want)
	}
	// No case in the corpus formats to nothing, so an empty answer means a
	// line was taken apart wrongly rather than that ICU said nothing.
	for _, c := range f.Cases {
		if c.Want == "" {
			t.Errorf("line %d: %s: no expected answer", c.Line, c.Source)
		}
		if c.Method == "" {
			t.Errorf("line %d: %s: no method", c.Line, c.Source)
		}
		if c.Locale == "" {
			t.Errorf("line %d: %s: no locale", c.Line, c.Source)
		}
		if c.Service == "" && c.Kind != corpus.ToLocale {
			t.Errorf("line %d: %s: no service", c.Line, c.Source)
		}
		if len(c.Args) == 0 {
			t.Errorf("line %d: %s: no arguments", c.Line, c.Source)
		}
	}
}

// The counts are checked so that a corpus regenerated against a different ICU,
// or one that grew a shape this package cannot take apart, is noticed here
// rather than as a service quietly losing its coverage.
func TestCaseCounts(t *testing.T) {
	f := load(t)

	byService := map[string]int{}
	byKind := map[corpus.Kind]int{}
	for _, c := range f.Cases {
		byService[c.Service]++
		byKind[c.Kind]++
	}

	wantService := map[string]int{
		"NumberFormat":       2970,
		"Collator":           1805,
		"RelativeTimeFormat": 1260,
		"DateTimeFormat":     1230,
		"PluralRules":        300,
		"Segmenter":          140,
		"ListFormat":         120,
		"DisplayNames":       34,
		"":                   90, // the legacy toLocale methods name no service
	}
	for service, want := range wantService {
		if got := byService[service]; got != want {
			t.Errorf("service %q has %d cases, want %d", service, got, want)
		}
	}
	for service, got := range byService {
		if _, ok := wantService[service]; !ok {
			t.Errorf("unexpected service %q with %d cases", service, got)
		}
	}

	wantKind := map[corpus.Kind]int{
		corpus.Format:   5914,
		corpus.Sort:     1750,
		corpus.Segment:  140,
		corpus.ToLocale: 90,
		corpus.Compare:  55,
	}
	for kind, want := range wantKind {
		if got := byKind[kind]; got != want {
			t.Errorf("kind %v has %d cases, want %d", kind, got, want)
		}
	}
}

// One case of each shape, taken apart by hand, so that a change in the parser
// that still balances the counts is caught by what the pieces actually are.
func TestShapesDecode(t *testing.T) {
	f := load(t)

	byShape := map[corpus.Kind]*corpus.Case{}
	for _, c := range f.Cases {
		if _, seen := byShape[c.Kind]; !seen {
			byShape[c.Kind] = c
		}
	}
	for _, kind := range []corpus.Kind{corpus.Format, corpus.Compare,
		corpus.Sort, corpus.Segment, corpus.ToLocale} {
		if byShape[kind] == nil {
			t.Errorf("no case of kind %v", kind)
		}
	}

	var number, list, segment, sorted, compare *corpus.Case
	for _, c := range f.Cases {
		switch {
		case number == nil && c.Service == "NumberFormat" && len(c.Options) > 0:
			number = c
		case list == nil && c.Service == "ListFormat":
			list = c
		case segment == nil && c.Kind == corpus.Segment:
			segment = c
		case sorted == nil && c.Kind == corpus.Sort:
			sorted = c
		case compare == nil && c.Kind == corpus.Compare:
			compare = c
		}
	}

	if number != nil {
		if _, ok := number.Number(); !ok {
			t.Errorf("%s: no numeric argument", number.Source)
		}
		if number.Method != "format" {
			t.Errorf("%s: method is %q", number.Source, number.Method)
		}
	}
	if list != nil {
		if items, ok := list.Strings(); !ok || len(items) == 0 {
			t.Errorf("%s: no list argument", list.Source)
		}
	}
	if segment != nil {
		switch segment.Field {
		case "segment", "index", "isWordLike":
		default:
			t.Errorf("%s: reads field %q", segment.Source, segment.Field)
		}
		if segment.Join == "" {
			t.Errorf("%s: no join separator", segment.Source)
		}
	}
	if sorted != nil {
		items, ok := sorted.Strings()
		if !ok || len(items) < 2 {
			t.Errorf("%s: sorts %d items", sorted.Source, len(items))
		}
		if sorted.Service != "Collator" {
			t.Errorf("%s: sorts through %q", sorted.Source, sorted.Service)
		}
	}
	if compare != nil && len(compare.Args) != 2 {
		t.Errorf("%s: compares %d arguments", compare.Source, len(compare.Args))
	}
}

// The parser has to reject what it cannot represent rather than return a case
// with pieces quietly missing, which would be scored as a passing comparison.
func TestMalformedIsRejected(t *testing.T) {
	cases := []string{
		"new Intl.NumberFormat(\"en\").format(1)", // no tab
		"notACall(1)\t\"1\"",
		"new Intl.NumberFormat(\"en\", {).format(1)\t\"1\"",
		"new Intl.NumberFormat()\t\"1\"",
	}
	for _, src := range cases {
		if _, err := corpus.Read(strings.NewReader(src)); err == nil {
			t.Errorf("%q was accepted", src)
		}
	}
}
