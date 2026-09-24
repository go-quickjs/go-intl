// Command dategen writes the date tables the intl package carries.
//
// It reads CLDR's cldr-dates-full package, which is not vendored: SOURCES.md
// pins the release and its checksum, and the path to an unpacked copy is given
// here.
//
//	go run ./internal/dategen -icu icu4c-78.3-data.zip <cldr-dates-full/package>
//
// Each calendar beyond the Gregorian one is its own CLDR package, and each is
// given as a further argument:
//
//	go run ./internal/dategen -icu icu4c-78.3-data.zip <cldr-dates-full/package> <cldr-cal-buddhist-full/package>
//
// ICU's data sources, pinned in SOURCES.md, settle one thing cldr-json gets
// wrong. CLDR's root points a calendar's date-and-time glue at the asking
// locale's own, and cldr-json resolves that as if it pointed at the root's,
// so Arabic's Buddhist dates take a Latin comma. ICU looks the glue up in the
// locale's calendar and, finding none, in the locale's Gregorian one.
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

	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/icusrc"
)

//go:embed calendarPreferenceData.json
var calendarPreferenceJSON []byte

//go:embed dayPeriods.json
var dayPeriodsJSON []byte

//go:embed timeData.json
var timeDataJSON []byte

// appendFields are CLDR's names for the append items, by pattern-generator
// field. The fields with no append item of their own are empty.
var appendFields = [datedata.Fields]string{
	"Era", "Year", "Quarter", "Month", "Week", "", "Day-Of-Week",
	"", "", "Day", "", "Hour", "Minute", "Second", "", "Timezone",
}

// fieldNames are CLDR's names for the fields in dateFields.json, by
// pattern-generator field.
var fieldNames = [datedata.Fields]string{
	"era", "year", "quarter", "month", "week", "weekOfMonth", "weekday",
	"dayOfYear", "weekdayOfMonth", "day", "dayperiod", "hour", "minute", "second", "", "zone",
}

// extras are the calendars beyond the Gregorian one, in the order their
// packages are given. CLDR's name for a calendar is not always BCP-47's, which
// is the name ECMA-402 uses and the one stored.
var extras = []struct{ cldr, bcp47 string }{
	{"buddhist", "buddhist"},
}

type file struct {
	Main map[string]struct {
		Dates struct {
			Calendars map[string]calendar `json:"calendars"`
		} `json:"dates"`
	} `json:"main"`
}

type calendar struct {
	Months     map[string]map[string]map[string]string `json:"months"`
	Days       map[string]map[string]map[string]string `json:"days"`
	DayPeriods map[string]map[string]map[string]string `json:"dayPeriods"`
	Eras       map[string]map[string]string            `json:"eras"`

	DateFormats     map[string]json.RawMessage `json:"dateFormats"`
	TimeFormats     map[string]json.RawMessage `json:"timeFormats"`
	DateTimeFormats map[string]json.RawMessage `json:"dateTimeFormats"`
	AtTime          struct {
		Standard map[string]json.RawMessage `json:"standard"`
	} `json:"dateTimeFormats-atTime"`
}

// widthNames are CLDR's names for the widths, in storage order.
var widthNames = [datedata.Widths]string{"wide", "abbreviated", "narrow", "short"}

// contextNames are CLDR's names for the two contexts, in storage order.
var contextNames = [datedata.Contexts]string{"format", "stand-alone"}

// weekdayKeys are CLDR's keys for the days, Sunday first.
var weekdayKeys = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// eraWidths are CLDR's era name sets, in storage order.
var eraWidths = [datedata.Widths]string{"eraNames", "eraAbbr", "eraNarrow", "eraNarrow"}

