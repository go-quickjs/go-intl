package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// writeValues writes data/values.bin: the lists Intl.supportedValuesOf
// answers with for collations, currencies and time zones, as V8 builds them
// from ICU (intl-objects.cc), one "<key> <value>" line each; and for
// Intl.Locale, each region's canonical zones ("zone <region> <id>"), the
// collation types' BCP 47 spellings ("cotype phonebook phonebk") and the
// scripts ICU writes right to left ("rtl Arab"). Calendars, numbering
// systems and units are go-intl's own lists.
func writeValues(zip string) error {
	var lines []string
	collations, err := collationValues(zip)
	if err != nil {
		return err
	}
	for _, v := range collations {
		lines = append(lines, "collation "+v)
	}
	currencies, err := currencyValues(zip)
	if err != nil {
		return err
	}
	for _, v := range currencies {
		lines = append(lines, "currency "+v)
	}
	zones, err := zoneValues(zip)
	if err != nil {
		return err
	}
	for _, v := range zones {
		lines = append(lines, "timezone "+v)
	}
	regional, err := regionZones(zip)
	if err != nil {
		return err
	}
	lines = append(lines, regional...)
	// The collation types' BCP 47 spellings where ICU's data spells them
	// otherwise, "cotype phonebook phonebk", for the names Intl.Locale lists.
	keyTypes, err := readMisc(zip, "keyTypeData")
	if err != nil {
		return err
	}
	if t := keyTypes.Get("typeMap", "collation"); t != nil {
		var cotypes []string
		for _, e := range t.Children {
			if e.Value != "" && e.Value != e.Key {
				cotypes = append(cotypes, "cotype "+e.Key+" "+e.Value)
			}
		}
		sort.Strings(cotypes)
		lines = append(lines, cotypes...)
	}
	rtl, err := icusrc.RightToLeftScripts()
	if err != nil {
		return err
	}
	sort.Strings(rtl)
	for _, script := range rtl {
		lines = append(lines, "rtl "+script)
	}
	target := filepath.Join("data", "values.bin")
	if err := os.WriteFile(target+".tmp", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(target+".tmp", target)
}

func readMisc(zip, name string) (*icutxt.Node, error) {
	l, err := openLocales(zip)
	if err != nil {
		return nil, err
	}
	defer l.Close()
	b, err := l.ReadMisc(name)
	if err != nil {
		return nil, err
	}
	n, err := icutxt.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s.txt: %w", name, err)
	}
	return n, nil
}

// collationValues is AvailableCollations: every collation the collation
// tree's bundles name (Collator::getKeywordValues), in its BCP 47 spelling,
// but "standard" and "search", sorted.
func collationValues(zip string) ([]string, error) {
	coll, err := icusrc.OpenTree(zip, "coll")
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	keyTypes, err := readMisc(zip, "keyTypeData")
	if err != nil {
		return nil, err
	}
	bcp := map[string]string{}
	if t := keyTypes.Get("typeMap", "collation"); t != nil {
		for _, e := range t.Children {
			if e.Value != "" {
				bcp[e.Key] = e.Value
			}
		}
	}
	set := map[string]bool{}
	for _, name := range coll.Names() {
		n, err := coll.Get(name)
		if err != nil {
			return nil, err
		}
		if n == nil {
			continue
		}
		t := n.Get("collations")
		if t == nil || !t.Table {
			continue
		}
		for _, c := range t.Children {
			if c.Key == "default" || strings.HasPrefix(c.Key, "private-") {
				continue
			}
			v := c.Key
			if b, ok := bcp[v]; ok {
				v = b
			}
			if v != "standard" && v != "search" {
				set[v] = true
			}
		}
	}
	return sorted(set), nil
}

