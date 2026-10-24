package intl_test

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// dateTimeGaps are cases the expectations hold go-intl to that it does not
// meet yet, each with the reason: of every recording, or of those whose
// maker begins with who, "Chrome". A gap all of whose cases start to pass
// is reported, so the entry goes.
var dateTimeGaps = []struct {
	who                   string
	locale                string
	zone, style, calendar string
	why                   string
}{
	{who: "Chrome", locale: "he", calendar: "hebrew",
		why: "Chrome's data has no Hebrew numerals, and ICU's SimpleDateFormat falls back to its default pattern"},
}

// dateTimeGap returns why a case is a known gap, if it is one.
func dateTimeGap(who, tag string, opts map[string]any) (string, bool) {
	for _, g := range dateTimeGaps {
		if !strings.HasPrefix(who, g.who) || g.locale != "" && g.locale != tag {
			continue
		}
		if g.calendar != "" && opts["calendar"] == g.calendar ||
			g.calendar == "" && opts["timeZone"] == g.zone && opts["timeZoneName"] == g.style {
			return g.why, true
		}
	}
	return "", false
}

// TestDateTimeFormatMatchesNode holds every locale with date data to what
// Node writes under a spread of styles and field sets, in two zones and two
// seasons. The expectations are written by testdata/datetime_node.js.
func TestDateTimeFormatMatchesNode(t *testing.T) {
	testDateTimeAgainstNode(t, "testdata/datetime_node.txt.gz")
}

// TestDateTimeZonesMatchNode holds the names of every zone Node knows, in
// every timeZoneName style, to Node's, in a handful of locales. The
// expectations are written by testdata/datetime_zones_node.js.
func TestDateTimeZonesMatchNode(t *testing.T) {
	testDateTimeAgainstNode(t, "testdata/datetime_zones_node.txt.gz")
}

// TestDateTimeCalendarsMatchNode holds every calendar Node supports to Node,
// in forty locales. The expectations are written by
// testdata/datetime_calendars_node.js.
func TestDateTimeCalendarsMatchNode(t *testing.T) {
	testDateTimeAgainstNode(t, "testdata/datetime_calendars_node.txt.gz")
}

// TestDateTimeCalendarsInEveryLocaleMatchNode holds every calendar but the
// Gregorian to Node in every locale whose date data is its own, more
// thinly (ISSUES.md TE-1). The expectations are written by
// testdata/datetime_calendars_all_node.js.
func TestDateTimeCalendarsInEveryLocaleMatchNode(t *testing.T) {
	testDateTimeAgainstNode(t, "testdata/datetime_calendars_all_node.txt.gz")
}

// TestDateTimeFeaturesMatchNode holds dayPeriod, fractionalSecondDigits and
// the six timeZoneName styles to Node, in every locale. The expectations are
// written by testdata/datetime_features_node.js.
func TestDateTimeFeaturesMatchNode(t *testing.T) {
	testDateTimeAgainstNode(t, "testdata/datetime_features_node.txt.gz")
}

func testDateTimeAgainstNode(t *testing.T, file string) {
	for _, r := range recordings(t, file) {
		t.Run(r.who, func(t *testing.T) { replayDateTimes(t, r) })
	}
}

func replayDateTimes(t *testing.T, r recording) {
	r.checkHeader(t)
	h := r.host(t)
	var ran, matched, gaps int
	gapCases, gapPasses := map[string]int{}, map[string]int{}
	var differences []string
	var lastKey string
	var last *intl.DateTimeFormat
	var lastErr error
	for _, line := range r.lines {
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 5 {
			t.Fatalf("a line that is not five fields: %s", line)
		}
		var tag, want, cycle string
		var opts map[string]any
		var when float64
		for i, dst := range []any{&tag, &opts, &when, &want, &cycle} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		ran++
		name := fmt.Sprintf("%s %s %v", tag, fields[1], int64(when))
		// The cases come two instants to a formatter, in a row.
		if key := tag + string(fields[1]); key != lastKey {
			o, err := dateTimeOptions(opts)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			o.Compat = r.compat
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			last, lastErr = intl.NewDateTimeFormatFrom(r.src, h.resolve(intl.ServiceDateTimeFormat, loc), o)
			lastKey = key
		}
		why, gap := dateTimeGap(r.who, tag, opts)
		if lastErr != nil && !gap {
			differences = append(differences, name+": "+lastErr.Error())
			continue
		}
		got := ""
		if lastErr == nil {
			got = last.Format(time.UnixMilli(int64(when)))
		}
		if gap {
			// Some cases of a gap pass by chance -- a locale that writes
			// the offset either way -- so a gap is over only when all do.
			ran--
			gaps++
			gapCases[why]++
			if got == want {
				gapPasses[why]++
			}
			continue
		}
		if got == want {
			matched++
			continue
		}
		differences = append(differences, fmt.Sprintf("%s\n   got  %+q\n   want %+q", name, got, want))
	}
	for why, n := range gapCases {
		if gapPasses[why] == n {
			differences = append(differences, fmt.Sprintf("all %d cases of %q pass, so it is no longer a gap", n, why))
		}
	}
	t.Logf("%d of %d match %s exactly; %d left out as known gaps", matched, ran, r.who, gaps)
	if len(differences) > 0 {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 && os.Getenv("ALLDIFF") == "" {
			shown = shown[:25]
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(shown, "\n"))
	}
}
