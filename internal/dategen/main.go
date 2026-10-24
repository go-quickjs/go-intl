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
// Which calendar a locale reckons in by default is a property of its region
// rather than its language, and CLDR's calendarPreferenceData says so. That
// file is vendored beside this command and written out as its own table.
package main

import (
	_ "embed"
	"encoding/json"
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

//go:embed calendarPreferenceData.json
var calendarPreferenceJSON []byte

//go:embed dayPeriods.json
var dayPeriodsJSON []byte

//go:embed weekData.json
var weekDataJSON []byte

//go:embed timeData.json
var timeDataJSON []byte

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

// amPmMarkers are ICU's arrays of the two halves of the day, by width; the
// short width has none.
var amPmMarkers = [datedata.Widths]string{"AmPmMarkers", "AmPmMarkersAbbr", "AmPmMarkersNarrow", ""}

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
	built, err := icusrc.EachLocale(icu, fb, func(v *icusrc.View) ([]byte, error) {
		l, err := readLocale(icu, fb, v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", v.Name(), err)
		}
		return datedata.Encode(l, pool), nil
	})
	if err != nil {
		return err
	}

	prefs, err := calendarPreferences()
	if err != nil {
		return err
	}
	patterns, err := patternCalendars(icu, fb)
	if err != nil {
		return err
	}
	hours, err := timeData()
	if err != nil {
		return err
	}
	week, err := weekData()
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
// in extras' order, and the ISO calendar last.
func readLocale(icu *icusrc.Locales, fb icusrc.Fallback, v *icusrc.View) (*datedata.Locale, error) {
	name := v.Name()
	chain, err := icu.Resolve(name, fb)
	if err != nil {
		return nil, err
	}
	l := &datedata.Locale{PeriodRules: periodRules(icusrc.Tag(name))}
	if l.FieldNames, err = readFieldNames(v); err != nil {
		return nil, err
	}
	if l.DateTimeGlue, err = dateTimeGlue(chain); err != nil {
		return nil, err
	}
	calendar := func(cal string) (*datedata.Calendar, error) {
		c, err := readCalendar(v, cal)
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
	iso := isoCalendar(*greg, chain)
	if iso.IntervalFallback, iso.Intervals, err = intervalsFromICU(chain, "iso8601"); err != nil {
		return nil, fmt.Errorf("iso8601: %w", err)
	}
	if iso.Available, err = availableFromICU(chain, "iso8601"); err != nil {
		return nil, fmt.Errorf("iso8601: %w", err)
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

// calendarPreferences writes the calendars each region reckons in, most
// preferred first, as lines of "region calendar...", sorted by region: the
// first is the one it defaults to, and the whole list is what
// Intl.Locale.prototype.getCalendars answers.
func calendarPreferences() ([]byte, error) {
	var res struct {
		Supplemental struct {
			CalendarPreferenceData map[string][]string `json:"calendarPreferenceData"`
		} `json:"supplemental"`
	}
	if err := json.Unmarshal(calendarPreferenceJSON, &res); err != nil {
		return nil, fmt.Errorf("calendarPreferenceData.json: %w", err)
	}
	if len(res.Supplemental.CalendarPreferenceData) == 0 {
		return nil, fmt.Errorf("calendarPreferenceData.json names no regions")
	}
	regions := make([]string, 0, len(res.Supplemental.CalendarPreferenceData))
	for region := range res.Supplemental.CalendarPreferenceData {
		regions = append(regions, region)
	}
	sort.Strings(regions)

	var b strings.Builder
	for _, region := range regions {
		list := res.Supplemental.CalendarPreferenceData[region]
		if len(list) == 0 {
			continue
		}
		names := make([]string, len(list))
		for i, name := range list {
			// CLDR writes some calendars' names in full where BCP-47
			// shortens them, and the short ones are what ECMA-402 uses.
			switch name {
			case "gregorian":
				name = "gregory"
			case "ethiopic-amete-alem":
				name = "ethioaa"
			}
			names[i] = name
		}
		fmt.Fprintf(&b, "%s %s\n", region, strings.Join(names, " "))
	}
	return []byte(b.String()), nil
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

// timeData writes CLDR's hour-cycle preferences, which are a property of a
// region, or of a language in a region, as an index (blob.Index) read where
// it lies: by "de_AT" or "AT", the preferred cycle, a space and the allowed
// ones separated by commas, as CLDR spells them ("H H,hB").
func timeData() ([]byte, error) {
	var res struct {
		Supplemental struct {
			TimeData map[string]struct {
				Allowed   string `json:"_allowed"`
				Preferred string `json:"_preferred"`
			} `json:"timeData"`
		} `json:"supplemental"`
	}
	if err := json.Unmarshal(timeDataJSON, &res); err != nil {
		return nil, fmt.Errorf("timeData.json: %w", err)
	}
	keys := make([]string, 0, len(res.Supplemental.TimeData))
	for key := range res.Supplemental.TimeData {
		keys = append(keys, key)
	}
	// An index (blob.Index), looked up where it lies: the key, "de_AT" or
	// "AT", then the preferred hour format and the allowed ones, "H H,hB".
	records := map[string][]byte{}
	for _, key := range keys {
		d := res.Supplemental.TimeData[key]
		records[strings.ReplaceAll(key, "-", "_")] = []byte(d.Preferred + " " +
			strings.Join(strings.Fields(d.Allowed), ","))
	}
	return blob.BuildIndex(layout.TimeData, records)
}

// japaneseEras writes the start dates of the Japanese calendar's eras, from
// ICU's supplemental data, as EraRules reads them: one line each, the era's
// number and its first day, "236 2019 5 1". Tentative eras, which have no
// name yet, are left out, as ICU leaves them out unless told otherwise.
func japaneseEras(icu *icusrc.Locales) ([]byte, error) {
	b, err := icu.ReadMisc("supplementalData")
	if err != nil {
		return nil, err
	}
	root, err := icutxt.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("supplementalData.txt: %w", err)
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

// weekData writes CLDR's week conventions, which are a property of a
// region: one line each, the field, the region and the value, sorted. The
// days are numbered from Sunday, zero. "001" is the world's.
func weekData() ([]byte, error) {
	var res struct {
		Supplemental struct {
			WeekData struct {
				MinDays      map[string]string `json:"minDays"`
				FirstDay     map[string]string `json:"firstDay"`
				WeekendStart map[string]string `json:"weekendStart"`
				WeekendEnd   map[string]string `json:"weekendEnd"`
			} `json:"weekData"`
		} `json:"supplemental"`
	}
	if err := json.Unmarshal(weekDataJSON, &res); err != nil {
		return nil, fmt.Errorf("weekData.json: %w", err)
	}
	days := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	var lines []string
	w := res.Supplemental.WeekData
	for _, f := range []struct {
		name   string
		values map[string]string
		isDay  bool
	}{
		{"firstDay", w.FirstDay, true}, {"minDays", w.MinDays, false},
		{"weekendStart", w.WeekendStart, true}, {"weekendEnd", w.WeekendEnd, true},
	} {
		for region, v := range f.values {
			// CLDR adds variants such as "GB-alt-variant", which ICU leaves out.
			if strings.Contains(region, "-") {
				continue
			}
			if f.isDay {
				d, ok := days[v]
				if !ok {
					return nil, fmt.Errorf("weekData.json: %s %s is %q", f.name, region, v)
				}
				v = strconv.Itoa(d)
			}
			lines = append(lines, f.name+" "+region+" "+v)
		}
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// periodRuleSets are CLDR's day-period rules, read once.
var periodRuleSets = func() map[string]map[string]map[string]string {
	var res struct {
		Supplemental struct {
			DayPeriodRuleSet map[string]map[string]map[string]string `json:"dayPeriodRuleSet"`
		} `json:"supplemental"`
	}
	if err := json.Unmarshal(dayPeriodsJSON, &res); err != nil {
		panic("dategen: dayPeriods.json: " + err.Error())
	}
	return res.Supplemental.DayPeriodRuleSet
}()

// periodRules returns the rules that say which part of the day an hour falls
// in. CLDR keeps them per language rather than per locale, so a locale with no
// rules of its own takes its language's.
func periodRules(name string) []datedata.PeriodRule {
	set, ok := periodRuleSets[name]
	if !ok {
		language, _, _ := strings.Cut(name, "-")
		if set, ok = periodRuleSets[language]; !ok {
			return nil
		}
	}
	out := make([]datedata.PeriodRule, 0, len(set))
	for id, rule := range set {
		r := datedata.PeriodRule{ID: id}
		if at, ok := rule["_at"]; ok {
			r.At, r.Point = minutes(at), true
		} else {
			from, hasFrom := rule["_from"]
			before, hasBefore := rule["_before"]
			if !hasFrom || !hasBefore {
				continue
			}
			r.From, r.Before = minutes(from), minutes(before)
		}
		out = append(out, r)
	}
	// The ranges come first so that a language naming both a range over
	// midnight and the moment itself is written with the range, which is what
	// ICU does.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Point != out[j].Point {
			return !out[i].Point
		}
		return out[i].ID < out[j].ID
	})
	return out
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
func readCalendar(v *icusrc.View, cal string) (*datedata.Calendar, error) {
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
		if amPmMarkers[w] != "" {
			marks, err := get(amPmMarkers[w])
			if err != nil {
				return nil, err
			}
			if m := texts(marks); len(m) == 2 {
				c.AM[w], c.PM[w] = m[0], m[1]
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
