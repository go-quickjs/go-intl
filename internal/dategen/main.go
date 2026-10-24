// Command dategen writes the date tables the intl package carries.
//
// It reads ICU's locale bundles, data/locales/*.txt in the data archive
// SOURCES.md pins: each locale's calendars -- their names of months, days,
// day periods and eras, their patterns, intervals and skeletons -- and its
// fields' names, as ICU resolves them.
//
//	go run ./internal/dategen [-seed pool.bin] icu4c-78.3-data.zip
//
// -seed numbers the pool's parts after an earlier pool's (datawrite.NewPool).
//
// What is a property of a region rather than a locale -- the calendars it
// reckons in, its hour cycles and its weeks -- is the archive's
// misc/supplementalData.txt, and the day periods' rules are its
// misc/dayPeriods.txt; each is written out as its own table.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/layout"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

// appendFields are ICU's names for the append items, by pattern-generator
// field. The fields with no append item of their own are empty.
var appendFields = [datedata.Fields]string{
	"Era", "Year", "Quarter", "Month", "Week", "", "Day-Of-Week",
	"", "", "Day", "", "Hour", "Minute", "Second", "", "Timezone",
}

// fieldNames are ICU's names for the fields, by pattern-generator field.
var fieldNames = [datedata.Fields]string{
	"era", "year", "quarter", "month", "week", "weekOfMonth", "weekday",
	"dayOfYear", "weekdayOfMonth", "day", "dayperiod", "hour", "minute", "second", "", "zone",
}

// extras are the calendars beyond the Gregorian one, in the order they are
// stored. ICU's name for a calendar is not always BCP-47's, which is the
// name ECMA-402 uses and the one stored.
var extras = []struct{ icu, bcp47 string }{
	{"buddhist", "buddhist"},
	{"persian", "persian"},
	{"coptic", "coptic"},
	{"ethiopic", "ethiopic"},
	{"ethiopic-amete-alem", "ethioaa"},
	{"indian", "indian"},
	{"islamic-civil", "islamic-civil"},
	{"islamic-tbla", "islamic-tbla"},
	{"islamic", "islamic"},
	{"islamic-umalqura", "islamic-umalqura"},
	{"islamic-rgsa", "islamic-rgsa"},
	{"roc", "roc"},
	{"hebrew", "hebrew"},
	{"japanese", "japanese"},
	{"chinese", "chinese"},
	{"dangi", "dangi"},
}

// widthNames are ICU's names for the widths, in storage order.
var widthNames = [datedata.Widths]string{"wide", "abbreviated", "narrow", "short"}

// contextNames are ICU's names for the two contexts, in storage order.
var contextNames = [datedata.Contexts]string{"format", "stand-alone"}

// amPmMarkers are ICU's arrays of the two halves of the day, by width, each
// the first of the arrays named that the calendar has; the short width has
// none. ICU 78's DateFormatSymbols reads each width's own; ICU before it
// read only AmPmMarkers, falling back to AmPmMarkersAbbr, for every width
// but the narrow.
var amPmMarkers = [datedata.Widths][]string{
	{"AmPmMarkers"}, {"AmPmMarkersAbbr"}, {"AmPmMarkersNarrow"}, nil,
}

var amPmMarkersBefore78 = [datedata.Widths][]string{
	{"AmPmMarkers", "AmPmMarkersAbbr"}, {"AmPmMarkers", "AmPmMarkersAbbr"},
	{"AmPmMarkersNarrow", "AmPmMarkersAbbr"}, nil,
}

// A tree is what dategen reads of the sources as a whole, which decides how
// it reads each locale: whether the ISO 8601 calendar has era rules, and
// which arrays the halves of the day are read from.
type tree struct {
	isoEras bool
	amPm    [datedata.Widths][]string
	// supplemental is misc/supplementalData.txt.
	supplemental *icutxt.Node
	// periods are the day-period rules by locale tag (periodRules).
	periods map[string][]datedata.PeriodRule
}

