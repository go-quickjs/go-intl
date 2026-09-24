// Command zonegen writes the time-zone name tables the intl package carries.
//
// It reads CLDR's cldr-dates-full package for the names and the vendored
// metaZones.json for the mapping from a zone to the metazone it belongs to.
//
//	go run ./internal/zonegen <cldr-dates-full/package>
//
// A locale does not name zones one by one: it names metazones, the Eastern
// Time a dozen American zones share. The mapping has changed over the years
// and CLDR records the ranges; the current one is what is written here, since
// that is what a zone is called now.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/zonedata"
)

//go:embed metaZones.json
var metaZonesJSON []byte

//go:embed timezone.json
var timezoneJSON []byte

type namesFile struct {
	Main map[string]struct {
		Dates struct {
			TimeZoneNames struct {
				HourFormat string                     `json:"hourFormat"`
				GMTFormat  string                     `json:"gmtFormat"`
				GMTZero    string                     `json:"gmtZeroFormat"`
				Zone       map[string]json.RawMessage `json:"zone"`
				Metazone   map[string]widths          `json:"metazone"`
			} `json:"timeZoneNames"`
		} `json:"dates"`
	} `json:"main"`
}

type widths struct {
	Long  forms `json:"long"`
	Short forms `json:"short"`
}

type forms struct {
	Generic  string `json:"generic"`
	Standard string `json:"standard"`
	Daylight string `json:"daylight"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/zonegen <cldr-dates-full/package>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "zonegen:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	mapping, err := zoneMetazones()
	if err != nil {
		return err
	}

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
		built[e.Name()] = zonedata.Encode(l)
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}

	out := filepath.Join("data", "zonenames")
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
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(out, name+".bin"), built[name], 0o644); err != nil {
			return err
		}
	}

	// The zone-to-metazone mapping does not vary by locale, so it is one
	// table: lines of "zone metazone", sorted.
	var b strings.Builder
	zones := make([]string, 0, len(mapping))
	for zone := range mapping {
		zones = append(zones, zone)
	}
	sort.Strings(zones)
	for _, zone := range zones {
		fmt.Fprintf(&b, "%s %s\n", zone, mapping[zone])
	}
	if err := os.WriteFile(filepath.Join("data", "metazones.bin"), []byte(b.String()), 0o644); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "zonegen: %d locales, %d zones mapped\n", len(names), len(zones))
	return nil
}

// zoneMetazones reads which metazone each zone belongs to now. A zone that has
// moved between metazones has several entries, and the one with no end is the
// current one.
func zoneMetazones() (map[string]string, error) {
	var res struct {
		Supplemental struct {
			MetaZones struct {
				MetazoneInfo struct {
					Timezone map[string]json.RawMessage `json:"timezone"`
				} `json:"metazoneInfo"`
			} `json:"metaZones"`
		} `json:"supplemental"`
	}
	if err := json.Unmarshal(metaZonesJSON, &res); err != nil {
		return nil, fmt.Errorf("metaZones.json: %w", err)
	}
	out := map[string]string{}
	for region, raw := range res.Supplemental.MetaZones.MetazoneInfo.Timezone {
		walkZones(region, raw, out)
	}

	// CLDR files a zone under the name it had when the metazone was recorded,
	// so India is "Asia/Calcutta" and a caller asking for "Asia/Kolkata" would
	// find nothing. Every name a zone goes by is given the same metazone.
	aliases, err := zoneAliases()
	if err != nil {
		return nil, err
	}
	for _, names := range aliases {
		var metazone string
		for _, name := range names {
			if m, ok := out[name]; ok {
				metazone = m
				break
			}
		}
		if metazone == "" {
			continue
		}
		for _, name := range names {
			out[name] = metazone
		}
	}
	return out, nil
}

// zoneAliases returns the groups of names that mean the same zone.
func zoneAliases() ([][]string, error) {
	var res struct {
		Keyword struct {
			U struct {
				TZ map[string]json.RawMessage `json:"tz"`
			} `json:"u"`
		} `json:"keyword"`
	}
	if err := json.Unmarshal(timezoneJSON, &res); err != nil {
		return nil, fmt.Errorf("timezone.json: %w", err)
	}
	var out [][]string
	for _, raw := range res.Keyword.U.TZ {
		var entry struct {
			Alias string `json:"_alias"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil || entry.Alias == "" {
			continue
		}
		names := strings.Fields(entry.Alias)
		if len(names) > 1 {
			out = append(out, names)
		}
	}
	return out, nil
}

