package intl_test

import (
	"fmt"
	"slices"
	"testing"
	"unicode/utf16"

	intl "github.com/go-quickjs/go-intl"
)

// ICU opens its break-rule and collation trees for a break iterator's rules
// and a collator's tailoring with ures_openNoDefault, so a locale those
// trees have no bundle for reads the root's, whatever the default locale;
// the list of a locale's collations is read with ures_open, which reaches
// the default's. Neither Thai nor Akan has a bundle in either tree: a
// Chrome running in Greek breaks Thai sentences with the root's rules, not
// Greek's, which end one at a ";", and lists Korean's collations for Akan
// in a Chrome running in Korean, but sorts Akan as the root does.
func TestNoDefaultForRulesAndTailorings(t *testing.T) {
	locale := func(tag string) intl.Locale {
		l, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	with := func(tag string) intl.Source { return intl.WithDefaultLocale(intl.Embedded, locale(tag)) }

	text := utf16.Encode([]rune("\u039a\u03ac\u03c0\u03bf\u03c4\u03b5 \u03ae\u03c4\u03b1\u03bd \u03ad\u03bd\u03b1\u03c2 \u03b2\u03b1\u03c3\u03b9\u03bb\u03b9\u03ac\u03c2; \u03b6\u03bf\u03cd\u03c3\u03b5 \u03c3\u03b5 \u03ad\u03bd\u03b1 \u03c0\u03b1\u03bb\u03ac\u03c4\u03b9. \u03a4\u03ad\u03bb\u03bf\u03c2;"))
	for _, c := range []struct {
		def, tag, want string
	}{
		{"en-US", "th", "[0 48]"},
		{"el", "th", "[0 48]"},
		{"en-US", "el", "[0 27 48]"},
	} {
		s, err := intl.NewSegmenterFrom(with(c.def), locale(c.tag), intl.SegmenterOptions{Granularity: intl.GranularitySentence})
		if err != nil {
			t.Fatal(err)
		}
		var starts []int
		for _, seg := range s.Segment(text).All() {
			starts = append(starts, seg.Index)
		}
		if got := fmt.Sprint(starts); got != c.want {
			t.Errorf("default %s, %s: sentences start at %s, want %s", c.def, c.tag, got, c.want)
		}
	}

	sorted := func(def string) []string {
		col, err := intl.NewCollatorFrom(with(def), locale("ak"), intl.CollatorOptions{})
		if err != nil {
			t.Fatal(err)
		}
		words := []string{"\uac00", "\u52a0", "a", "Z", "\u6f22", "\ud55c", "\u00e9", "\u00df"}
		slices.SortStableFunc(words, col.Compare)
		return words
	}
	if en, ko := sorted("en-US"), sorted("ko-KR"); !slices.Equal(en, ko) {
		t.Errorf("Akan sorts as %+q with Korean the default, %+q with English", ko, en)
	}
	root, err := intl.NewCollatorFrom(intl.Embedded, intl.Locale{}, intl.CollatorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	words := []string{"\uac00", "\u52a0", "a", "Z", "\u6f22", "\ud55c", "\u00e9", "\u00df"}
	slices.SortStableFunc(words, root.Compare)
	if ak := sorted("ko-KR"); !slices.Equal(ak, words) {
		t.Errorf("Akan sorts as %+q, the root as %+q", ak, words)
	}

	for _, c := range []struct {
		def      string
		searchjl bool
	}{{"en-US", false}, {"ko-KR", true}} {
		info, err := intl.NewLocaleInfo(with(c.def), intl.LocaleInfoOptions{})
		if err != nil {
			t.Fatal(err)
		}
		list, err := info.Collations(locale("ak"))
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(list, "searchjl"); got != c.searchjl {
			t.Errorf("default %s: Akan lists %v", c.def, list)
		}
	}
}