// readTree reads it. The ICU version is the sources' misc/icuver.txt.
func readTree(icu *icusrc.Locales) (*tree, error) {
	supplemental, err := icu.Misc("supplementalData")
	if err != nil {
		return nil, err
	}
	version, err := icu.Misc("icuver")
	if err != nil {
		return nil, err
	}
	v := version.Get("ICUVersion")
	if v == nil {
		return nil, fmt.Errorf("icuver.txt names no ICUVersion")
	}
	major, err := strconv.Atoi(strings.Split(v.Value, ".")[0])
	if err != nil {
		return nil, fmt.Errorf("icuver.txt: the ICU version %q", v.Value)
	}
	// An ICU before 78 reckons the ISO 8601 calendar as the Gregorian one,
	// eras and all; ICU 78's has its own, whose eras need their rules.
	t := &tree{isoEras: major < 78 || supplemental.Get("calendarData", "iso8601", "eras") != nil, amPm: amPmMarkers,
		supplemental: supplemental}
	if major < 78 {
		t.amPm = amPmMarkersBefore78
	}
	if t.periods, err = readPeriodRules(icu); err != nil {
		return nil, err
	}
	return t, nil
}

// eraWidths are ICU's era name sets, in storage order: the short width
// writes the narrow names.
var eraWidths = [datedata.Widths]string{"wide", "abbreviated", "narrow", "narrow"}

func main() {
	seed := flag.String("seed", "", "a pool to number the parts after")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/dategen [-seed pool.bin] <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	icu, err := icusrc.OpenLocales(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "dategen:", err)
		os.Exit(1)
	}
	defer icu.Close()
	if err := run(icu, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "dategen:", err)
		os.Exit(1)
	}
}

func run(icu *icusrc.Locales, seed string) error {
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}
	// The locales share one pool of strings and lists, written beside
	// them. EachLocale visits the bundles in order, so the pool is the same
	// every run.
	pool, err := datawrite.NewPool(datedata.Version, seed)
	if err != nil {
		return err
	}
	t, err := readTree(icu)
	if err != nil {
		return err
	}
	built, err := icusrc.EachLocale(icu, fb, func(v *icusrc.View) ([]byte, error) {
		l, err := readLocale(icu, fb, v, t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", v.Name(), err)
		}
		return datedata.Encode(l, pool), nil
	})
	if err != nil {
		return err
	}

	prefs, err := calendarPreferences(t.supplemental)
	if err != nil {
		return err
	}
	patterns, err := patternCalendars(icu, fb)
	if err != nil {
		return err
	}
	hours, err := timeData(t.supplemental)
	if err != nil {
		return err
	}
	week, err := weekData(t.supplemental)
	if err != nil {
		return err
	}
	eras, err := japaneseEras(icu)
	if err != nil {
		return err
	}
	umm, err := ummAlQura()
	if err != nil {
		return err
	}

	set := writeset.New()
	if err := datawrite.AddLocales(set, filepath.Join("data", "dates"), built); err != nil {
		return err
	}
	set.File(filepath.Join("data", "datesshared.bin"), pool.Bytes())
	set.File(filepath.Join("data", "calendarprefs.bin"), prefs)
	set.File(filepath.Join("data", "patterncalendars.bin"), patterns)
	set.File(filepath.Join("data", "japaneseeras.bin"), eras)
	set.File(filepath.Join("data", "ummalqura.bin"), umm)
	set.File(filepath.Join("data", "weekdata.bin"), week)
	set.File(filepath.Join("data", "timedata.bin"), hours)
	if err := set.Commit(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "dategen: %d locales, %d calendars\n", len(built), len(extras)+2)
	return nil
}