func main() {
	icuData := flag.String("icu", "", "the path to icu4c-78.3-data.zip")
	flag.Parse()
	if flag.NArg() < 1 || *icuData == "" {
		fmt.Fprintln(os.Stderr,
			"usage: go run ./internal/dategen -icu <icu4c-78.3-data.zip> <cldr-dates-full/package> [<cldr-cal-*-full/package>...]")
		os.Exit(2)
	}
	icu, err := icusrc.OpenLocales(*icuData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dategen:", err)
		os.Exit(1)
	}
	defer icu.Close()
	if err := run(icu, flag.Arg(0), flag.Args()[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dategen:", err)
		os.Exit(1)
	}
}

func run(icu *icusrc.Locales, root string, others []string) error {
	main := filepath.Join(root, "main")
	entries, err := os.ReadDir(main)
	if err != nil {
		return fmt.Errorf("reading %s: %w", main, err)
	}

	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}
	built := map[string][]byte{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		l, err := read(main, e.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if l == nil {
			continue
		}
		greg := &l.Calendars[0].Calendar
		if greg.AtTimeFormats, err = atTimeFromICU(icu, e.Name(), "gregorian"); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		chain, err := icuChain(icu, fb, e.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if l.DateTimeGlue, err = dateTimeGlue(chain); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if greg.IntervalFallback, greg.Intervals, err = intervalsFromICU(chain, "gregorian"); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		for i, other := range others {
			if i >= len(extras) {
				break
			}
			c, err := readCalendar(filepath.Join(other, "main"), e.Name(),
				extras[i].cldr, "ca-"+extras[i].cldr+".json")
			if err != nil {
				return fmt.Errorf("%s: %s: %w", e.Name(), extras[i].cldr, err)
			}
			if c == nil {
				continue
			}
			if c.AtTimeFormats, err = atTimeFromICU(icu, e.Name(), extras[i].cldr); err != nil {
				return fmt.Errorf("%s: %s: %w", e.Name(), extras[i].cldr, err)
			}
			if c.IntervalFallback, c.Intervals, err = intervalsFromICU(chain, extras[i].cldr); err != nil {
				return fmt.Errorf("%s: %s: %w", e.Name(), extras[i].cldr, err)
			}
			l.Calendars = append(l.Calendars, datedata.NamedCalendar{
				Name: extras[i].bcp47, Calendar: *c,
			})
		}
		built[e.Name()] = datedata.Encode(l)
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}

	out := filepath.Join("data", "dates")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(out, "*.bin"))
	for _, name := range old {
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(built))
	for name := range built {
		names = append(names, name)
	}
	sort.Strings(names)
	var total int
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(out, name+".bin"), built[name], 0o644); err != nil {
			return err
		}
		total += len(built[name])
	}
	prefs, err := calendarPreferences()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "calendarprefs.bin"), prefs, 0o644); err != nil {
		return err
	}
	hours, err := timeData()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "timedata.bin"), hours, 0o644); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "dategen: %d locales, %d calendars, %.1f MB\n",
		len(names), 1+len(others), float64(total)/(1<<20))
	return nil
}

// calendarPreferences writes which calendar each region reckons in, as lines
// of "region calendar", sorted. Only the first is kept: the rest are calendars
// a region also uses rather than ones it defaults to.
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
		name := list[0]
		// CLDR writes the Gregorian calendar's name in full where BCP-47
		// shortens it, and the short one is what ECMA-402 uses.
		if name == "gregorian" {
			name = "gregory"
		}
		fmt.Fprintf(&b, "%s %s\n", region, name)
	}
	return []byte(b.String()), nil
}

func read(main, name string) (*datedata.Locale, error) {
	c, err := readCalendar(main, name, "gregorian", "ca-gregorian.json")
	if err != nil || c == nil {
		return nil, err
	}
	l := &datedata.Locale{
		Calendars:   []datedata.NamedCalendar{{Name: "gregory", Calendar: *c}},
		PeriodRules: periodRules(name),
	}
	if l.FieldNames, err = readFieldNames(main, name); err != nil {
		return nil, err
	}
	return l, nil
}

