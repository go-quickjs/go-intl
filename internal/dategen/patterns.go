package main

import (
	"fmt"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// dateTimePatterns are a calendar's style patterns as ICU's DateFormat and
// pattern generator read them: the calendar's DateTimePatterns from the
// first bundle up the chain that has them, and where that is an alias to
// another calendar's, "/LOCALE/calendar/generic/DateTimePatterns", that
// calendar's, looked up again from the locale itself.
//
// cldr-json resolves the root's alias to the generic calendar as the
// locale's own Gregorian patterns, where ICU finds the generic calendar's
// up the chain: British English writes a Coptic time with English's generic
// "h:mm a", not its Gregorian "HH:mm", and Danish a Chinese one with the
// root's colon, not Danish's full stop.
type dateTimePatterns struct {
	times, dates, glue [datedata.Lengths]string
	timeNums, dateNums [datedata.Lengths]string
}

func dateTimePatternsFromICU(chain []*icutxt.Node, calendar string) (*dateTimePatterns, error) {
	seen := map[string]bool{}
	for cal := calendar; ; {
		if seen[cal] {
			return nil, fmt.Errorf("the patterns of %s alias in a loop", calendar)
		}
		seen[cal] = true
		var found *icutxt.Node
		for _, n := range chain {
			if p := n.Get("calendar", cal, "DateTimePatterns"); p != nil {
				found = p
				break
			}
		}
		if found == nil {
			return nil, nil
		}
		if found.Alias {
			const prefix, suffix = "/LOCALE/calendar/", "/DateTimePatterns"
			if !strings.HasPrefix(found.Value, prefix) || !strings.HasSuffix(found.Value, suffix) {
				return nil, fmt.Errorf("a patterns alias to %q", found.Value)
			}
			cal = found.Value[len(prefix) : len(found.Value)-len(suffix)]
			continue
		}
		if len(found.Children) < 13 {
			return nil, fmt.Errorf("%s has %d patterns", cal, len(found.Children))
		}
		// Each is a pattern, or a pattern and its numbering override.
		read := func(i int) (string, string) {
			e := found.Children[i]
			if len(e.Values) >= 2 {
				return e.Values[0], e.Values[1]
			}
			return e.Value, ""
		}
		var out dateTimePatterns
		for i := 0; i < datedata.Lengths; i++ {
			out.times[i], out.timeNums[i] = read(i)
			out.dates[i], out.dateNums[i] = read(4 + i)
			out.glue[i], _ = read(9 + i)
		}
		return &out, nil
	}
}

// usePatterns puts ICU's style patterns in a calendar, where ICU has some.
func usePatterns(c *datedata.Calendar, p *dateTimePatterns) {
	if p == nil {
		return
	}
	c.TimeFormats, c.DateFormats, c.DateTimeFormats = p.times, p.dates, p.glue
	c.TimeNumbers, c.DateNumbers = p.timeNums, p.dateNums
}
