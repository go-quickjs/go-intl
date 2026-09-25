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
// The file is lines of a service and a tag: "number zh-Hant-HK". The index
// of ICU's trees its resource fallback reads is written beside it (see
// writeIndex).
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
	if err := writeValues(os.Args[1]); err != nil {
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

	if err := writeIndex(zip); err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// indexTrees are the trees of ICU's data go-intl's data mirrors, whose index
// the runtime resolves a locale in as ICU does.
var indexTrees = []string{"locales", "unit", "curr", "lang", "region", "zone", "coll"}

// writeIndex writes what ICU's resource fallback reads, so that go-intl can
// open, for any locale, the bundle ICU opens. Each tree has a file,
// data/icutree-<tree>.bin: its bundles ("bundles <name>..."), those that are
// aliases ("alias <name> <target>") and those that name their parent
// ("parent <name> <parent>"). data/icufallback.bin holds the tables ICU
// consults when a bundle does not exist: the default scripts
// ("defaultscript sr_ME Latn") and parent locales ("icuparent en_150
// en_001"). ICU opens an alias bundle as the one it names and, for one that
// does not exist, drops a default script, so "sr-ME" is Serbian in Latin,
// "zh-TW" traditional Chinese and "az-Arab", which ICU has no data for, the
// root.
func writeIndex(zip string) error {
	files := map[string][]string{}
	for _, tree := range indexTrees {
		t, err := icusrc.OpenTree(zip, tree)
		if err != nil {
			return err
		}
		names := t.Names()
		out := []string{"bundles " + strings.Join(names, " ")}
		for _, name := range names {
			n, err := t.Get(name)
			if err != nil {
				t.Close()
				return err
			}
			if n == nil {
				continue
			}
			if a := n.Get("%%ALIAS"); a != nil && a.Value != "" {
				out = append(out, "alias "+name+" "+a.Value)
			}
			if p := n.Get("%%Parent"); p != nil && p.Value != "" {
				out = append(out, "parent "+name+" "+p.Value)
			}
		}
		t.Close()
		files["icutree-"+tree+".bin"] = out
	}
	defaults, parents, err := icusrc.FallbackTables()
	if err != nil {
		return err
	}
	var tables []string
	for k, v := range defaults {
		tables = append(tables, "defaultscript "+k+" "+v)
	}
	for k, v := range parents {
		tables = append(tables, "icuparent "+k+" "+v)
	}
	sort.Strings(tables)
	files["icufallback.bin"] = tables
	// Every file is built before any is written.
	for name, lines := range files {
		target := filepath.Join("data", name)
		if err := os.WriteFile(target+".tmp", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return err
		}
	}
	for name := range files {
		target := filepath.Join("data", name)
		if err := os.Rename(target+".tmp", target); err != nil {
			return err
		}
	}
	return nil
}