// readFieldNames reads what a locale calls each field, from dateFields.json.
func readFieldNames(main, name string) ([datedata.Fields]string, error) {
	var out [datedata.Fields]string
	raw, err := os.ReadFile(filepath.Join(main, name, "dateFields.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	var f struct {
		Main map[string]struct {
			Dates struct {
				Fields map[string]struct {
					DisplayName string `json:"displayName"`
				} `json:"fields"`
			} `json:"dates"`
		} `json:"main"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return out, fmt.Errorf("dateFields.json: %w", err)
	}
	fields := f.Main[name].Dates.Fields
	for i, key := range fieldNames {
		if key != "" {
			out[i] = fields[key].DisplayName
		}
	}
	return out, nil
}

// timeData writes CLDR's hour-cycle preferences, which are a property of a
// region, or of a language in a region: one line each, the key, the preferred
// cycle and the allowed ones, as CLDR spells them ("h", "H", "hB").
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
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		d := res.Supplemental.TimeData[key]
		fmt.Fprintf(&b, "%s %s %s\n", strings.ReplaceAll(key, "-", "_"), d.Preferred,
			strings.Join(strings.Fields(d.Allowed), ","))
	}
	return []byte(b.String()), nil
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

// readCalendar reads one calendar out of one package.
func readCalendar(main, name, calendarName, fileName string) (*datedata.Calendar, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", fileName, err)
	}
	entry, ok := f.Main[name]
	if !ok {
		return nil, nil
	}
	source, ok := entry.Dates.Calendars[calendarName]
	if !ok {
		return nil, nil
	}

	var c datedata.Calendar
	for ctx := 0; ctx < datedata.Contexts; ctx++ {
		for w := 0; w < datedata.Widths; w++ {
			at := ctx*datedata.Widths + w
			if set, ok := source.Months[contextNames[ctx]][widthNames[w]]; ok {
				text := make([]string, 12)
				for key, value := range set {
					n, err := strconv.Atoi(key)
					if err != nil || n < 1 || n > 12 {
						continue
					}
					text[n-1] = value
				}
				c.Months[at].Text = text
			}
			if set, ok := source.Days[contextNames[ctx]][widthNames[w]]; ok {
				text := make([]string, 7)
				for i, key := range weekdayKeys {
					text[i] = set[key]
				}
				c.Days[at].Text = text
			}
		}
	}
	for w := 0; w < datedata.Widths; w++ {
		if set, ok := source.DayPeriods["format"][widthNames[w]]; ok {
			c.AM[w], c.PM[w] = set["am"], set["pm"]
			// The finer parts of the day are kept too, for the patterns that
			// ask for them rather than for the two halves.
			for id, text := range set {
				if text == "" || strings.Contains(id, "-alt-") ||
					id == "am" || id == "pm" {
					continue
				}
				c.Periods[w] = append(c.Periods[w],
					datedata.DayPeriod{ID: id, Text: text})
			}
			sort.Slice(c.Periods[w], func(a, b int) bool {
				return c.Periods[w][a].ID < c.Periods[w][b].ID
			})
		}
		if set, ok := source.Eras[eraWidths[w]]; ok {
			c.Eras[w].Text = []string{set["0"], set["1"]}
		}
	}

	for i, length := range datedata.LengthNames {
		c.DateFormats[i] = pattern(source.DateFormats[length])
		c.TimeFormats[i] = pattern(source.TimeFormats[length])
		c.DateNumbers[i] = numbersOverride(source.DateFormats[length])
		c.TimeNumbers[i] = numbersOverride(source.TimeFormats[length])
		c.DateTimeFormats[i] = pattern(source.DateTimeFormats[length])
		c.AtTimeFormats[i] = pattern(source.AtTime.Standard[length])
	}

	if raw, ok := source.DateTimeFormats["availableFormats"]; ok {
		var loose map[string]json.RawMessage
		if err := json.Unmarshal(raw, &loose); err != nil {
			return nil, fmt.Errorf("availableFormats: %w", err)
		}
		available := make(map[string]string, len(loose))
		for id, value := range loose {
			available[id] = pattern(value)
		}
		for id, text := range available {
			if strings.Contains(id, "-alt-") || strings.Contains(id, "-count-") || text == "" {
				continue
			}
			c.Available = append(c.Available, datedata.Skeleton{ID: id, Pattern: text})
		}
		sort.Slice(c.Available, func(i, j int) bool {
			return c.Available[i].ID < c.Available[j].ID
		})
	}

	if raw, ok := source.DateTimeFormats["appendItems"]; ok {
		var items map[string]string
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("appendItems: %w", err)
		}
		for i, key := range appendFields {
			if key != "" {
				c.AppendItems[i] = items[key]
			}
		}
	}

	if c.DateFormats[datedata.Full] == "" && len(c.Available) == 0 {
		return nil, nil
	}
	return &c, nil
}

// pattern reads a pattern, which CLDR writes either as a string or, where it
// carries attributes of its own, as an object with the pattern under _value.
func pattern(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var wrapped struct {
		Value string `json:"_value"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.Value
	}
	return ""
}

// numbersOverride is the numbering override CLDR gives a pattern, written
// beside it as "_numbers": "M=romanlow".
func numbersOverride(raw json.RawMessage) string {
	var wrapped struct {
		Numbers string `json:"_numbers"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return ""
	}
	return wrapped.Numbers
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
