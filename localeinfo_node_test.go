package intl_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/go-quickjs/go-intl"
)

// TestLocaleInfoMatchesNode replays testdata/localeinfo_node.js: what
// Intl.Locale says of 690 locales, part by part.
func TestLocaleInfoMatchesNode(t *testing.T) {
	replay(t, "testdata/localeinfo_node.txt.gz", func(t *testing.T, r recording) {
		info, err := intl.NewLocaleInfo(r.src, intl.LocaleInfoOptions{Compat: r.compatFor(intl.NodeICU)})
		if err != nil {
			t.Fatal(err)
		}
		ran, matched := map[string]int{}, map[string]int{}
		diffs := map[string][]string{}
		for _, line := range r.lines {
			var c []json.RawMessage
			if err := json.Unmarshal([]byte(line), &c); err != nil || len(c) != 2 {
				t.Fatalf("a line that is not two fields: %s", line)
			}
			var tag string
			json.Unmarshal(c[0], &tag)
			var want map[string]json.RawMessage
			json.Unmarshal(c[1], &want)
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}
			got := localeInfoOf(info, loc)
			for part, w := range want {
				g, ok := got[part]
				if !ok {
					continue
				}
				gj, _ := json.Marshal(g)
				ran[part]++
				if string(gj) == string(w) {
					matched[part]++
				} else if len(diffs[part]) < 8 {
					diffs[part] = append(diffs[part], fmt.Sprintf("%s: got %s, want %s", tag, gj, w))
				}
			}
		}
		var parts []string
		for p := range ran {
			parts = append(parts, p)
		}
		sort.Strings(parts)
		for _, p := range parts {
			if matched[p] == ran[p] {
				t.Logf("%s: all %d match Node", p, ran[p])
				continue
			}
			t.Errorf("%s: %d of %d match Node:\n\t%s", p, matched[p], ran[p], strings.Join(diffs[p], "\n\t"))
		}
	})
}

// localeInfoOf answers each part the recording holds, as JSON would write
// it, null where Node answers undefined.
func localeInfoOf(info *intl.LocaleInfo, l intl.Locale) map[string]any {
	out := map[string]any{
		"maximize":  info.Maximize(l).String(),
		"minimize":  info.Minimize(l).String(),
		"calendars": info.Calendars(l),
	}
	if v, err := info.Collations(l); err == nil {
		out["collations"] = v
	}
	if v, err := info.HourCycles(l); err == nil {
		out["hourCycles"] = v
	}
	if v, err := info.NumberingSystems(l); err == nil {
		out["numberingSystems"] = v
	}
	if v, ok, err := info.TimeZones(l); err == nil {
		if ok {
			out["timeZones"] = v
		} else {
			out["timeZones"] = nil
		}
	}
	if rtl, err := info.RightToLeft(l); err == nil {
		dir := "ltr"
		if rtl {
			dir = "rtl"
		}
		out["textInfo"] = map[string]string{"direction": dir}
	}
	if first, weekend, err := info.WeekInfo(l); err == nil {
		out["weekInfo"] = struct {
			FirstDay int   `json:"firstDay"`
			Weekend  []int `json:"weekend"`
		}{first, weekend}
	}
	return out
}
