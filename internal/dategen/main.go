// Command dategen writes the date tables the intl package carries.
//
// It reads CLDR's cldr-dates-full package, which is not vendored: SOURCES.md
// pins the release and its checksum, and the path to an unpacked copy is given
// here.
//
//	go run ./internal/dategen <cldr-dates-full/package>
//
// Only the Gregorian calendar is written so far. The encoding keeps the
// calendars in a list keyed by name, so the other fifteen arrive without its
// shape changing.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datedata"
)

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
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/dategen <cldr-dates-full/package>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "dategen:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	main := filepath.Join(root, "main")
	entries, err := os.ReadDir(main)
	if err != nil {
		return fmt.Errorf("reading %s: %w", main, err)
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
	fmt.Fprintf(os.Stderr, "dategen: %d locales, %.1f MB\n",
		len(names), float64(total)/(1<<20))
	return nil
}

func read(main, name string) (*datedata.Locale, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, "ca-gregorian.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("ca-gregorian.json: %w", err)
	}
	entry, ok := f.Main[name]
	if !ok {
		return nil, nil
	}
	source, ok := entry.Dates.Calendars["gregorian"]
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
					text[n-1] = ascii(value)
				}
				c.Months[at].Text = text
			}
			if set, ok := source.Days[contextNames[ctx]][widthNames[w]]; ok {
				text := make([]string, 7)
				for i, key := range weekdayKeys {
					text[i] = ascii(set[key])
				}
				c.Days[at].Text = text
			}
		}
	}
	for w := 0; w < datedata.Widths; w++ {
		if set, ok := source.DayPeriods["format"][widthNames[w]]; ok {
			c.AM[w], c.PM[w] = ascii(set["am"]), ascii(set["pm"])
		}
		if set, ok := source.Eras[eraWidths[w]]; ok {
			c.Eras[w].Text = []string{ascii(set["0"]), ascii(set["1"])}
		}
	}

	for i, length := range datedata.LengthNames {
		c.DateFormats[i] = pattern(source.DateFormats[length])
		c.TimeFormats[i] = pattern(source.TimeFormats[length])
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
			if strings.Contains(id, "-alt-") || text == "" {
				continue
			}
			c.Available = append(c.Available, datedata.Skeleton{ID: id, Pattern: text})
		}
		sort.Slice(c.Available, func(i, j int) bool {
			return c.Available[i].ID < c.Available[j].ID
		})
	}

	if c.DateFormats[datedata.Full] == "" && len(c.Available) == 0 {
		return nil, nil
	}
	return &datedata.Locale{
		Calendars: []datedata.NamedCalendar{{Name: "gregory", Calendar: c}},
	}, nil
}

// pattern reads a pattern, which CLDR writes either as a string or, where it
// carries attributes of its own, as an object with the pattern under _value.
func pattern(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return ascii(text)
	}
	var wrapped struct {
		Value string `json:"_value"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return ascii(wrapped.Value)
	}
	return ""
}

// ascii puts a plain space where CLDR writes a narrow no-break one.
//
// CLDR separates a time from its day period with U+202F, and the ICU this is
// anchored to writes U+0020 instead, in all 440 locales that have the narrow
// one. ECMA-402 does not specify the character, so the reference decides, and
// the reference is ICU.
//
// Only the character is taken. CLDR's "-alt-ascii" patterns look like they say
// the same thing and do not: en-GB's medium time is "HH:mm:ss" and its ascii
// alternate is "h:mm:ss a", which is a different clock rather than a different
// space.
func ascii(pattern string) string {
	return strings.ReplaceAll(pattern, " ", " ")
}
