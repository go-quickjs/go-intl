// Command tzgen writes the time zones the intl package reckons offsets in:
// ICU's zoneinfo64, from its time zone update 2026c, which is what Node runs
// (see icusrc.TZSHA256).
//
//	go run ./internal/tzgen <icu4c-78.3-data.zip> <icu-tz-2026c dir>
//
// Each zone ICU knows by name has a file, data/tz/<name>.bin, the name in
// lowercase, so that a name is valid when its file exists, in any case, as
// ECMA-402 matches names, and a formatter reads only its own zone. A file is
// lines of text:
//
//	name Asia/Calcutta
//	canonical Asia/Calcutta
//	link asia/kolkata
//
//	name Europe/Paris
//	canonical Europe/Paris
//	types 561,0 0,0 0,3600 0,7200 3600,0 3600,3600
//	trans -1855958961:1 -1689814800:2 -1680397200:1 ...
//	final 3600 1997 2 -31 -1 3600 2 9 -31 -1 3600 2 3600
//
// "name" is the name as ICU spells it. "canonical" is the name
// ZoneMeta::getCanonicalCLDRID gives it, which
// resolvedOptions reports and zone names are keyed by. A name that is a link
// in the tz data has "link", the zone whose rules it keeps, and nothing
// else. A zone has its offsets as ICU's OlsonTimeZone keeps them: "types",
// each a raw offset and a daylight saving, in seconds, the first in force
// before any transition; "trans", each transition's instant, in seconds
// since 1970, and the type it starts; and "final", when the zone ends in a
// rule, the raw offset, the year the rule governs from, and ICU's eleven
// numbers for the rule: start month, day, day of week, time and time mode,
// the same for the end, and the saving.
//
// A name whose canonical zone is Etc/Unknown, as Etc/Unknown's and
// Factory's are, has no file: V8 rejects it.
//
// These are ICU's inputs, the tz database as ICU compiles it: rearguard
// data, whose daylight saving is never negative, so that Ireland's summer is
// its daylight time, as Node has it.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/tzgen <icu4c-78.3-data.zip> <icu-tz-2026c dir>")
		os.Exit(2)
	}
	files, err := build(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "tzgen:", err)
		os.Exit(1)
	}
	if err := write(files); err != nil {
		fmt.Fprintln(os.Stderr, "tzgen:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "tzgen: %d zones\n", len(files))
}

func read(dir, name string) (*icutxt.Node, error) {
	b, err := icusrc.ReadTZ(dir, name)
	if err != nil {
		return nil, err
	}
	n, err := icutxt.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s.txt: %w", name, err)
	}
	return n, nil
}

func build(dir string) (map[string]string, error) {
	info, err := read(dir, "zoneinfo64")
	if err != nil {
		return nil, err
	}
	types, err := read(dir, "timezoneTypes")
	if err != nil {
		return nil, err
	}
	names, zones, rules := info.Get("Names"), info.Get("Zones"), info.Get("Rules")
	if names == nil || zones == nil || rules == nil || len(names.Values) != len(zones.Children) {
		return nil, fmt.Errorf("zoneinfo64.txt's names and zones do not line up")
	}
	id := func(key string) string { return strings.ReplaceAll(key, ":", "/") }
	canonicalSet := map[string]bool{}
	if t := types.Get("typeMap", "timezone"); t != nil {
		for _, c := range t.Children {
			canonicalSet[id(c.Key)] = true
		}
	}
	typeAlias := map[string]string{}
	if t := types.Get("typeAlias", "timezone"); t != nil {
		for _, c := range t.Children {
			typeAlias[id(c.Key)] = c.Value
		}
	}

	// linkTarget is the zone a link keeps the rules of, or the zone itself.
	linkTarget := func(i int) int {
		for step := 0; step < 8; step++ {
			z := zones.Children[i]
			if z.Table {
				return i
			}
			j, err := strconv.Atoi(z.Value)
			if err != nil || j < 0 || j >= len(names.Values) {
				return -1
			}
			i = j
		}
		return -1
	}
	// ZoneMeta::getCanonicalCLDRID.
	canonicalOf := func(i int) string {
		name := names.Values[i]
		if canonicalSet[name] {
			return name
		}
		if to, ok := typeAlias[name]; ok {
			return to
		}
		target := name
		if t := linkTarget(i); t >= 0 {
			target = names.Values[t]
		}
		if to, ok := typeAlias[target]; ok {
			return to
		}
		return target
	}

	files := map[string]string{}
	for i, name := range names.Values {
		canonical := canonicalOf(i)
		if canonical == "Etc/Unknown" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := files[key]; dup {
			return nil, fmt.Errorf("zoneinfo64.txt: two names are %s in lowercase", key)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "name %s\ncanonical %s\n", name, canonical)
		t := linkTarget(i)
		if t < 0 {
			return nil, fmt.Errorf("zoneinfo64.txt: %s links nowhere", name)
		}
		if t != i {
			fmt.Fprintf(&b, "link %s\n", strings.ToLower(names.Values[t]))
			files[key] = b.String()
			continue
		}
		if err := writeZone(&b, zones.Children[i], rules); err != nil {
			return nil, fmt.Errorf("zoneinfo64.txt: %s: %w", name, err)
		}
		files[key] = b.String()
	}
	return files, nil
}