// currencyValues is ResourceAvailableCurrencies: ICU's common, current ISO
// currencies (gCurrencyList), and four V8 adds, that have an English name
// other than their code, without VEF, sorted.
func currencyValues(zip string) ([]string, error) {
	list, err := icusrc.CurrencyList()
	if err != nil {
		return nil, err
	}
	curr, err := icusrc.OpenTree(zip, "curr")
	if err != nil {
		return nil, err
	}
	defer curr.Close()
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return nil, err
	}
	chain, err := curr.Resolve("en", fb)
	if err != nil {
		return nil, err
	}
	// ucurr_getName's long name: the second element of the currency's entry,
	// through the chain; the code itself when there is none.
	named := func(code string) bool {
		for _, n := range chain {
			if e := n.Get("Currencies", code); e != nil && len(e.Values) >= 2 {
				return e.Values[1] != code
			}
		}
		return false
	}
	set := map[string]bool{}
	for _, c := range list {
		common, current := false, false
		for _, f := range c.Flags {
			common = common || f == "UCURR_COMMON"
			current = current || f == "UCURR_NON_DEPRECATED"
		}
		if common && current && c.Code != "VEF" && named(c.Code) {
			set[c.Code] = true
		}
	}
	for _, code := range []string{"SVC", "XDR", "XSU", "ZWL"} {
		if named(code) {
			set[code] = true
		}
	}
	return sorted(set), nil
}

// zoneValues is AvailableTimeZones: ICU's zones
// (UCAL_ZONE_TYPE_CANONICAL_LOCATION) that are canonical, not an alias, and
// in a region rather than the world, but Etc/Unknown, sorted.
func zoneValues(zip string) ([]string, error) {
	info, err := readMisc(zip, "zoneinfo64")
	if err != nil {
		return nil, err
	}
	types, err := readMisc(zip, "timezoneTypes")
	if err != nil {
		return nil, err
	}
	names, regions := info.Get("Names"), info.Get("Regions")
	if names == nil || regions == nil || len(names.Values) != len(regions.Values) {
		return nil, fmt.Errorf("zoneinfo64.txt: Names and Regions do not agree")
	}
	canonical, err := canonicalZones(zip, types, names.Values)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for i, id := range names.Values {
		if id == "Etc/Unknown" || !canonical(id) || regions.Values[i] == "001" {
			continue
		}
		set[id] = true
	}
	return sorted(set), nil
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// regionZones is what TimeZone::createTimeZoneIDEnumeration lists for a
// region with UCAL_ZONE_TYPE_CANONICAL: the canonical zones, aliases and
// Etc/Unknown left out, whose region it is, as "zone <region> <id>" lines
// sorted by region and zone.
func regionZones(zip string) ([]string, error) {
	info, err := readMisc(zip, "zoneinfo64")
	if err != nil {
		return nil, err
	}
	types, err := readMisc(zip, "timezoneTypes")
	if err != nil {
		return nil, err
	}
	names, regions := info.Get("Names"), info.Get("Regions")
	canonical, err := canonicalZones(zip, types, names.Values)
	if err != nil {
		return nil, err
	}
	var out []string
	for i, id := range names.Values {
		if id == "Etc/Unknown" || !canonical(id) {
			continue
		}
		out = append(out, "zone "+regions.Values[i]+" "+id)
	}
	sort.Strings(out)
	return out, nil
}

// canonicalZones is ZoneMeta::getCanonicalCLDRID's test of whether a zone is
// its own canonical ID: it is a canonical type in keyTypeData, or it is
// neither an alias there nor a link in the tz data, as the SystemV zones are.
func canonicalZones(zip string, types *icutxt.Node, names []string) (func(string) bool, error) {
	typeMap := map[string]bool{}
	if t := types.Get("typeMap", "timezone"); t != nil {
		for _, e := range t.Children {
			typeMap[strings.ReplaceAll(e.Key, ":", "/")] = true
		}
	}
	typeAlias := map[string]bool{}
	if t := types.Get("typeAlias", "timezone"); t != nil {
		for _, e := range t.Children {
			typeAlias[strings.ReplaceAll(e.Key, ":", "/")] = true
		}
	}
	l, err := openLocales(zip)
	if err != nil {
		return nil, err
	}
	defer l.Close()
	raw, err := l.ReadMisc("zoneinfo64")
	if err != nil {
		return nil, err
	}
	// The Zones array holds each name's entry in Names' order: a table for
	// a zone, an integer, the index of its target, for a link.
	links := map[string]bool{}
	entry := regexp.MustCompile(`(?m)^  /\* (\S+) \*/ :(int|table) \{`)
	matches := entry.FindAllStringSubmatch(string(raw), -1)
	if len(matches) != len(names) {
		return nil, fmt.Errorf("zoneinfo64.txt: %d zone entries for %d names", len(matches), len(names))
	}
	for _, m := range matches {
		if m[2] == "int" {
			links[m[1]] = true
		}
	}
	return func(id string) bool {
		return typeMap[id] || !typeAlias[id] && !links[id]
	}, nil
}
