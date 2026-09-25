// Command aliasgen writes data/aliases.bin: the aliases a locale identifier's
// canonical form replaces, as ICU 78.3 holds them.
//
//	go run ./internal/aliasgen icu4c-78.3-data.zip <icu-tz-2026c dir>
//
// The subtag aliases are CLDR's supplementalMetadata, from the data archive's
// misc/metadata.txt; the Unicode extension types are misc/keyTypeData.txt and
// misc/timezoneTypes.txt, the last from ICU's time zone update 2026c, which
// is what Node runs (see icusrc.TZSHA256). ICU is read rather than cldr-json because what
// Node answers is ICU's canonicalization, and ICU's tables are what it
// consults.
//
// The file is lines of text, one alias each:
//
//	language aa_saaho ssy
//	territory SU RU AM AZ BY EE GE KZ KG LV LT MD TJ TM UA UZ
//	script Qaai Zinh
//	variant heploc alalc97
//	subdivision cn11 cnbj
//	type ca islamicc islamic-civil
//	legacy art-lojban jbo
//	redundant sgn-no nsl
//
// A type line is a Unicode extension key, a value a tag may carry and the
// canonical value it becomes: every spelling ICU's keyTypeData accepts for
// a type -- its BCP 47 id, its legacy id, an alias of either -- written as
// the BCP 47 id. Only the spellings that differ from their canonical form,
// and that a tag can hold, are written.
//
// The legacy and redundant lines are the whole tags ICU's parser rewrites
// before anything else, in its order, from its source (uloc_tag.cpp, vendored
// in internal/icusrc).
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

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/aliasgen <icu4c-78.3-data.zip> <icu-tz-2026c dir>")
		os.Exit(2)
	}
	out, err := build(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "aliasgen:", err)
		os.Exit(1)
	}
	target := filepath.Join("data", "aliases.bin")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "aliasgen:", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, target); err != nil {
		fmt.Fprintln(os.Stderr, "aliasgen:", err)
		os.Exit(1)
	}
}

func build(zip, tzDir string) ([]byte, error) {
	icu, err := icusrc.OpenLocales(zip)
	if err != nil {
		return nil, err
	}
	defer icu.Close()
	icu.UseTZ(tzDir)
	misc := func(name string) (*icutxt.Node, error) {
		b, err := icu.ReadMisc(name)
		if err != nil {
			return nil, err
		}
		n, err := icutxt.Parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s.txt: %w", name, err)
		}
		return n, nil
	}
	metadata, err := misc("metadata")
	if err != nil {
		return nil, err
	}
	keyTypes, err := misc("keyTypeData")
	if err != nil {
		return nil, err
	}
	zones, err := misc("timezoneTypes")
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, table := range []string{"language", "territory", "script", "variant", "subdivision"} {
		t := metadata.Get("alias", table)
		if t == nil || !t.Table {
			return nil, fmt.Errorf("metadata.txt has no %s aliases", table)
		}
		var entries []string
		for _, e := range t.Children {
			r := e.Get("replacement")
			if r == nil {
				return nil, fmt.Errorf("metadata.txt: %s alias %s has no replacement", table, e.Key)
			}
			entries = append(entries, table+" "+e.Key+" "+r.Value)
		}
		sort.Strings(entries)
		lines = append(lines, entries...)
	}

	types, err := typeAliases(keyTypes, zones)
	if err != nil {
		return nil, err
	}
	lines = append(lines, types...)

	legacy, redundant, err := icusrc.LegacyTags()
	if err != nil {
		return nil, err
	}
	for _, p := range legacy {
		lines = append(lines, "legacy "+p[0]+" "+p[1])
	}
	for _, p := range redundant {
		lines = append(lines, "redundant "+p[0]+" "+p[1])
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// bcpShaped is what a Unicode extension value can hold: subtags of three to
// eight letters or digits.
var bcpShaped = regexp.MustCompile(`^[a-z0-9]{3,8}(-[a-z0-9]{3,8})*$`)

// typeAliases builds the type lines as ICU's uloc_keytype.cpp builds its
// type map: under each key, every legacy id, BCP id and alias leads to one
// type, written as its BCP id.
func typeAliases(keyTypes, zones *icutxt.Node) ([]string, error) {
	keyMap := keyTypes.Get("keyMap")
	typeMap := keyTypes.Get("typeMap")
	if keyMap == nil || typeMap == nil {
		return nil, fmt.Errorf("keyTypeData.txt has no keyMap or typeMap")
	}
	// A table here may be an alias into timezoneTypes.
	resolve := func(n *icutxt.Node) *icutxt.Node {
		if n != nil && n.Alias {
			path := strings.TrimPrefix(n.Value, "/ICUDATA/timezoneTypes/")
			return zones.Get(strings.Split(path, "/")...)
		}
		return n
	}
	var lines []string
	for _, legacyKey := range typeMap.Children {
		bcpKey := legacyKey.Key
		if k := keyMap.Get(legacyKey.Key); k != nil && k.Value != "" {
			bcpKey = k.Value
		}
		types := resolve(legacyKey)
		if types == nil || !types.Table {
			continue
		}
		to := map[string]string{} // legacy id to BCP id
		from := map[string]string{}
		for _, t := range types.Children {
			legacy := strings.ReplaceAll(t.Key, ":", "/")
			bcp := t.Value
			if bcp == "" {
				bcp = legacy
			}
			to[legacy] = bcp
			from[strings.ToLower(legacy)] = bcp
			from[bcp] = bcp
		}
		if a := resolve(keyTypes.Get("typeAlias", legacyKey.Key)); a != nil && a.Table {
			for _, e := range a.Children {
				if bcp, ok := to[e.Value]; ok {
					from[strings.ToLower(strings.ReplaceAll(e.Key, ":", "/"))] = bcp
				}
			}
		}
		if a := resolve(keyTypes.Get("bcpTypeAlias", bcpKey)); a != nil && a.Table {
			for _, e := range a.Children {
				from[e.Key] = e.Value
			}
		}
		var entries []string
		for f, t := range from {
			if f != t && bcpShaped.MatchString(f) {
				entries = append(entries, "type "+bcpKey+" "+f+" "+t)
			}
		}
		sort.Strings(entries)
		lines = append(lines, entries...)
	}
	return lines, nil
}
