// Command availgen writes data/available.bin: the locales each service is
// available in, as V8 builds its lists from ICU 78.3's.
//
//	go run ./internal/availgen icu4c-78.3-data.zip
//
// ECMA-402's locale negotiation resolves a request among a service's
// available locales, and which locales those are decides answers: ICU has no
// data for "az-Arab", so V8 resolves it as "az". V8's lists
// (Intl::BuildLocaleSet, intl-objects.cc) start from ICU's installed locales,
// keep those whose bundle, or its language's, holds what the service reads --
// "NumberElements" for numbers, "calendar" for dates, "listPattern" for lists
// -- and add each one's form without its script, "zh-HK" for "zh-Hant-HK".
// The Collator's list is the collation tree's own, and PluralRules' is the
// languages ICU has plural rules for.
//
// ICU's installed locales are what its build indexes (BUILDRULES.py): every
// bundle of the tree but root and a few deprecated names, and, in the list
// V8 asks for, the alias bundles LOCALE_DEPS.json names.
//
// The file is lines of a service and a tag: "number zh-Hant-HK"; and the
// redirects ICU's bundles make, per tree (see redirectLines).
package main

import (
	"encoding/json"
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
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/availgen <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	out, err := build(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "availgen:", err)
		os.Exit(1)
	}
	target := filepath.Join("data", "available.bin")
	if err := os.WriteFile(target+".tmp", out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "availgen:", err)
		os.Exit(1)
	}
	if err := os.Rename(target+".tmp", target); err != nil {
		fmt.Fprintln(os.Stderr, "availgen:", err)
		os.Exit(1)
	}
}

// excluded are the bundles ICU's build leaves out of its index.
var excluded = map[string]bool{
	"ja_JP_TRADITIONAL": true, "th_TH_TRADITIONAL": true, "de_": true, "de__PHONEBOOK": true,
	"es_": true, "es__TRADITIONAL": true, "root": true,
}

// installed is a tree's index: what uloc_openAvailableByType lists, with
// the alias bundles when withAliases is set, as V8 asks for them
// (ULOC_AVAILABLE_WITH_LEGACY_ALIASES); the Collator's list has none.
func installed(t *icusrc.Locales, withAliases bool) ([]string, error) {
	deps, err := t.ReadTreeFile("LOCALE_DEPS.json")
	if err != nil {
		return nil, err
	}
	// The file opens with comments, which encoding/json refuses.
	text := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(deps), "")
	var d struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		return nil, fmt.Errorf("LOCALE_DEPS.json: %w", err)
	}
	var out []string
	for _, name := range t.Names() {
		if _, alias := d.Aliases[name]; excluded[name] || alias && !withAliases {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// v8Tag is how V8 writes an ICU locale name: its underscores made hyphens,
// and en_US_POSIX as its keyword.
func v8Tag(name string) string {
	tag := strings.ReplaceAll(name, "_", "-")
	if tag == "en-US-POSIX" {
		return "en-US-u-va-posix"
	}
	return tag
}

// validate is V8's ValidateResource: the locale's own bundle holds the key,
// or failing that the bundle of its language and script, or of its language.
// An alias bundle is read as the bundle it names. A bundle that does not
// exist, which ICU would open only by falling back, does not count.
func validate(t *icusrc.Locales, name, key string) bool {
	if t.Has(name) {
		n := resolveAlias(t, name)
		if n != nil && (key == "" || n.Get(key) != nil) {
			return true
		}
	}
	parts := strings.Split(name, "_")
	var script, region string
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4 && script == "" && region == "":
			script = p
		case len(p) == 2 || len(p) == 3 && p[0] >= '0' && p[0] <= '9':
			if region == "" {
				region = p
			}
		}
	}
	switch {
	case region != "" && script != "":
		return validate(t, parts[0]+"_"+script, key)
	case region != "" || script != "":
		return validate(t, parts[0], key)
	}
	return false
}

func resolveAlias(t *icusrc.Locales, name string) *icutxt.Node {
	for i := 0; i < 8; i++ {
		n, err := t.Get(name)
		if err != nil || n == nil {
			return nil
		}
		a := n.Get("%%ALIAS")
		if a == nil {
			return n
		}
		name = a.Value
	}
	return nil
}

// withoutScript is RemoveLocaleScriptTag: a tag with a script, written
// without it.
func withoutScript(name string) (string, bool) {
	parts := strings.Split(name, "_")
	if len(parts) < 2 || len(parts[1]) != 4 {
		return "", false
	}
	out := parts[0]
	if len(parts) > 2 && (len(parts[2]) == 2 || len(parts[2]) == 3) {
		out += "-" + parts[2]
	}
	return out, true
}

