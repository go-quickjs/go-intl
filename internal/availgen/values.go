package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// writeValues writes data/values.bin: the lists Intl.supportedValuesOf
// answers with for collations, currencies and time zones, as V8 builds them
// from ICU (intl-objects.cc), one "<key> <value>" line each. Calendars,
// numbering systems and units are go-intl's own lists.
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
	target := filepath.Join("data", "values.bin")
	if err := os.WriteFile(target+".tmp", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(target+".tmp", target)
}

func readMisc(zip, name string) (*icutxt.Node, error) {
	l, err := icusrc.OpenLocales(zip)
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
	canonical := map[string]bool{}
	if t := types.Get("typeMap", "timezone"); t != nil {
		for _, e := range t.Children {
			canonical[strings.ReplaceAll(e.Key, ":", "/")] = true
		}
	}
	set := map[string]bool{}
	for i, id := range names.Values {
		if id == "Etc/Unknown" || !canonical[id] || regions.Values[i] == "001" {
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
