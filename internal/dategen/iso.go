package main

import (
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// isoCalendar builds the ISO 8601 calendar as ICU keeps it: the root's own
// patterns -- "y MMMM d, EEEE", "y-MM-dd" -- over the locale's Gregorian
// names, which the root's aliases send it to, with the few locales that say
// something of their own (Chinese) merged over the root key by key. The glue
// between a date and a time, which it has none of its own for, stays the
// Gregorian calendar's, as ICU falls back to it.
func isoCalendar(greg datedata.Calendar, chain []*icutxt.Node) *datedata.Calendar {
	c := greg
	c.DateNumbers, c.TimeNumbers = [datedata.Lengths]string{}, [datedata.Lengths]string{}
	// ICU 78 has no era rules for the ISO 8601 calendar, and what
	// DateFormatSymbols is left holding is no wide or abbreviated era names
	// at all and, of the narrow ones, only the first: Node writes nothing for
	// "AD" at any width and the narrow name for "BC", "B" in English.
	c.Eras = [datedata.Widths]datedata.Names{}
	c.Eras[datedata.Narrow].Text = []string{greg.Era(datedata.Narrow, 0)}
	for _, n := range chain {
		p := n.Get("calendar", "iso8601", "DateTimePatterns")
		if p == nil || p.Alias || len(p.Values) < 13 {
			continue
		}
		for i := 0; i < datedata.Lengths; i++ {
			c.TimeFormats[i] = p.Values[i]
			c.DateFormats[i] = p.Values[4+i]
			c.DateTimeFormats[i] = p.Values[9+i]
		}
		break
	}

	available := map[string]string{}
	var items [datedata.Fields]string
	for _, n := range chain {
		if t := n.Get("calendar", "iso8601", "availableFormats"); t != nil && t.Table {
			for _, s := range t.Children {
				if _, done := available[s.Key]; !done && !s.Table && !s.Alias && s.Value != "" &&
					!strings.Contains(s.Key, "-alt-") {
					available[s.Key] = s.Value
				}
			}
		}
		if t := n.Get("calendar", "iso8601", "appendItems"); t != nil && t.Table {
			for i, key := range appendFields {
				if key == "" || items[i] != "" {
					continue
				}
				if v := t.Get(key); v != nil {
					items[i] = v.Value
				}
			}
		}
	}
	if len(available) > 0 {
		c.Available = nil
		for id, text := range available {
			c.Available = append(c.Available, datedata.Skeleton{ID: id, Pattern: text})
		}
		sort.Slice(c.Available, func(i, j int) bool { return c.Available[i].ID < c.Available[j].ID })
	}
	for i, item := range items {
		if item != "" {
			c.AppendItems[i] = item
		}
	}
	return &c
}
