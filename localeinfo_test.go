package intl_test

import (
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// A language the likely subtags do not know is not maximized, as ICU leaves
// it and Node answers: "gss" is "gss", not "gss-Latn-US" by way of "und",
// and what depends on its region is the world's. "und" itself is.
func TestMaximizeUnknownLanguage(t *testing.T) {
	info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for tag, want := range map[string]string{
		"gss": "gss", "xyz": "xyz", "xyz-DE": "xyz-DE", "xyz-Cyrl": "xyz-Cyrl", "qaa": "qaa",
		"und": "en-Latn-US", "und-DE": "de-Latn-DE", "und-Cyrl": "ru-Cyrl-RU", "zzj": "zzj-Hani-CN",
	} {
		l, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Maximize(l).String(); got != want {
			t.Errorf("%s maximizes to %s, want %s", tag, got, want)
		}
	}
	l, _ := intl.ParseLocale("gss")
	if got, err := info.HourCycles(l); err != nil || len(got) != 1 || got[0] != "h23" {
		t.Errorf("gss's hour cycles are %v, %v, want [h23]", got, err)
	}
	if first, _, err := info.WeekInfo(l); err != nil || first != 1 {
		t.Errorf("gss's week starts on %d, %v, want 1", first, err)
	}
}