// writeZone writes an OlsonTimeZone's offsets.
func writeZone(b *strings.Builder, z, rules *icutxt.Node) error {
	ints := func(key string) ([]int64, error) {
		n := z.Get(key)
		if n == nil {
			return nil, nil
		}
		out := make([]int64, len(n.Values))
		for i, v := range n.Values {
			x, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%s: %q", key, v)
			}
			out[i] = x
		}
		return out, nil
	}
	// Transitions: pairs of 32-bit halves before 1901 and after 2038, and
	// 32-bit seconds between, in order.
	var trans []int64
	pre, err := ints("transPre32")
	if err != nil {
		return err
	}
	for i := 0; i+1 < len(pre); i += 2 {
		trans = append(trans, pre[i]<<32|pre[i+1]&0xFFFFFFFF)
	}
	mid, err := ints("trans")
	if err != nil {
		return err
	}
	trans = append(trans, mid...)
	post, err := ints("transPost32")
	if err != nil {
		return err
	}
	for i := 0; i+1 < len(post); i += 2 {
		trans = append(trans, post[i]<<32|post[i+1]&0xFFFFFFFF)
	}

	offsets, err := ints("typeOffsets")
	if err != nil {
		return err
	}
	if len(offsets) == 0 || len(offsets)%2 != 0 {
		return fmt.Errorf("%d type offsets", len(offsets))
	}
	b.WriteString("types")
	for i := 0; i < len(offsets); i += 2 {
		fmt.Fprintf(b, " %d,%d", offsets[i], offsets[i+1])
	}
	b.WriteString("\n")

	if len(trans) > 0 {
		m := z.Get("typeMap")
		if m == nil {
			return fmt.Errorf("transitions without a type map")
		}
		hex := strings.Trim(strings.TrimSpace(m.Value), "\"")
		if len(hex) != 2*len(trans) {
			return fmt.Errorf("%d transitions, a type map of %d", len(trans), len(hex)/2)
		}
		b.WriteString("trans")
		for i, t := range trans {
			typ, err := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
			if err != nil {
				return fmt.Errorf("type map %q", hex)
			}
			fmt.Fprintf(b, " %d:%d", t, typ)
		}
		b.WriteString("\n")
	}

	if r := z.Get("finalRule"); r != nil && r.Value != "" {
		rule := rules.Get(r.Value)
		raw, year := z.Get("finalRaw"), z.Get("finalYear")
		if rule == nil || len(rule.Values) != 11 || raw == nil || year == nil {
			return fmt.Errorf("final rule %s", r.Value)
		}
		fmt.Fprintf(b, "final %s %s", strings.TrimSpace(raw.Value), strings.TrimSpace(year.Value))
		for _, v := range rule.Values {
			fmt.Fprintf(b, " %s", strings.TrimSpace(v))
		}
		b.WriteString("\n")
	}
	return nil
}

// write replaces data/tz with the files, all built before any is written.
func write(files map[string]string) error {
	tmp := filepath.Join("data", "tz.tmp")
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	for name, text := range files {
		target := filepath.Join(tmp, filepath.FromSlash(name)+".bin")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
			return err
		}
	}
	final := filepath.Join("data", "tz")
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}