// buildSet is BuildLocaleSet: the locales that validate, and each one's form
// without its script.
func buildSet(t *icusrc.Locales, names []string, key string, check bool) map[string]bool {
	set := map[string]bool{}
	for _, name := range names {
		if check && !validate(t, name, key) {
			// V8 tries "no" for "nb".
			if name != "nb" || !validate(t, "no", key) {
				continue
			}
		}
		set[v8Tag(name)] = true
		if short, ok := withoutScript(name); ok {
			set[short] = true
		}
	}
	return set
}

func build(zip string) ([]byte, error) {
	locales, err := icusrc.OpenLocales(zip)
	if err != nil {
		return nil, err
	}
	defer locales.Close()
	coll, err := icusrc.OpenTree(zip, "coll")
	if err != nil {
		return nil, err
	}
	defer coll.Close()

	all, err := installed(locales, true)
	if err != nil {
		return nil, err
	}
	collNames, err := installed(coll, false)
	if err != nil {
		return nil, err
	}
	sets := map[string]map[string]bool{
		"all":      buildSet(locales, all, "", false),
		"number":   buildSet(locales, all, "NumberElements", true),
		"date":     buildSet(locales, all, "calendar", true),
		"list":     buildSet(locales, all, "listPattern", true),
		"collator": buildSet(coll, collNames, "", true),
	}

	b, err := locales.ReadMisc("plurals")
	if err != nil {
		return nil, err
	}
	plurals, err := icutxt.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("plurals.txt: %w", err)
	}
	table := plurals.Get("locales")
	if table == nil || !table.Table {
		return nil, fmt.Errorf("plurals.txt has no locales")
	}
	sets["plural"] = map[string]bool{}
	for _, e := range table.Children {
		tag := e.Key
		if len(tag) > 3 {
			tag = strings.ReplaceAll(tag, "_", "-")
		}
		sets["plural"][tag] = true
	}

	var lines []string
	for service, set := range sets {
		for tag := range set {
			lines = append(lines, service+" "+tag)
		}
	}
	sort.Strings(lines)

	redirects, err := redirectLines(zip, sets["all"])
	if err != nil {
		return nil, err
	}
	lines = append(lines, redirects...)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// redirectTrees are the trees whose bundles a service's data comes from.
var redirectTrees = []string{"locales", "unit", "curr", "lang", "region", "zone"}

// redirectLines are the locales ICU reads another locale's bundle for, in
// each tree, as "redirect <tree> <tag> <bundle>": "redirect locales zh-TW
// zh-Hant-TW", "redirect unit sr-Cyrl-ME sr-Latn-ME". ICU opens a locale's
// own bundle if the tree has it, an alias bundle as the bundle it names, and
// otherwise the one its fallback finds, which drops a default script; CLDR's
// chain goes elsewhere for these. A bundle that is only a shorter form of
// the tag, which CLDR's chain reaches too, is not written, nor a tag with a
// variant, which a data locale cannot hold.
func redirectLines(zip string, available map[string]bool) ([]string, error) {
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return nil, err
	}
	var tags []string
	for tag := range available {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	var out []string
	for _, tree := range redirectTrees {
		t, err := icusrc.OpenTree(zip, tree)
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			name := strings.ReplaceAll(tag, "-", "_")
			if strings.Contains(tag, "_u_") || strings.Contains(name, "_u_") || hasVariant(name) {
				continue
			}
			bundle := t.Bundle(name, fb)
			if bundle == name || bundle == "root" || strings.HasPrefix(name+"_", bundle+"_") || hasVariant(bundle) {
				continue
			}
			out = append(out, "redirect "+tree+" "+tag+" "+strings.ReplaceAll(bundle, "_", "-"))
		}
		t.Close()
	}
	return out, nil
}

// hasVariant reports whether an ICU locale name is more than a language,
// script and region: "no_NO_NY", whose "NY" is ICU's legacy variant.
func hasVariant(name string) bool {
	parts := strings.Split(name, "_")[1:]
	if len(parts) > 0 && len(parts[0]) == 4 {
		parts = parts[1:]
	}
	if len(parts) > 0 && (len(parts[0]) == 2 || len(parts[0]) == 3 && parts[0][0] >= '0' && parts[0][0] <= '9') {
		parts = parts[1:]
	}
	return len(parts) > 0
}
