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

// patternCalendars is the table of locales whose patterns ICU's pattern
// generator and interval formatter read from another calendar than the one
// the locale inherits as its default, when no calendar is asked for: a line
// each, the locale's tag and the calendar's BCP 47 name, "uz-AF gregory".
//
// ICU looks the calendar up with ures_getFunctionalEquivalent, which for an
// empty bundle can skip the parent that names the default (see
// icusrc.FunctionalDefault). The locales are those V8 can hand ICU: every
// bundle of the tree, and each one's form without its script, which V8 adds
// to its available locales (Intl::BuildLocaleSet).
func patternCalendars(icu *icusrc.Locales, fb icusrc.Fallback) ([]byte, error) {
	names := map[string]bool{}
	for _, n := range icu.Names() {
		if n == "root" {
			continue
		}
		names[n] = true
		// RemoveLocaleScriptTag: the language and the region.
		parts := strings.Split(n, "_")
		if len(parts) >= 2 && len(parts[1]) == 4 {
			short := parts[0]
			if len(parts) >= 3 && (len(parts[2]) == 2 || len(parts[2]) == 3) {
				short += "_" + parts[2]
			}
			names[short] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	var b strings.Builder
	for _, n := range sorted {
		cal, err := patternCalendar(icu, fb, n)
		if err != nil {
			return nil, err
		}
		if cal != "" {
			fmt.Fprintf(&b, "%s %s\n", strings.ReplaceAll(n, "_", "-"), cal)
		}
	}
	return []byte(b.String()), nil
}

// patternCalendar is the calendar ICU's pattern generator reads a locale's
// patterns from when none is asked for, by its BCP 47 name, where that is
// not the one the locale inherits as its default; "" everywhere else. The
// locale is ICU's name for it, "uz_Arab_AF".
func patternCalendar(icu *icusrc.Locales, fb icusrc.Fallback, icuName string) (string, error) {
	functional, err := icu.FunctionalDefault(icuName, "calendar", fb)
	if err != nil {
		return "", err
	}
	inherited, err := icu.Inherited(icuName, "calendar", fb)
	if err != nil {
		return "", err
	}
	if functional == "" {
		// The walk found no default, and the generator keeps the one it
		// starts with (getCalendarTypeToUse).
		functional = "gregorian"
	}
	if inherited == "" {
		inherited = "gregorian"
	}
	if functional == inherited {
		return "", nil
	}
	if functional == "gregorian" {
		return "gregory", nil
	}
	for _, x := range extras {
		if x.cldr == functional {
			return x.bcp47, nil
		}
	}
	return "", fmt.Errorf("ICU's pattern generator reads %s's patterns from %q, which has no BCP 47 name here", icuName, functional)
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
