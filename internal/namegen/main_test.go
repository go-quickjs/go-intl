package main

import (
	"testing"

	"github.com/go-quickjs/go-intl/internal/namedata"
)

// fill does not depend on the order a map gives its keys in: two
// alternates of one width, where there is no plain name, are taken in the
// order alternates lists them, and a plain name is always the long one.
// It had taken whichever the map gave first, when "variant" and "menu"
// both stood for the long width (ISSUES.md RU-5).
func TestFillIsDeterministic(t *testing.T) {
	alts := []alternate{
		{"short", namedata.Short},
		{"menu", namedata.Short},
		{"narrow", namedata.Narrow},
	}
	source := map[string]string{
		"GB":              "United Kingdom",
		"GB-alt-short":    "UK",
		"GB-alt-menu":     "Britain",
		"XA-alt-menu":     "menu only",
		"XA-alt-narrow":   "narrow",
		"XB":              "plain",
		"XB-alt-variant":  "unread",
		"XC-alt-short":    "",
		"XC-alt-menu":     "the menu name",
		"YY-alt-short":    "first",
		"YY-alt-menu":     "second",
		"ZZ-alt-unknown":  "unread",
		"EMPTY":           "",
		"EMPTY-alt-short": "short for nothing",
	}
	want := map[int][]namedata.Entry{
		namedata.Long: {{Code: "GB", Name: "United Kingdom"}, {Code: "XB", Name: "plain"}},
		namedata.Short: {{Code: "EMPTY", Name: "short for nothing"}, {Code: "GB", Name: "UK"},
			{Code: "XA", Name: "menu only"}, {Code: "XC", Name: "the menu name"}, {Code: "YY", Name: "first"}},
		namedata.Narrow: {{Code: "XA", Name: "narrow"}},
	}
	for run := 0; run < 50; run++ {
		var out namedata.Built
		fill(&out, namedata.Region, source, alts)
		for w, entries := range want {
			got := out.Sets[namedata.Region*namedata.Widths+w].Entries
			if len(got) != len(entries) {
				t.Fatalf("run %d width %d: %v, want %v", run, w, got, entries)
			}
			for i := range got {
				if got[i] != entries[i] {
					t.Fatalf("run %d width %d: %v, want %v", run, w, got, entries)
				}
			}
		}
	}
}
