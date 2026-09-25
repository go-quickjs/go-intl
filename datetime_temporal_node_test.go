package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// TestDateTimeTemporalMatchesNode holds the writing of Temporal values to
// Node's: Intl.DateTimeFormat's format, formatToParts and formatRange of
// each kind, and each type's toLocaleString, with the errors Node throws.
// The expectations are written by testdata/datetime_temporal_node.js.
//
// The replay does what an engine does around go-intl, as V8 does it: checks
// the value's calendar, turns the value into an instant in the formatter's
// zone with Temporal's "compatible" disambiguation, and asks ForTemporal for
// the kind's formatter.
func TestDateTimeTemporalMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/datetime_temporal_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(make([]byte, 1<<20), 1<<20)

	zones := map[string]*intl.TimeZone{}
	zone := func(name string) *intl.TimeZone {
		if zones[name] == nil {
			tz, err := intl.LoadTimeZone(intl.Embedded, name)
			if err != nil {
				t.Fatal(err)
			}
			zones[name] = tz
		}
		return zones[name]
	}

	var ran, matched int
	var differences []string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var c struct {
			tag      string
			opts     map[string]any
			method   string
			kind     string
			calendar string
			values   [][]any
			result   json.RawMessage
		}
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 7 {
			t.Fatalf("a line that is not seven fields: %s", line)
		}
		for i, dst := range []any{&c.tag, &c.opts, &c.method, &c.kind, &c.calendar, &c.values} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		c.result = fields[6]
		ran++

		got := temporalOutcome(c.tag, c.opts, c.method, c.kind, c.calendar, c.values, zone)
		var want any
		if err := json.Unmarshal(c.result, &want); err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(normalizeJSON(got), want) {
			matched++
			continue
		}
		gotText, _ := json.Marshal(got)
		differences = append(differences, fmt.Sprintf("%s %s %s %s %s %v\n   got  %s\n   want %s",
			c.tag, fields[1], c.method, c.kind, c.calendar, c.values, gotText, c.result))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly", matched, ran)
	if len(differences) > 0 {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 && os.Getenv("ALLDIFF") == "" {
			shown = shown[:25]
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(shown, "\n"))
	}
}

// normalizeJSON is a value as encoding/json reads it back.
func normalizeJSON(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

type jsError struct {
	Error string `json:"error"`
}

var (
	typeError  = jsError{"TypeError"}
	rangeError = jsError{"RangeError"}
)

// temporalKinds are the replay's kinds as go-intl's.
var temporalKinds = map[string]intl.TemporalKind{
	"date": intl.TemporalPlainDate, "datetime": intl.TemporalPlainDateTime,
	"time": intl.TemporalPlainTime, "yearmonth": intl.TemporalPlainYearMonth,
	"monthday": intl.TemporalPlainMonthDay, "instant": intl.TemporalInstant,
	"zoned": intl.TemporalInstant,
}

func temporalOutcome(tag string, in map[string]any, method, kind, calendar string, values [][]any,
	zone func(string) *intl.TimeZone) any {
	opts, err := dateTimeOptions(in)
	if err != nil {
		return err.Error()
	}
	if method == "toLocaleString" {
		// CreateDateTimeFormat with the type's required and defaults, and a
		// ZonedDateTime's own zone.
		switch kind {
		case "date", "yearmonth", "monthday":
			opts.Required, opts.Defaults = intl.ComponentsDate, intl.ComponentsDate
		case "time":
			opts.Required, opts.Defaults = intl.ComponentsTime, intl.ComponentsTime
		default:
			opts.Required, opts.Defaults = intl.ComponentsAny, intl.ComponentsAll
		}
		if kind == "zoned" {
			if opts.TimeZone != "" {
				return typeError
			}
			opts.TimeZone, opts.ToLocaleStringTimeZone = values[0][1].(string), true
		}
	}
	loc, err := intl.ParseLocale(tag)
	if err != nil {
		return err.Error()
	}
	f, err := intl.NewDateTimeFormat(loc, opts)
	if err != nil {
		// The option errors CreateDateTimeFormat throws here are the
		// TypeErrors of a style a method's required fields refuse.
		return typeError
	}
	k := temporalKinds[kind]
	if !f.CalendarMatches(k, calendar) {
		return rangeError
	}
	tz := zone(f.ResolvedOptions().TimeZone)
	instants := make([]time.Time, len(values))
	for i, v := range values {
		instants[i] = temporalInstant(kind, v, tz)
	}
	kf, err := f.ForTemporal(k)
	if errors.Is(err, intl.ErrTemporalFormat) {
		return typeError
	}
	if err != nil {
		return err.Error()
	}
	switch method {
	case "parts":
		var out [][2]string
		for _, p := range kf.FormatToParts(instants[0]) {
			out = append(out, [2]string{string(p.Kind), p.Value})
		}
		return out
	case "range":
		return kf.FormatRange(instants[0], instants[1])
	}
	return kf.Format(instants[0])
}

// temporalInstant is the instant a value names: a plain value's fields in
// the zone, read as Temporal's "compatible" reads them -- a skipped time
// and a repeated one both with the offset before the transition.
func temporalInstant(kind string, v []any, tz *intl.TimeZone) time.Time {
	n := func(i int) int64 { return int64(v[i].(float64)) }
	if kind == "instant" || kind == "zoned" {
		return time.UnixMilli(n(0))
	}
	day := time.Date(int(n(0)), time.Month(n(1)), int(n(2)), 0, 0, 0, 0, time.UTC).UnixMilli()
	local := day + ((n(3)*60+n(4))*60+n(5))*1000 + n(6)
	offset := tz.OffsetFromLocal(local, intl.Former, intl.Former)
	return time.UnixMilli(local - int64(offset.Total())*1000)
}
