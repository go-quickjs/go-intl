// Command zonegen builds the time-zone name data from ICU's data sources: the
// zone tree, data/zone/*.txt, for what each locale calls zones; the region
// tree, data/region/*.txt, for the names of the regions zones are in; and
// data/misc for what every locale shares -- metaZones.txt, timezoneTypes.txt
// and zoneinfo64.txt.
//
//	go run ./internal/zonegen <icu4c-78.3-data.zip>
//
// It writes data/zonenames/<locale>.bin for every locale with date data, and
// data/metazones.bin. The locales are resolved as ICU's resource bundles
// resolve them, with ICU's own fallback tables (internal/icusrc).
//
// Why ICU's sources and not CLDR's JSON: ICU reads zone names field by field
// along its own fallback chain, and a locale ICU keeps no zone bundle for
// falls back by ICU's resource rules rather than CLDR's. cldr-json resolves
// each locale by CLDR's, and writes "sr-Cyrl-ME" with names ICU never gives
// it.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/zonedata"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/zonegen <icu4c-78.3-data.zip> <icu-tz-2026c dir>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "zonegen:", err)
		os.Exit(1)
	}
}

// noInheritance is CLDR's marker for a value a locale deliberately leaves
// empty, which stops its parents' value from showing through.
const noInheritance = "∅∅∅"

func run(zipPath, tzDir string) error {
	archive, err := icusrc.Open(zipPath, icusrc.DataSHA256)
	if err != nil {
		return err
	}
	defer archive.Close()
	meta, err := readMeta(archive, tzDir)
	if err != nil {
		return err
	}
	zones, err := icusrc.OpenTree(zipPath, "zone")
	if err != nil {
		return err
	}
	defer zones.Close()
	regions, err := icusrc.OpenTree(zipPath, "region")
	if err != nil {
		return err
	}
	defer regions.Close()
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}

	// The regions a zone is in, which are the ones a zone is ever named by.
	codes := map[string]bool{}
	for _, z := range meta.Zones {
		if z.Region != "001" {
			codes[z.Region] = true
		}
	}

	// The locales are the date data's, each written once with what it
	// shares with another named in its same.bin.
	dates, err := datawrite.Tags(filepath.Join("data", "dates"))
	if err != nil {
		return err
	}
	if len(dates) == 0 {
		return fmt.Errorf("no locales under data/dates; run dategen first")
	}
	built := map[string][]byte{}
	// The locales share one pool of names, written beside them, in the
	// sorted order Tags gives, so the pool is the same every run.
	pool := blob.NewPool(zonedata.Version)
	for _, tag := range dates {
		name := strings.ReplaceAll(tag, "-", "_")
		if tag == "und" {
			name = "root"
		}
		l, err := readLocale(zones, regions, fb, name, codes)
		if err != nil {
			return fmt.Errorf("%s: %w", tag, err)
		}
		built[tag] = zonedata.Encode(l, pool)
	}

	metaBytes, err := zonedata.EncodeMeta(meta)
	if err != nil {
		return err
	}

	out := filepath.Join("data", "zonenames")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := datawrite.Locales(out, built); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "zonenamesshared.bin"), pool.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "metazones.bin"), metaBytes, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "zonegen: %d locales, %d zones, %d aliases\n", len(built), len(meta.Zones), len(meta.Aliases))
	return nil
}

// readLocale merges what a locale's zone and region bundles say, as ICU's
// TimeZoneNames and LocaleDisplayNames read them.
func readLocale(zones, regions *icusrc.Locales, fb icusrc.Fallback, name string, codes map[string]bool) (*zonedata.Built, error) {
	chain, err := zones.Resolve(name, fb)
	if err != nil {
		return nil, err
	}
	var tables []*icutxt.Node
	for _, n := range chain {
		if t := n.Get("zoneStrings"); t != nil {
			tables = append(tables, t)
		}
	}
	first := func(key string) string {
		for _, t := range tables {
			if n := t.Get(key); n != nil && n.Value != "" {
				return n.Value
			}
		}
		return ""
	}
	l := &zonedata.Built{
		GMTFormat:      first("gmtFormat"),
		HourFormat:     first("hourFormat"),
		RegionFormat:   first("regionFormat"),
		FallbackFormat: first("fallbackFormat"),
	}

	// Every zone and metazone any bundle in the chain names, each merged
	// field by field: the first bundle to give a field decides it, and one
	// that gives the no-inheritance marker decides it is empty.
	keys := map[string]bool{}
	for _, t := range tables {
		for _, c := range t.Children {
			if c.Table && strings.Contains(c.Key, ":") {
				keys[c.Key] = true
			}
		}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, key := range sorted {
		fields := map[string]string{}
		for _, t := range tables {
			n := t.Get(key)
			if n == nil {
				continue
			}
			for _, c := range n.Children {
				if _, done := fields[c.Key]; !done {
					fields[c.Key] = c.Value
				}
			}
		}
		for k, v := range fields {
			if v == noInheritance {
				fields[k] = ""
			}
		}
		names := zonedata.Names{
			LongGeneric: fields["lg"], LongStandard: fields["ls"], LongDaylight: fields["ld"],
			ShortGeneric: fields["sg"], ShortStandard: fields["ss"], ShortDaylight: fields["sd"],
		}
		if metazone, ok := strings.CutPrefix(key, "meta:"); ok {
			if !names.Empty() {
				l.Metazones = append(l.Metazones, zonedata.Entry{Metazone: metazone, Names: names})
			}
			continue
		}
		if names.Empty() && fields["ec"] == "" {
			continue
		}
		l.Zones = append(l.Zones, zonedata.ZoneEntry{
			Zone: strings.ReplaceAll(key, ":", "/"), Names: names, City: fields["ec"],
		})
	}
	sort.Slice(l.Metazones, func(i, j int) bool { return l.Metazones[i].Metazone < l.Metazones[j].Metazone })
	sort.Slice(l.Zones, func(i, j int) bool { return l.Zones[i].Zone < l.Zones[j].Zone })

	// Region names are looked up one by one along the region tree's chain.
	regionChain, err := regions.Resolve(name, fb)
	if err != nil {
		return nil, err
	}
	for code := range codes {
		for _, n := range regionChain {
			if v := n.Get("Countries", code); v != nil && v.Value != "" {
				l.Regions = append(l.Regions, zonedata.RegionEntry{Region: code, Name: v.Value})
				break
			}
		}
	}
	sort.Slice(l.Regions, func(i, j int) bool { return l.Regions[i].Region < l.Regions[j].Region })
	return l, nil
}
