// Command tzgen writes the time zones the intl package reckons offsets in:
// ICU's zoneinfo64, from its time zone update 2026c, which is what Node runs
// (see icusrc.TZSHA256).
//
//	go run ./internal/tzgen <icu-tz-2026c dir>
//
// Each zone ICU knows by name has a file, data/tz/<name>.bin, the name in
// lowercase, so that a name is valid when its file exists, in any case, as
// ECMA-402 matches names, and a formatter reads only its own zone. A file is
// a tzdata.Zone:
//
//	name       Europe/Paris
//	canonical  Europe/Paris
//	types      561,0 0,0 0,3600 0,7200 3600,0 3600,3600
//	trans      -1855958961:1 -1689814800:2 -1680397200:1 ...
//	final      3600 1997 2 -31 -1 3600 2 9 -31 -1 3600 2 3600
//
// The name is as ICU spells it, and the canonical name the one
// ZoneMeta::getCanonicalCLDRID gives it, which resolvedOptions reports and
// zone names are keyed by. A name that is a link in the tz data has the zone
// whose rules it keeps, "asia/kolkata" for Asia/Calcutta, and nothing else.
// A zone has its offsets as ICU's OlsonTimeZone keeps them: the types, each
// a raw offset and a daylight saving, in seconds, the first in force before
// any transition; the transitions, each an instant, in seconds since 1970,
// and the type it starts; and, when the zone ends in a rule, the raw offset,
// the year the rule governs from, and ICU's eleven numbers for the rule:
// start month, day, day of week, time and time mode, the same for the end,
// and the saving.
//
// data/windowszones.bin is windowsZones.txt's mapTimezones, which ICU
// consults to name the zone a Windows machine is set to: a line per Windows
// zone and region, tab-separated, with the zones CLDR lists for it, the
// first being the one ICU takes.
//
//	Eastern Standard Time	001	America/New_York
//	Eastern Standard Time	US	America/New_York America/Detroit ...
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
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/tzdata"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/tzgen <icu-tz-2026c dir>")
		os.Exit(2)
	}
	files, err := build(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "tzgen:", err)
		os.Exit(1)
	}
	windows, err := buildWindows(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "tzgen:", err)
		os.Exit(1)
	}
	if err := write(files, windows); err != nil {
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

func build(dir string) (map[string][]byte, error) {
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

	files := map[string][]byte{}
	for i, name := range names.Values {
		canonical := canonicalOf(i)
		if canonical == "Etc/Unknown" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := files[key]; dup {
			return nil, fmt.Errorf("zoneinfo64.txt: two names are %s in lowercase", key)
		}
		zone := &tzdata.Zone{Name: name, Canonical: canonical}
		t := linkTarget(i)
		if t < 0 {
			return nil, fmt.Errorf("zoneinfo64.txt: %s links nowhere", name)
		}
		if t != i {
			zone.Link = strings.ToLower(names.Values[t])
		} else if err := readZone(zone, zones.Children[i], rules); err != nil {
			return nil, fmt.Errorf("zoneinfo64.txt: %s: %w", name, err)
		}
		files[key] = tzdata.Encode(zone)
	}
	return files, nil
}

// readZone reads an OlsonTimeZone's offsets.
func readZone(out *tzdata.Zone, z, rules *icutxt.Node) error {
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
	for i := 0; i < len(offsets); i += 2 {
		raw, dst, err := int32s(offsets[i], offsets[i+1])
		if err != nil {
			return fmt.Errorf("type offsets: %w", err)
		}
		out.Types = append(out.Types, tzdata.Offset{Raw: raw, DST: dst})
	}

	if len(trans) > 0 {
		m := z.Get("typeMap")
		if m == nil {
			return fmt.Errorf("transitions without a type map")
		}
		hex := strings.Trim(strings.TrimSpace(m.Value), "\"")
		if len(hex) != 2*len(trans) {
			return fmt.Errorf("%d transitions, a type map of %d", len(trans), len(hex)/2)
		}
		for i, t := range trans {
			typ, err := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
			if err != nil {
				return fmt.Errorf("type map %q", hex)
			}
			out.Trans = append(out.Trans, t)
			out.TransTypes = append(out.TransTypes, uint8(typ))
		}
	}

	if r := z.Get("finalRule"); r != nil && r.Value != "" {
		rule := rules.Get(r.Value)
		raw, year := z.Get("finalRaw"), z.Get("finalYear")
		if rule == nil || len(rule.Values) != 11 || raw == nil || year == nil {
			return fmt.Errorf("final rule %s", r.Value)
		}
		for _, v := range append([]string{raw.Value, year.Value}, rule.Values...) {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
			if err != nil {
				return fmt.Errorf("final rule %s: %q", r.Value, v)
			}
			out.Final = append(out.Final, int32(n))
		}
	}
	return nil
}

// int32s narrows two offsets, which ICU keeps as 32-bit numbers.
func int32s(a, b int64) (int32, int32, error) {
	if a != int64(int32(a)) || b != int64(int32(b)) {
		return 0, 0, fmt.Errorf("%d,%d do not fit 32 bits", a, b)
	}
	return int32(a), int32(b), nil
}

// buildWindows writes windowsZones.txt's mapTimezones as lines.
func buildWindows(dir string) ([]byte, error) {
	n, err := read(dir, "windowsZones")
	if err != nil {
		return nil, err
	}
	m := n.Get("mapTimezones")
	if m == nil || len(m.Children) == 0 {
		return nil, fmt.Errorf("windowsZones.txt has no mapTimezones")
	}
	var lines []string
	for _, zone := range m.Children {
		for _, region := range zone.Children {
			if strings.ContainsAny(zone.Key+region.Key+region.Value, "\t\n") || region.Value == "" {
				return nil, fmt.Errorf("windowsZones.txt: %s %s: %q", zone.Key, region.Key, region.Value)
			}
			lines = append(lines, zone.Key+"\t"+region.Key+"\t"+region.Value)
		}
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// write replaces data/tz and data/windowszones.bin, all built before any is
// written. The names are ICU's, and so safe as paths.
func write(files map[string][]byte, windows []byte) error {
	tmp := filepath.Join("data", "tz.tmp")
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	for name, b := range files {
		target := filepath.Join(tmp, filepath.FromSlash(name)+".bin")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return err
		}
	}
	winTmp := filepath.Join("data", "windowszones.bin.tmp")
	if err := os.WriteFile(winTmp, windows, 0o644); err != nil {
		return err
	}
	final := filepath.Join("data", "tz")
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	return os.Rename(winTmp, filepath.Join("data", "windowszones.bin"))
}
