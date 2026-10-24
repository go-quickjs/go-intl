package intl_test

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"
	"unicode/utf16"

	intl "github.com/go-quickjs/go-intl"
)

// TestVariantMatchesNode replays testdata/variant_node.js: every service
// with "-u-va-posix", which ICU reads as the locale's variant POSIX. The
// locale is resolved as a host resolves it, among the service's available
// locales, then built as Node builds it.
func TestVariantMatchesNode(t *testing.T) {
	replay(t, "testdata/variant_node.txt.gz", func(t *testing.T, r recording) {
		def, _ := intl.ParseLocale("en-US")
		matchers := map[string]*intl.LocaleMatcher{}
		resolve := func(service, tag string) intl.Locale {
			m, ok := matchers[service]
			if !ok {
				var err error
				m, err = intl.NewLocaleMatcher(r.src, intl.Service(service))
				if err != nil {
					t.Fatal(err)
				}
				matchers[service] = m
			}
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}
			return m.Resolve([]intl.Locale{loc}, intl.BestFit, def)
		}
		date := time.Date(2020, 6, 15, 13, 4, 5, 0, time.UTC)
		const text = "e-mail: someone@example.com; a:b 12:30 1.5 x_y"
		numbers := []float64{1234567.891, -0.5, math.Inf(1), math.NaN(), 0.1234, 1e21}

		ran := 0
		for _, line := range r.lines {
			var c []json.RawMessage
			if err := json.Unmarshal([]byte(line), &c); err != nil || len(c) != 4 {
				t.Fatalf("%s: %v", line, err)
			}
			var service, tag, resolved string
			json.Unmarshal(c[0], &service)
			json.Unmarshal(c[1], &tag)
			json.Unmarshal(c[2], &resolved)
			var want []json.RawMessage
			json.Unmarshal(c[3], &want)
			loc := resolve(service, tag)
			var gotLocale string
			var got []any
			fail := func(err error) {
				t.Errorf("%s %s: %v", service, tag, err)
			}
			switch service {
			case "NumberFormat":
				var raw map[string]any
				json.Unmarshal(want[0], &raw)
				o, err := numberOptions(raw)
				if err != nil {
					fail(err)
					continue
				}
				o.Compat = r.compatFor(o.Compat)
				nf, err := intl.NewNumberFormatFrom(r.src, loc, o)
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = nf.ResolvedOptions().Locale
				var out []string
				for _, v := range numbers {
					out = append(out, nf.Format(v))
				}
				got = []any{raw, out}
			case "Collator":
				col, err := intl.NewCollatorFrom(r.src, loc, intl.CollatorOptions{Compat: r.compatFor(intl.Standard)})
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = col.ResolvedOptions().Locale
				words := []string{"b", "A", "a", "B", "_", "1", "é", "e", "[", "~", " ", "Z", "z", "0", "ä"}
				sort.SliceStable(words, func(i, j int) bool { return col.Compare(words[i], words[j]) < 0 })
				for _, w := range words {
					got = append(got, w)
				}
			case "DateTimeFormat":
				var raw map[string]any
				json.Unmarshal(want[0], &raw)
				withZone := map[string]any{"timeZone": "UTC"}
				for k, v := range raw {
					withZone[k] = v
				}
				o, err := dateTimeOptions(withZone)
				if err != nil {
					fail(err)
					continue
				}
				o.Compat = r.compatFor(o.Compat)
				df, err := intl.NewDateTimeFormatFrom(r.src, loc, o)
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = df.ResolvedOptions().Locale
				got = []any{raw, df.Format(date)}
			case "RelativeTimeFormat":
				rf, err := intl.NewRelativeTimeFormatFrom(r.src, loc, intl.RelativeTimeFormatOptions{Compat: r.compatFor(intl.Standard)})
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = rf.ResolvedOptions().Locale
				got = []any{rf.Format(1234567.5, intl.RelativeDay), rf.Format(-1, intl.RelativeDay)}
			case "DurationFormat":
				du, err := intl.NewDurationFormatFrom(r.src, loc, intl.DurationFormatOptions{Compat: r.compatFor(intl.Standard)})
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = du.ResolvedOptions().Locale
				var d intl.Duration
				d[intl.DurationHours], d[intl.DurationMinutes] = 1234567, 5
				text, err := du.Format(d)
				if err != nil {
					fail(err)
					continue
				}
				got = []any{text}
			case "PluralRules":
				p, err := intl.NewPluralRulesFrom(r.src, loc, intl.PluralRulesOptions{Compat: r.compatFor(intl.Standard)})
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = p.ResolvedOptions().Locale
				got = []any{string(p.Select(1)), string(p.Select(2))}
			case "ListFormat":
				l, err := intl.NewListFormatFrom(r.src, loc, intl.ListFormatOptions{Compat: r.compatFor(intl.Standard)})
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = l.ResolvedOptions().Locale
				got = []any{l.Format([]string{"a", "b", "c"})}
			case "Segmenter":
				sg, err := intl.NewSegmenterFrom(r.src, loc, intl.SegmenterOptions{Granularity: intl.GranularityWord})
				if err != nil {
					fail(err)
					continue
				}
				// The Segmenter reports no options; its locale is the resolved
				// one, which the other services check.
				gotLocale = resolved
				var segs []string
				u16 := utf16.Encode([]rune(text))
				for _, seg := range sg.SegmentString(text).All() {
					segs = append(segs, string(utf16.Decode(u16[seg.Index:seg.End])))
				}
				got = []any{segs}
			case "DisplayNames":
				dn, err := intl.NewDisplayNamesFrom(r.src, loc, intl.DisplayNamesOptions{Kind: intl.DisplayRegion, Compat: r.compatFor(intl.Standard)})
				if err != nil {
					fail(err)
					continue
				}
				gotLocale = dn.ResolvedOptions().Locale
				name, _ := dn.Of("US")
				got = []any{name}
			default:
				t.Fatalf("no service %s", service)
			}
			ran++
			gotJSON, _ := json.Marshal(got)
			var norm any
			json.Unmarshal(gotJSON, &norm)
			var wantNorm any
			json.Unmarshal(c[3], &wantNorm)
			a, _ := json.Marshal(norm)
			b, _ := json.Marshal(wantNorm)
			if gotLocale != resolved {
				t.Errorf("%s %s: resolved %s, want %s", service, tag, gotLocale, resolved)
			}
			if string(a) != string(b) {
				t.Errorf("%s %s:\n\tgot  %s\n\twant %s", service, tag, a, b)
			}
		}
		if ran == 0 {
			t.Fatal("no cases")
		}
		t.Log(fmt.Sprintf("%d cases", ran))
	})
}
