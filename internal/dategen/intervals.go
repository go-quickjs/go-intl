package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// icuChain is a locale's bundles as ICU's ures_open resolves them, the root
// last.
func icuChain(icu *icusrc.Locales, fb icusrc.Fallback, name string) ([]*icutxt.Node, error) {
	icuName := strings.ReplaceAll(name, "-", "_")
	if name == "und" {
		icuName = "root"
	}
	return icu.Resolve(icuName, fb)
}

// intervalsFromICU reads a calendar's interval patterns as ICU's
// DateIntervalInfo loads them (dtitvinf.cpp). Each bundle up the chain adds
// the skeletons and fields the ones before it lacked, its keys taken in
// sorted order, as ICU's binary tables hold them; a calendar whose patterns
// are an alias to another's -- the Buddhist calendar's are the generic
// calendar's, which are the Gregorian's -- then goes on to that one's, up
// the whole chain again. The order skeletons are first stored in is kept,
// because ICU's choice between two equally good ones depends on it.
func intervalsFromICU(chain []*icutxt.Node, calendar string) (string, []datedata.Interval, error) {
	var out []datedata.Interval
	index := map[string]int{}
	seen := map[string]bool{}
	for cal := calendar; cal != ""; {
		if seen[cal] {
			return "", nil, fmt.Errorf("the interval formats of %s alias in a loop", calendar)
		}
		seen[cal] = true
		next := ""
		for _, n := range chain {
			formats := n.Get("calendar", cal, "intervalFormats")
			if formats == nil {
				continue
			}
			if formats.Alias {
				target, err := aliasCalendar(formats.Value)
				if err != nil {
					return "", nil, err
				}
				next = target
				continue
			}
			skeletons := append([]*icutxt.Node(nil), formats.Children...)
			sort.Slice(skeletons, func(i, j int) bool { return skeletons[i].Key < skeletons[j].Key })
			for _, sk := range skeletons {
				if !sk.Table {
					continue
				}
				fields := append([]*icutxt.Node(nil), sk.Children...)
				sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
				for _, f := range fields {
					field, ok := intervalField(f.Key)
					if !ok || f.Table || f.Alias {
						continue
					}
					i, ok := index[sk.Key]
					if !ok {
						i = len(out)
						index[sk.Key] = i
						out = append(out, datedata.Interval{Skeleton: sk.Key})
					}
					if out[i].Patterns[field] == "" {
						out[i].Patterns[field] = f.Value
					}
				}
			}
		}
		cal = next
	}
	return intervalFallback(chain, calendar), out, nil
}

// intervalFallback is the pattern ICU joins a range with when nothing better
// fits, as DateIntervalInfo::initializeData reads it: the calendar's
// "intervalFormats/fallback" up the chain, where the tables up the chain
// that are an alias to another calendar's go on to that calendar's, from
// the locale itself ("/LOCALE/"). English has Hebrew interval formats of
// its own but no fallback among them, and the root's are the generic
// calendar's, so English's generic fallback is the Hebrew calendar's.
func intervalFallback(chain []*icutxt.Node, calendar string) string {
	seen := map[string]bool{}
	for cal := calendar; cal != "" && !seen[cal]; {
		seen[cal] = true
		next := ""
		for _, n := range chain {
			formats := n.Get("calendar", cal, "intervalFormats")
			if formats == nil {
				continue
			}
			if formats.Alias {
				next, _ = aliasCalendar(formats.Value)
				break
			}
			if f := n.Get("calendar", cal, "intervalFormats", "fallback"); f != nil && f.Value != "" {
				return f.Value
			}
		}
		cal = next
	}
	return ""
}

// aliasCalendar reads the calendar out of an alias to another calendar's
// interval formats, "/LOCALE/calendar/generic/intervalFormats".
func aliasCalendar(path string) (string, error) {
	const prefix, suffix = "/LOCALE/calendar/", "/intervalFormats"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", fmt.Errorf("an interval formats alias to %q", path)
	}
	return path[len(prefix) : len(path)-len(suffix)], nil
}

// intervalField is DateIntervalSink::validateAndProcessPatternLetter: the
// field a pattern is keyed by. ICU takes the flexible day period for the
// day period, and ignores the seconds.
func intervalField(key string) (int, bool) {
	switch key {
	case "G":
		return datedata.IntervalEra, true
	case "y":
		return datedata.IntervalYear, true
	case "M":
		return datedata.IntervalMonth, true
	case "d":
		return datedata.IntervalDay, true
	case "a", "B":
		return datedata.IntervalDayPeriod, true
	case "h", "H":
		return datedata.IntervalHour, true
	case "m":
		return datedata.IntervalMinute, true
	}
	return 0, false
}

// dateTimeGlue is the Gregorian calendar's DateTimePatterns[8], the default
// glue, from the first bundle up the chain that has the patterns.
func dateTimeGlue(chain []*icutxt.Node) (string, error) {
	for _, n := range chain {
		p := n.Get("calendar", "gregorian", "DateTimePatterns")
		if p == nil {
			continue
		}
		if len(p.Values) <= 8 {
			return "", fmt.Errorf("the Gregorian DateTimePatterns have %d entries", len(p.Values))
		}
		return p.Values[8], nil
	}
	return "", nil
}