type usage struct {
	UsesMetazone struct {
		Mzone string `json:"_mzone"`
		From  string `json:"_from"`
		To    string `json:"_to"`
	} `json:"usesMetazone"`
}

// walkZones descends the tree of zone names, which is nested by the parts of
// the zone's own name: America, then New_York.
func walkZones(prefix string, raw json.RawMessage, out map[string]string) {
	var list []usage
	if err := json.Unmarshal(raw, &list); err == nil {
		for _, u := range list {
			// The entry with no end is the one in force now.
			if u.UsesMetazone.To == "" && u.UsesMetazone.Mzone != "" {
				out[prefix] = u.UsesMetazone.Mzone
			}
		}
		return
	}
	var children map[string]json.RawMessage
	if err := json.Unmarshal(raw, &children); err != nil {
		return
	}
	for name, child := range children {
		walkZones(prefix+"/"+name, child, out)
	}
}

func read(main, name string) (*zonedata.Locale, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, "timeZoneNames.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f namesFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("timeZoneNames.json: %w", err)
	}
	entry, ok := f.Main[name]
	if !ok {
		return nil, nil
	}
	src := entry.Dates.TimeZoneNames

	out := &zonedata.Locale{
		GMTFormat: src.GMTFormat, HourFormat: src.HourFormat, GMTZero: src.GMTZero,
	}
	for metazone, w := range src.Metazone {
		names := zonedata.Names{
			LongGeneric: w.Long.Generic, LongStandard: w.Long.Standard,
			LongDaylight: w.Long.Daylight, ShortGeneric: w.Short.Generic,
			ShortStandard: w.Short.Standard, ShortDaylight: w.Short.Daylight,
		}
		if names.Empty() {
			continue
		}
		out.Metazones = append(out.Metazones, zonedata.Entry{Metazone: metazone, Names: names})
	}
	sort.Slice(out.Metazones, func(i, j int) bool {
		return out.Metazones[i].Metazone < out.Metazones[j].Metazone
	})

	for region, raw := range src.Zone {
		walkZoneNames(region, raw, out)
	}
	sort.Slice(out.Zones, func(i, j int) bool { return out.Zones[i].Zone < out.Zones[j].Zone })

	if out.GMTFormat == "" && len(out.Metazones) == 0 {
		return nil, nil
	}
	return out, nil
}

// walkZoneNames finds the few zones a locale names itself rather than through
// a metazone.
func walkZoneNames(prefix string, raw json.RawMessage, out *zonedata.Locale) {
	var w widths
	if err := json.Unmarshal(raw, &w); err == nil {
		names := zonedata.Names{
			LongGeneric: w.Long.Generic, LongStandard: w.Long.Standard,
			LongDaylight: w.Long.Daylight, ShortGeneric: w.Short.Generic,
			ShortStandard: w.Short.Standard, ShortDaylight: w.Short.Daylight,
		}
		if !names.Empty() {
			out.Zones = append(out.Zones, zonedata.ZoneEntry{Zone: prefix, Names: names})
			return
		}
	}
	var children map[string]json.RawMessage
	if err := json.Unmarshal(raw, &children); err != nil {
		return
	}
	for name, child := range children {
		walkZoneNames(prefix+"/"+name, child, out)
	}
}