// readLocale builds one locale's dates: the Gregorian calendar, the others
// in extras' order, and the ISO calendar last, whose eras depend on whether
// the supplemental data gives it era rules (isoCalendar).
func readLocale(icu *icusrc.Locales, fb icusrc.Fallback, v *icusrc.View, t *tree) (*datedata.Locale, error) {
	name := v.Name()
	chain, err := icu.Resolve(name, fb)
	if err != nil {
		return nil, err
	}
	l := &datedata.Locale{PeriodRules: t.periodRules(icusrc.Tag(name))}
	if l.FieldNames, err = readFieldNames(v); err != nil {
		return nil, err
	}
	if l.DateTimeGlue, err = dateTimeGlue(chain); err != nil {
		return nil, err
	}
	calendar := func(cal string) (*datedata.Calendar, error) {
		c, err := readCalendar(v, cal, t)
		if err != nil {
			return nil, err
		}
		if c.AtTimeFormats, err = atTimeFromICU(icu, name, cal); err != nil {
			return nil, err
		}
		if c.IntervalFallback, c.Intervals, err = intervalsFromICU(chain, cal); err != nil {
			return nil, err
		}
		if c.Available, err = availableFromICU(chain, cal); err != nil {
			return nil, err
		}
		patterns, err := dateTimePatternsFromICU(chain, cal)
		if err != nil {
			return nil, err
		}
		usePatterns(c, patterns)
		return c, nil
	}
	greg, err := calendar("gregorian")
	if err != nil {
		return nil, err
	}
	l.Calendars = append(l.Calendars, datedata.NamedCalendar{Name: "gregory", Calendar: *greg})
	iso := isoCalendar(*greg, chain, t.isoEras)
	// Data before ICU 76's, CLDR 46's, has no ISO 8601 calendar. ICU reads
	// the Gregorian one's names and styles for it (CalendarData's
	// fallback), but its pattern generator and its intervals read the
	// calendar ures_getFunctionalEquivalent finds, the locale's default:
	// Persian's skeletons in Persian.
	formats := "iso8601"
	if !hasCalendar(chain, "iso8601") {
		formats = defaultCalendar(chain)
	}
	if formats != "gregorian" {
		if iso.IntervalFallback, iso.Intervals, err = intervalsFromICU(chain, formats); err != nil {
			return nil, fmt.Errorf("iso8601: %w", err)
		}
		if iso.Available, err = availableFromICU(chain, formats); err != nil {
			return nil, fmt.Errorf("iso8601: %w", err)
		}
	}
	for _, e := range extras {
		c, err := calendar(e.icu)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.icu, err)
		}
		l.Calendars = append(l.Calendars, datedata.NamedCalendar{Name: e.bcp47, Calendar: *c})
	}
	l.Calendars = append(l.Calendars, datedata.NamedCalendar{Name: "iso8601", Calendar: *iso})
	return l, nil
}

// defaultCalendar is the calendar a locale defaults to: the first
// calendar/default up its chain, the root's "gregorian" last.
func defaultCalendar(chain []*icutxt.Node) string {
	for _, n := range chain {
		if d := n.Get("calendar", "default"); d != nil && d.Value != "" {
			return d.Value
		}
	}
	return "gregorian"
}

// hasCalendar reports whether a bundle of the chain has the calendar.
func hasCalendar(chain []*icutxt.Node, cal string) bool {
	for _, n := range chain {
		if n.Get("calendar", cal) != nil {
			return true
		}
	}
	return false
}

