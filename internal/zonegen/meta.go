package main

import (
	"archive/zip"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/zonedata"
)

// readMeta reads what every locale shares about zones, from ICU's data/misc
// and, for the zone files, the time zone update.
func readMeta(archive *zip.ReadCloser, tzDir string) (*zonedata.Meta, error) {
	read := func(name string) (*icutxt.Node, error) {
		b, err := icusrc.ReadFile(archive, "data/misc/"+name+".txt")
		if _, ok := icusrc.TZSHA256[name]; ok {
			b, err = icusrc.ReadTZ(tzDir, name)
		}
		if err != nil {
			return nil, err
		}
		n, err := icutxt.Parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s.txt: %w", name, err)
		}
		return n, nil
	}
	metaZones, err := read("metaZones")
	if err != nil {
		return nil, err
	}
	types, err := read("timezoneTypes")
	if err != nil {
		return nil, err
	}
	zoneinfo, err := read("zoneinfo64")
	if err != nil {
		return nil, err
	}

	// CLDR's canonical identifiers are the keys of the zone type map, and
	// its aliases say which canonical zone an old name means.
	id := func(key string) string { return strings.ReplaceAll(key, ":", "/") }
	canonical := map[string]bool{}
	if t := types.Get("typeMap", "timezone"); t != nil {
		for _, c := range t.Children {
			canonical[id(c.Key)] = true
		}
	}
	typeAlias := map[string]string{}
	if t := types.Get("typeAlias", "timezone"); t != nil {
		for _, c := range t.Children {
			typeAlias[id(c.Key)] = c.Value
		}
	}
	if len(canonical) == 0 {
		return nil, fmt.Errorf("timezoneTypes.txt has no zone type map")
	}

	// zoneinfo64's zones, in the order of their names, with the regions they
	// are in; a zone that is an int is a link to another by index.
	names := zoneinfo.Get("Names")
	zonesArr := zoneinfo.Get("Zones")
	regionsArr := zoneinfo.Get("Regions")
	if names == nil || zonesArr == nil || regionsArr == nil ||
		len(names.Values) != len(zonesArr.Children) || len(names.Values) != len(regionsArr.Values) {
		return nil, fmt.Errorf("zoneinfo64.txt's names, zones and regions do not line up")
	}
	region := map[string]string{}
	for i, name := range names.Values {
		region[name] = regionsArr.Values[i]
	}

	// ZoneMeta::getCanonicalCLDRID: a canonical name is itself; an alias
	// is what the type data says; anything else is dereferenced through
	// zoneinfo64's links, and the target's alias taken if it has one.
	canonicalOf := func(i int) string {
		name := names.Values[i]
		if canonical[name] {
			return name
		}
		if to, ok := typeAlias[name]; ok {
			return to
		}
		target := name
		if z := zonesArr.Children[i]; !z.Table && z.Value != "" {
			j, err := strconv.Atoi(z.Value)
			if err == nil && j >= 0 && j < len(names.Values) {
				target = names.Values[j]
			}
		}
		if to, ok := typeAlias[target]; ok {
			return to
		}
		return target
	}

	m := &zonedata.Meta{}
	zoneSet := map[string]bool{}
	for c := range canonical {
		zoneSet[c] = true
	}
	aliases := map[string]string{}
	for i, name := range names.Values {
		c := canonicalOf(i)
		if c != name {
			aliases[name] = c
		} else {
			zoneSet[name] = true
		}
	}
	for from, to := range typeAlias {
		if _, ok := aliases[from]; !ok && !zoneSet[from] {
			aliases[from] = to
		}
	}

	uses := map[string][]zonedata.Use{}
	if info := metaZones.Get("metazoneInfo"); info != nil {
		for _, c := range info.Children {
			for _, el := range c.Children {
				if len(el.Values) == 0 {
					continue
				}
				u := zonedata.Use{Metazone: el.Values[0]}
				if len(el.Values) == 3 {
					if u.From, err = minutes(el.Values[1]); err != nil {
						return nil, err
					}
					if u.To, err = minutes(el.Values[2]); err != nil {
						return nil, err
					}
				}
				uses[id(c.Key)] = append(uses[id(c.Key)], u)
			}
		}
	}
	for zone := range zoneSet {
		r, ok := region[zone]
		if !ok {
			r = "001"
		}
		m.Zones = append(m.Zones, zonedata.MetaZone{Zone: zone, Region: r, Uses: uses[zone]})
	}
	sort.Slice(m.Zones, func(i, j int) bool { return m.Zones[i].Zone < m.Zones[j].Zone })
	for from, to := range aliases {
		m.Aliases = append(m.Aliases, zonedata.Alias{From: from, To: to})
	}
	sort.Slice(m.Aliases, func(i, j int) bool { return m.Aliases[i].From < m.Aliases[j].From })

	if t := metaZones.Get("mapTimezones"); t != nil {
		for _, mz := range t.Children {
			for _, r := range mz.Children {
				m.Golden = append(m.Golden, zonedata.Golden{Metazone: mz.Key, Region: r.Key, Zone: r.Value})
			}
		}
	}
	sort.Slice(m.Golden, func(i, j int) bool {
		a, b := m.Golden[i], m.Golden[j]
		if a.Metazone != b.Metazone {
			return a.Metazone < b.Metazone
		}
		return a.Region < b.Region
	})
	if t := metaZones.Get("primaryZones"); t != nil {
		for _, c := range t.Children {
			m.Primary = append(m.Primary, zonedata.Alias{From: c.Key, To: c.Value})
		}
	}
	sort.Slice(m.Primary, func(i, j int) bool { return m.Primary[i].From < m.Primary[j].From })
	return m, nil
}

// minutes turns metaZones.txt's "1970-01-01 00:00", which is UTC, into
// minutes since 1970 plus one, so that zero can mean unbounded.
func minutes(s string) (int64, error) {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		return 0, fmt.Errorf("metaZones.txt: %w", err)
	}
	m := t.Unix()/60 + 1
	if m < 1 {
		return 0, fmt.Errorf("metaZones.txt: %s is before 1970", s)
	}
	return m, nil
}
