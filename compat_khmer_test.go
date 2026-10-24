package intl_test

import (
	"fmt"
	"testing"
	"unicode/utf16"

	intl "github.com/go-quickjs/go-intl"
)

// TestKhmerCombining pins when the Khmer dictionary engine joins a run that
// is no word to the word before it. ICU, and so Node and the standard,
// joins it to a word of fewer than 3 code points; Chromium's patched engine
// to one of fewer than 10, which keeps the run of marks after the second
// word here in it. Thai, Lao and Burmese are left as ICU has them.
func TestKhmerCombining(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		text   string
		want   string
	}{
		{intl.Standard, "\u17ad\u178c\u1785\u17b9\u17bd\u17ca\u17ce\u17b6\u17c6\u17ce\u17cf\u17b9\u17bf\u179d\u17c2\u178d\u17a9\u1796", "[0 1 2 13 15]"},
		{intl.NodeICU, "\u17ad\u178c\u1785\u17b9\u17bd\u17ca\u17ce\u17b6\u17c6\u17ce\u17cf\u17b9\u17bf\u179d\u17c2\u178d\u17a9\u1796", "[0 1 2 13 15]"},
		{intl.NodeICU | intl.KhmerCombining, "\u17ad\u178c\u1785\u17b9\u17bd\u17ca\u17ce\u17b6\u17c6\u17ce\u17cf\u17b9\u17bf\u179d\u17c2\u178d\u17a9\u1796", "[0 1 13 15]"},
		// Lao, as Node 26.10 breaks it either way.
		{intl.NodeICU, "\u0e9a\u0ecd\u0ec8\u0ea1\u0eb5\u0e9a\u0eb1\u0e99\u0eab\u0eb2", "[0 5]"},
		{intl.NodeICU | intl.KhmerCombining, "\u0e9a\u0ecd\u0ec8\u0ea1\u0eb5\u0e9a\u0eb1\u0e99\u0eab\u0eb2", "[0 5]"},
	} {
		if got := wordStarts(t, c.compat, c.text); got != c.want {
			t.Errorf("%v %+q: %s, want %s", c.compat, c.text, got, c.want)
		}
	}
}

// wordStarts is where a Khmer word segmenter's segments begin.
func wordStarts(t *testing.T, compat intl.Compat, text string) string {
	t.Helper()
	loc, err := intl.ParseLocale("km")
	if err != nil {
		t.Fatal(err)
	}
	s, err := intl.NewSegmenter(loc, intl.SegmenterOptions{Granularity: intl.GranularityWord, Compat: compat})
	if err != nil {
		t.Fatal(err)
	}
	var starts []int
	for _, seg := range s.Segment(utf16.Encode([]rune(text))).All() {
		starts = append(starts, seg.Index)
	}
	return fmt.Sprint(starts)
}
