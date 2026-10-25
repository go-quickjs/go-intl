package date

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/chrome"
)

// TestDateMatchesNode replays testdata/date_node.js: toString,
// toDateString, toTimeString and getTimezoneOffset about every transition
// of every zone Node knows and at the ends of time, dates made from local
// fields across every transition, Date.parse, toUTCString and toISOString.
//
// Where INTL_CHROME asks for them -- Chrome versions separated by commas,
// or "all" -- it replays each Chrome's recording of the same cases too
// (testdata/chrome), with that Chrome's profile: its zones' rules and its
// names for them, in English and in each language it was recorded in,
// which is ICU's default locale and so the locale it names zones in. A
// Chrome's recording in English names the Chrome and not the clock, which
// is Node's: the names a Chrome asked for at its own now are those of the
// same metazones. One in another language keeps the Chrome's own clock.
func TestDateMatchesNode(t *testing.T) {
	lines := recordingLines(t, readGzipped(t, "../testdata/date_node.txt.gz"))
	// The header has the clock and the locale the zone names were asked
	// for at (V8 asks ICU for a zone's names once, at the current time).
	header := lines[0]
	i := strings.Index(header, "now ")
	if i < 0 || !strings.Contains(header, "ICU 78.3, tz 2026c") {
		t.Fatalf("the expectations come from %q", header)
	}
	var ms int64
	var tag string
	fmt.Sscanf(header[i:], "now %d, locale %s", &ms, &tag)
	now := time.UnixMilli(ms)
	loc, err := intl.ParseLocale(tag)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("Node", func(t *testing.T) { replayDate(t, intl.Embedded, intl.NodeICU, lines[1:], now, loc) })

	want := os.Getenv("INTL_CHROME")
	if want == "" {
		return
	}
	dirs, err := filepath.Glob(filepath.FromSlash("../testdata/chrome/[0-9]*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		version := filepath.Base(dir)
		if want != "all" && !contains(strings.Split(want, ","), version) {
			continue
		}
		// In English, then in each language there are recordings in, the
		// language being ICU's default locale, which Date names zones in.
		languages, err := filepath.Glob(filepath.Join(dir, "[a-z]*"))
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range append([]string{dir}, languages...) {
			delta := filepath.Join(d, "date_node.txt.gz.delta.gz")
			if _, err := os.Stat(delta); err != nil {
				continue
			}
			who, language, check := "Chrome "+version, loc, "# Chrome "+version+", ICU "
			if d != dir {
				who += " in " + filepath.Base(d)
				if language, err = intl.ParseLocale(filepath.Base(d)); err != nil {
					t.Fatal(err)
				}
			}
			t.Run(who, func(t *testing.T) {
				p, err := chrome.ForVersion(version)
				if err != nil {
					t.Fatal(err)
				}
				got := applyDelta(t, lines, readGzipped(t, delta))
				if !strings.HasPrefix(got[0], check) || d != dir && !strings.Contains(got[0], ", language "+language.String()) {
					t.Fatalf("the expectations come from %q", got[0])
				}
				// A recording in a language keeps the clock the Chrome
				// asked for its zones' names at.
				clock := now
				if i := strings.Index(got[0], "now "); i >= 0 {
					var ms int64
					fmt.Sscanf(got[0][i:], "now %d", &ms)
					clock = time.UnixMilli(ms)
				}
				replayDate(t, p.Source, p.Compat, got[1:], clock, language)
			})
		}
	}
}

// replayDate checks a recording's cases, but for its header, over a source
// and in a recording's divergences.
func replayDate(t *testing.T, src intl.Source, compat intl.Compat, lines []string, now time.Time, loc intl.Locale) {
	// The recordings ran with ICU's default locale the one the header
	// names, whatever the host's is.
	src = intl.WithDefaultLocale(src, loc)
	var env *Environment
	// Setting process.env.TZ empties V8's offset cache, so each zone line
	// starts a new Environment.
	envFor := func(zone string) *Environment {
		tz, err := intl.LoadTimeZone(src, zone)
		if err != nil {
			t.Fatalf("%s: %v", zone, err)
		}
		e, err := New(Options{Source: src, Locale: &loc, TimeZone: tz, Now: func() time.Time { return now }, Compat: compat})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	zone := ""
	parseEnvs := map[string]*Environment{}
	ran, bad := map[string]int{}, map[string]int{}
	fail := func(kind, format string, args ...any) {
		bad[kind]++
		if bad[kind] <= 15 {
			t.Errorf("%s %s: "+format, append([]any{kind, zone}, args...)...)
		}
	}
	for _, line := range lines {
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		var kind string
		json.Unmarshal(c[0], &kind)
		ran[kind]++
		switch kind {
		case "zone":
			json.Unmarshal(c[1], &zone)
			env = envFor(zone)
		case "str":
			var tv float64
			var str, ds, ts string
			var offset float64
			json.Unmarshal(c[1], &tv)
			json.Unmarshal(c[2], &str)
			json.Unmarshal(c[3], &ds)
			json.Unmarshal(c[4], &ts)
			json.Unmarshal(c[5], &offset)
			// new Date(t) reads its local time through the cache before
			// any of the four do.
			tv = env.NewDate(tv).Value()
			if got := env.String(tv); got != str {
				fail(kind, "String(%v) = %q, want %q", tv, got, str)
			}
			if got := env.DateString(tv); got != ds {
				fail(kind, "DateString(%v) = %q, want %q", tv, got, ds)
			}
			if got := env.TimeString(tv); got != ts {
				fail(kind, "TimeString(%v) = %q, want %q", tv, got, ts)
			}
			if got := env.TimezoneOffset(tv); got != offset {
				fail(kind, "TimezoneOffset(%v) = %v, want %v", tv, got, offset)
			}
		case "local":
			var fields []float64
			var want *float64
			json.Unmarshal(c[1], &fields)
			json.Unmarshal(c[2], &want)
			for len(fields) < 7 {
				v := 0.0
				if len(fields) == 2 {
					v = 1
				}
				fields = append(fields, v)
			}
			year := fields[0]
			// new Date(y, ...) takes a year of 0 to 99 as 1900 on.
			if y := math.Trunc(year); y >= 0 && y <= 99 {
				year = 1900 + y
			}
			local := MakeDate(MakeDay(year, fields[1], fields[2]), MakeTime(fields[3], fields[4], fields[5], fields[6]))
			// new Date(y, m, ...) reads the local time of what it made.
			got := env.NewDate(env.UTC(local)).Value()
			if want == nil && !math.IsNaN(got) || want != nil && got != *want {
				fail(kind, "UTC of %v = %v, want %v", fields, got, want)
			}
		case "parse":
			var pz string
			var str []uint16
			var want *float64
			json.Unmarshal(c[1], &pz)
			json.Unmarshal(c[2], &str)
			json.Unmarshal(c[3], &want)
			if parseEnvs[pz] == nil {
				parseEnvs[pz] = envFor(pz)
			}
			got := parseEnvs[pz].Parse(str)
			if want == nil && !math.IsNaN(got) || want != nil && got != *want {
				fail(kind, "Parse(%q) in %s = %v, want %v", string(utf16Runes(str)), pz, got, want)
			}
		case "utc":
			var tv *float64
			var utc string
			var iso *string
			json.Unmarshal(c[1], &tv)
			json.Unmarshal(c[2], &utc)
			json.Unmarshal(c[3], &iso)
			v := math.NaN()
			if tv != nil {
				v = TimeClip(*tv)
			}
			if got := UTCString(v); got != utc {
				fail(kind, "UTCString(%v) = %q, want %q", v, got, utc)
			}
			gotISO, ok := ISOString(v)
			if iso == nil && ok || iso != nil && gotISO != *iso {
				fail(kind, "ISOString(%v) = %q %v, want %v", v, gotISO, ok, iso)
			}
		}
	}
	total := 0
	for k, n := range ran {
		total += n
		if bad[k] > 0 {
			t.Logf("%s: %d of %d differ", k, bad[k], n)
		}
	}
	if total < 100000 {
		t.Fatalf("%d cases: the recording is short", total)
	}
	t.Logf("%d cases", total)
}

func readGzipped(t *testing.T, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(file))
	if err != nil {
		t.Fatal(err)
	}
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// recordingLines is a recording's lines, without the empty one after its
// last newline.
func recordingLines(t *testing.T, b []byte) []string {
	t.Helper()
	lines := strings.Split(string(b), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if len(lines) == 0 {
		t.Fatal("an empty recording")
	}
	return lines
}

// applyDelta makes a Chrome's recording from Node's lines and its edits, as
// testdata/chrome/delta.js writes them: "=" keeps n lines, "-" drops n, "+"
// adds the lines given.
func applyDelta(t *testing.T, base []string, delta []byte) []string {
	t.Helper()
	var d struct {
		Edits []json.RawMessage `json:"edits"`
	}
	if err := json.Unmarshal(delta, &d); err != nil {
		t.Fatal(err)
	}
	// delta.js's lines are the text split at each newline, a last empty
	// one included, which base has had taken off.
	base = append(base[:len(base):len(base)], "")
	var out []string
	at := 0
	for _, e := range d.Edits {
		var op []json.RawMessage
		var kind string
		if json.Unmarshal(e, &op) != nil || len(op) != 2 || json.Unmarshal(op[0], &kind) != nil {
			t.Fatalf("an edit %s", e)
		}
		switch kind {
		case "=", "-":
			var n int
			if err := json.Unmarshal(op[1], &n); err != nil || at+n > len(base) {
				t.Fatalf("an edit %s past the end", e)
			}
			if kind == "=" {
				out = append(out, base[at:at+n]...)
			}
			at += n
		case "+":
			var lines []string
			if err := json.Unmarshal(op[1], &lines); err != nil {
				t.Fatalf("an edit %s", e)
			}
			out = append(out, lines...)
		default:
			t.Fatalf("an edit %s", e)
		}
	}
	if at != len(base) {
		t.Fatalf("the edits cover %d of %d lines", at, len(base))
	}
	if n := len(out); n > 0 && out[n-1] == "" {
		out = out[:n-1]
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func utf16Runes(u []uint16) []rune {
	var out []rune
	for _, c := range u {
		out = append(out, rune(c))
	}
	return out
}