// calendarPreferences writes the calendars each region reckons in, most
// preferred first, as lines of "region calendar...", sorted by region: the
// first is the one it defaults to, and the whole list is what
// Intl.Locale.prototype.getCalendars answers. They are supplementalData.txt's
// calendarPreferenceData.
func calendarPreferences(supplemental *icutxt.Node) ([]byte, error) {
	data := supplemental.Get("calendarPreferenceData")
	if data == nil || len(data.Children) == 0 {
		return nil, fmt.Errorf("supplementalData.txt has no calendarPreferenceData")
	}
	var lines []string
	for _, region := range data.Children {
		list := texts(region)
		if len(list) == 0 && region.Value != "" {
			list = []string{region.Value}
		}
		if len(list) == 0 {
			continue
		}
		names := make([]string, len(list))
		for i, name := range list {
			// ICU writes some calendars' names in full where BCP-47
			// shortens them, and the short ones are what ECMA-402 uses.
			switch name {
			case "gregorian":
				name = "gregory"
			case "ethiopic-amete-alem":
				name = "ethioaa"
			}
			names[i] = name
		}
		lines = append(lines, region.Key+" "+strings.Join(names, " "))
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// readFieldNames reads what a locale calls each field.
func readFieldNames(v *icusrc.View) ([datedata.Fields]string, error) {
	var out [datedata.Fields]string
	for i, key := range fieldNames {
		if key == "" {
			continue
		}
		dn, err := v.Get("fields", key, "dn")
		if err != nil {
			return out, err
		}
		if dn != nil {
			out[i] = dn.Value
		}
	}
	return out, nil
}

// timeData writes the hour-cycle preferences, which are a property of a
// region, or of a language in a region, as an index (blob.Index) read where
// it lies: by "de_AT" or "AT", the preferred cycle, a space and the allowed
// ones separated by commas, as CLDR spells them ("H H,hB"). They are
// supplementalData.txt's timeData.
func timeData(supplemental *icutxt.Node) ([]byte, error) {
	data := supplemental.Get("timeData")
	if data == nil || len(data.Children) == 0 {
		return nil, fmt.Errorf("supplementalData.txt has no timeData")
	}
	records := map[string][]byte{}
	for _, key := range data.Children {
		preferred, allowed := key.Get("preferred"), texts(key.Get("allowed"))
		if a := key.Get("allowed"); a != nil && len(allowed) == 0 && a.Value != "" {
			allowed = []string{a.Value}
		}
		if preferred == nil || len(allowed) == 0 {
			return nil, fmt.Errorf("supplementalData.txt: timeData %s", key.Key)
		}
		records[key.Key] = []byte(preferred.Value + " " + strings.Join(allowed, ","))
	}
	return blob.BuildIndex(layout.TimeData, records)
}

// japaneseEras writes the start dates of the Japanese calendar's eras, from
// ICU's supplemental data, as EraRules reads them: one line each, the era's
// number and its first day, "236 2019 5 1". Tentative eras, which have no
// name yet, are left out, as ICU leaves them out unless told otherwise.
func japaneseEras(icu *icusrc.Locales) ([]byte, error) {
	root, err := icu.Misc("supplementalData")
	if err != nil {
		return nil, err
	}
	eras := root.Get("calendarData", "japanese", "eras")
	if eras == nil {
		return nil, fmt.Errorf("supplementalData.txt has no Japanese eras")
	}
	type era struct{ n, y, m, d int }
	var list []era
	for _, e := range eras.Children {
		if named := e.Get("named"); named != nil && named.Value == "false" {
			continue
		}
		start := e.Get("start")
		n, err := strconv.Atoi(e.Key)
		if start == nil || len(start.Values) != 3 || err != nil {
			return nil, fmt.Errorf("supplementalData.txt: Japanese era %s", e.Key)
		}
		var ymd [3]int
		for i, v := range start.Values {
			if ymd[i], err = strconv.Atoi(v); err != nil {
				return nil, fmt.Errorf("supplementalData.txt: Japanese era %s", e.Key)
			}
		}
		list = append(list, era{n, ymd[0], ymd[1], ymd[2]})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n < list[j].n })
	var out strings.Builder
	for _, e := range list {
		fmt.Fprintf(&out, "%d %d %d %d\n", e.n, e.y, e.m, e.d)
	}
	return []byte(out.String()), nil
}

// ummAlQura writes the Umm al-Qura calendar's tables from ICU's source, one
// line a year: the year, the mask of its thirty-day months, and the
// correction to its estimated first day, "1300 2730 0".
func ummAlQura() ([]byte, error) {
	u, err := icusrc.ReadUmmAlQura()
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	for i := range u.Months {
		fmt.Fprintf(&out, "%d %d %d\n", u.First+i, u.Months[i], u.Fixes[i])
	}
	return []byte(out.String()), nil
}

// weekData writes the week conventions, which are a property of a region:
// one line each, the field, the region and the value, sorted. The days are
// numbered from Sunday, zero. "001" is the world's. They are
// supplementalData.txt's weekData, ICU's six numbers for each region: the
// first day and the minimal days of the first week, then the day and time
// the weekend begins and the day and time it ends, the days counted from
// Sunday, one.
func weekData(supplemental *icutxt.Node) ([]byte, error) {
	data := supplemental.Get("weekData")
	if data == nil || len(data.Children) == 0 {
		return nil, fmt.Errorf("supplementalData.txt has no weekData")
	}
	read := func(region *icutxt.Node) ([4]int, error) {
		if len(region.Values) != 6 {
			return [4]int{}, fmt.Errorf("supplementalData.txt: weekData %s has %d numbers", region.Key, len(region.Values))
		}
		var n [6]int
		for i, v := range region.Values {
			var err error
			if n[i], err = strconv.Atoi(v); err != nil {
				return [4]int{}, fmt.Errorf("supplementalData.txt: weekData %s: %w", region.Key, err)
			}
		}
		return [4]int{n[0] - 1, n[1], n[2] - 1, n[4] - 1}, nil
	}
	world := data.Get("001")
	if world == nil {
		return nil, fmt.Errorf("supplementalData.txt has no weekData for 001")
	}
	def, err := read(world)
	if err != nil {
		return nil, err
	}
	fields := [4]string{"firstDay", "minDays", "weekendStart", "weekendEnd"}
	var lines []string
	for _, region := range data.Children {
		// ICU adds variants such as "GB_alt_variant", which no region is.
		if strings.Contains(region.Key, "_") {
			continue
		}
		v, err := read(region)
		if err != nil {
			return nil, err
		}
		// A region is written where it differs from the world.
		for i, name := range fields {
			if v[i] != def[i] || region.Key == "001" {
				lines = append(lines, name+" "+region.Key+" "+strconv.Itoa(v[i]))
			}
		}
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// readPeriodRules reads ICU's day-period rules, misc/dayPeriods.txt's
// "locales" and "rules", the rules each locale formats with, by its tag.
func readPeriodRules(icu *icusrc.Locales) (map[string][]datedata.PeriodRule, error) {
	file, err := icu.Misc("dayPeriods")
	if err != nil {
		return nil, err
	}
	locales, rules := file.Get("locales"), file.Get("rules")
	if locales == nil || rules == nil {
		return nil, fmt.Errorf("dayPeriods.txt has no locales or rules")
	}
	out := map[string][]datedata.PeriodRule{}
	for _, l := range locales.Children {
		set := rules.Get(l.Value)
		if set == nil {
			return nil, fmt.Errorf("dayPeriods.txt: %s's set %s is missing", l.Key, l.Value)
		}
		list := make([]datedata.PeriodRule, 0, len(set.Children))
		for _, rule := range set.Children {
			r := datedata.PeriodRule{ID: rule.Key}
			if at := rule.Get("at"); at != nil {
				r.At, r.Point = minutes(at.Value), true
			} else {
				from, before := rule.Get("from"), rule.Get("before")
				if from == nil || before == nil {
					continue
				}
				r.From, r.Before = minutes(from.Value), minutes(before.Value)
			}
			list = append(list, r)
		}
		// The ranges come first so that a language naming both a range
		// over midnight and the moment itself is written with the range,
		// which is what ICU does.
		sort.Slice(list, func(i, j int) bool {
			if list[i].Point != list[j].Point {
				return !list[i].Point
			}
			return list[i].ID < list[j].ID
		})
		out[icusrc.Tag(l.Key)] = list
	}
	return out, nil
}

// periodRules returns the rules that say which part of the day an hour
// falls in. ICU keeps them per language rather than per locale, so a
// locale with no rules of its own takes its language's.
func (t *tree) periodRules(name string) []datedata.PeriodRule {
	if set, ok := t.periods[name]; ok {
		return set
	}
	language, _, _ := strings.Cut(name, "-")
	return t.periods[language]
}

// minutes reads a time of day as minutes past midnight. CLDR writes the end of
// the day as "24:00".
func minutes(s string) int {
	hour, minute, ok := strings.Cut(s, ":")
	if !ok {
		return 0
	}
	h, _ := strconv.Atoi(hour)
	m, _ := strconv.Atoi(minute)
	return h*60 + m
}

// readCalendar reads one calendar's names: of the months, the days, the
// day periods and the eras, the leap-month patterns, the cyclic years, and
// the append items. Its patterns are read with the chain (usePatterns).
//
// What a calendar does not have, ICU's CalendarData reads from the
// locale's Gregorian calendar, as CLDR's root aliases it there.
func readCalendar(v *icusrc.View, cal string, t *tree) (*datedata.Calendar, error) {
	get := func(path ...string) (*icutxt.Node, error) {
		n, err := v.Get(append([]string{"calendar", cal}, path...)...)
		if n != nil || err != nil || cal == "gregorian" {
			return n, err
		}
		return v.Get(append([]string{"calendar", "gregorian"}, path...)...)
	}
	var c datedata.Calendar
	for ctx := 0; ctx < datedata.Contexts; ctx++ {
		for w := 0; w < datedata.Widths; w++ {
			at := ctx*datedata.Widths + w
			// Twelve months, or thirteen in the Coptic and Ethiopic
			// calendars, and fourteen in the Hebrew, whose Adar in a leap
			// year ICU keeps last.
			months, err := get("monthNames", contextNames[ctx], widthNames[w])
			if err != nil {
				return nil, err
			}
			if months != nil {
				c.Months[at].Text = texts(months)
			}
			days, err := get("dayNames", contextNames[ctx], widthNames[w])
			if err != nil {
				return nil, err
			}
			if days != nil {
				c.Days[at].Text = texts(days)
			}
		}
	}
	for w := 0; w < datedata.Widths; w++ {
		for _, name := range t.amPm[w] {
			marks, err := get(name)
			if err != nil {
				return nil, err
			}
			if m := texts(marks); len(m) == 2 {
				c.AM[w], c.PM[w] = m[0], m[1]
				break
			}
		}
		// The finer parts of the day are kept too, for the patterns that
		// ask for them rather than for the two halves.
		periods, err := dayPeriods(get, w)
		if err != nil {
			return nil, err
		}
		c.Periods[w] = periods
		// Two eras in most calendars, one in some, and 237 in the
		// Japanese, each under its number.
		eras, err := get("eras", eraWidths[w])
		if err != nil {
			return nil, err
		}
		if eras != nil {
			count := 0
			for _, e := range eras.Children {
				if n, err := strconv.Atoi(e.Key); err == nil && n+1 > count {
					count = n + 1
				}
			}
			text := make([]string, count)
			for _, e := range eras.Children {
				if n, err := strconv.Atoi(e.Key); err == nil {
					text[n] = e.Value
				}
			}
			c.Eras[w].Text = text
		}
	}
	monthPatterns, err := get("monthPatterns")
	if err != nil {
		return nil, err
	}
	c.LeapMonthPatterns = leapMonthPatterns(patternSets(monthPatterns))
	years, err := get("cyclicNameSets", "years", "format", "abbreviated")
	if err != nil {
		return nil, err
	}
	if years != nil {
		c.CyclicYears = texts(years)
	}
	items, err := get("appendItems")
	if err != nil {
		return nil, err
	}
	if items != nil {
		for i, key := range appendFields {
			if key != "" {
				if n := items.Get(key); n != nil {
					c.AppendItems[i] = n.Value
				}
			}
		}
	}
	return &c, nil
}

// dayPeriods reads the parts of the day at a width. A wide or narrow name
// the width lacks is the abbreviated one, as SimpleDateFormat takes it and
// CLDR's root aliases it; the short width has none.
func dayPeriods(get func(...string) (*icutxt.Node, error), w int) ([]datedata.DayPeriod, error) {
	if widthNames[w] == "short" {
		return nil, nil
	}
	byID := map[string]string{}
	sets := []string{widthNames[w]}
	if widthNames[w] != "abbreviated" {
		sets = append(sets, "abbreviated")
	}
	for _, width := range sets {
		periods, err := get("dayPeriod", "format", width)
		if err != nil {
			return nil, err
		}
		if periods == nil {
			continue
		}
		for _, p := range periods.Children {
			if p.Value == "" || p.Table || p.Key == "am" || p.Key == "pm" {
				continue
			}
			if _, ok := byID[p.Key]; !ok {
				byID[p.Key] = p.Value
			}
		}
	}
	out := make([]datedata.DayPeriod, 0, len(byID))
	for id, text := range byID {
		out = append(out, datedata.DayPeriod{ID: id, Text: text})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out, nil
}

// patternSets is a table of month patterns by context and width, as
// leapMonthPatterns takes them.
func patternSets(n *icutxt.Node) map[string]map[string]map[string]string {
	out := map[string]map[string]map[string]string{}
	if n == nil {
		return out
	}
	for _, ctx := range n.Children {
		out[ctx.Key] = map[string]map[string]string{}
		for _, w := range ctx.Children {
			set := map[string]string{}
			for _, p := range w.Children {
				set[p.Key] = p.Value
			}
			out[ctx.Key][w.Key] = set
		}
	}
	return out
}

// texts is an array's strings, or a table's in order: ICU writes the
// names of the months and days as arrays.
func texts(n *icutxt.Node) []string {
	if n == nil {
		return nil
	}
	if len(n.Values) > 0 {
		return append([]string(nil), n.Values...)
	}
	out := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		out = append(out, c.Value)
	}
	return out
}

// atTimeFromICU is a calendar's date-and-time glue as ICU looks it up: the
// first bundle up the locale's chain, the root included, that gives the
// calendar one, and for a calendar none gives one to, the Gregorian glue
// looked up the same way. A locale no bundle gives one to has none, and is
// joined with its plain glue.
//
// cldr-json gets this wrong twice over. It resolves CLDR's root alias for a
// calendar's glue to the root's value, so Arabic's Buddhist dates took a
// Latin comma; and where a locale overrides only the plain glue, it derives
// the atTime glue from that rather than inheriting its parent's, so French
// in Mali took a comma that French does not write.
func atTimeFromICU(icu *icusrc.Locales, name, calendar string) ([datedata.Lengths]string, error) {
	var out [datedata.Lengths]string
	chain, err := icu.Chain(strings.ReplaceAll(name, "-", "_"))
	if err != nil {
		return out, err
	}
	root, err := icu.Get("root")
	if err != nil {
		return out, err
	}
	chain = append(chain, root)
	for _, cal := range []string{calendar, "gregorian"} {
		for _, n := range chain {
			glue := n.Get("calendar", cal, "DateTimePatterns%atTime")
			if glue == nil {
				continue
			}
			if len(glue.Values) != datedata.Lengths {
				return out, fmt.Errorf("%s's atTime glue has %d patterns", cal, len(glue.Values))
			}
			for i, v := range glue.Values {
				out[i] = v
			}
			return out, nil
		}
	}
	// None anywhere, the root included: ICU then joins with the plain glue,
	// and so does the reader, finding this empty.
	return out, nil
}

// leapMonthPatterns reads a calendar's leap-month patterns as
// DateFormatSymbols::initializeData does: all seven sets must be there, or
// there are none; a set without a "leap" pattern gives an empty one; and
// the empty ones are then filled from their neighbours, in ICU's order,
// which ICU calls a hack for the Korean calendar's inheritance.
func leapMonthPatterns(patterns map[string]map[string]map[string]string) [datedata.LeapPatterns]string {
	var out [datedata.LeapPatterns]string
	paths := [datedata.LeapPatterns][2]string{
		datedata.LeapFormatWide:            {"format", "wide"},
		datedata.LeapFormatAbbreviated:     {"format", "abbreviated"},
		datedata.LeapFormatNarrow:          {"format", "narrow"},
		datedata.LeapStandAloneWide:        {"stand-alone", "wide"},
		datedata.LeapStandAloneAbbreviated: {"stand-alone", "abbreviated"},
		datedata.LeapStandAloneNarrow:      {"stand-alone", "narrow"},
		datedata.LeapNumeric:               {"numeric", "all"},
	}
	for i, path := range paths {
		set, ok := patterns[path[0]][path[1]]
		if !ok {
			return [datedata.LeapPatterns]string{}
		}
		out[i] = set["leap"]
	}
	if out[datedata.LeapFormatAbbreviated] == "" {
		out[datedata.LeapFormatAbbreviated] = out[datedata.LeapFormatWide]
	}
	if out[datedata.LeapFormatNarrow] == "" {
		out[datedata.LeapFormatNarrow] = out[datedata.LeapStandAloneNarrow]
	}
	if out[datedata.LeapStandAloneWide] == "" {
		out[datedata.LeapStandAloneWide] = out[datedata.LeapFormatWide]
	}
	if out[datedata.LeapStandAloneAbbreviated] == "" {
		out[datedata.LeapStandAloneAbbreviated] = out[datedata.LeapFormatAbbreviated]
	}
	return out
}
