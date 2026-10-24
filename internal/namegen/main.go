// Command namegen writes the display-name tables the intl package carries.
//
// It reads ICU's display names, which ICU keeps in three trees of the data
// archive SOURCES.md pins: the languages, scripts, variants, calendars and
// the locale pattern in data/lang, the regions in data/region, and the
// date fields in data/locales, each as ICU resolves it in its tree.
//
//	go run ./internal/namegen [-seed pool.bin] icu4c-78.3-data.zip
//
// -seed numbers the pool's parts after an earlier pool's (datawrite.NewPool).
//
// ICU writes the shorter forms in tables of their own beside the long one --
// "Countries" has GB as "United Kingdom" and "Countries%short" as "UK" -- so
// the table is what sorts a name into its width. A width holds only what
// differs from the one above it, which is why a lookup walks back rather
// than each width being complete.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/namedata"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

func main() {
	seed := flag.String("seed", "", "a pool to number the parts after")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/namegen [-seed pool.bin] <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *seed); err != nil {
		fmt.Fprintln(os.Stderr, "namegen:", err)
		os.Exit(1)
	}
}

// The trees the names are in, in the order EachLocaleIn is given them.
const (
	langTree = iota
	regionTree
	localesTree
)

func run(zip, seed string) error {
	var trees []*icusrc.Locales
	for _, name := range []string{"lang", "region", "locales"} {
		t, err := icusrc.OpenTree(zip, name)
		if err != nil {
			return err
		}
		defer t.Close()
		trees = append(trees, t)
	}
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}
	// The locales share one pool, written beside them. EachLocaleIn visits
	// the bundles in order, so the pool is the same every run.
	pool, err := datawrite.NewPool(namedata.Version, seed)
	if err != nil {
		return err
	}
	built, err := icusrc.EachLocaleIn(trees, fb, func(v []*icusrc.View) ([]byte, error) {
		l, err := read(v)
		if err != nil || l == nil {
			return nil, err
		}
		return namedata.Encode(l, pool), nil
	})
	if err != nil {
		return err
	}

	set := writeset.New()
	if err := datawrite.AddLocales(set, filepath.Join("data", "names"), built); err != nil {
		return err
	}
	set.File(filepath.Join("data", "namesshared.bin"), pool.Bytes())
	if err := set.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "namegen: %d locales\n", len(built))
	return nil
}

// An alternate is one of ICU's tables of a shorter form, by its suffix,
// and the width it fills.
type alternate struct {
	suffix string
	width  int
}

// alternates are the suffixes whose names fill a width, in the order they
// are taken where two fill one. The others, "variant" and "menu" among
// them, ICU keeps in tables its display names never read: Japanese has
// "hi-Latn" only as a variant, and names it as Hindi in Latin.
var alternates = []alternate{
	{"short", namedata.Short},
	{"narrow", namedata.Narrow},
}

// names gathers a kind's names from ICU's table of it and the tables of its
// shorter forms, keyed as fill takes them: "GB" for the long name and
// "GB-alt-short" for the short one. A language ICU names a locale of is
// keyed as the locale is in ICU, "de_AT", and here as BCP 47 writes it.
func names(v *icusrc.View, path ...string) (map[string]string, bool, error) {
	out := map[string]string{}
	found := false
	for _, suffix := range append([]string{""}, "short", "narrow") {
		p := append([]string(nil), path...)
		key := ""
		if suffix != "" {
			p[0] += "%" + suffix
			key = "-alt-" + suffix
		}
		t, err := v.Get(p...)
		if err != nil {
			return nil, false, err
		}
		if t == nil {
			continue
		}
		found = true
		for _, n := range t.Children {
			// ICU's name of the root, "root", is no code a name is asked for.
			if !n.Table && n.Value != "" && n.Key != "root" {
				out[strings.ReplaceAll(n.Key, "_", "-")+key] = n.Value
			}
		}
	}
	return out, found, nil
}

func read(v []*icusrc.View) (*namedata.Built, error) {
	var out namedata.Built
	any := false
	for _, k := range []struct {
		view int
		kind int
		path []string
		bcp  bool
	}{
		{langTree, namedata.Language, []string{"Languages"}, false},
		{regionTree, namedata.Region, []string{"Countries"}, false},
		{langTree, namedata.Script, []string{"Scripts"}, false},
		{langTree, namedata.Variant, []string{"Variants"}, false},
		{langTree, namedata.Calendar, []string{"Types", "calendar"}, true},
	} {
		source, found, err := names(v[k.view], k.path...)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if k.bcp {
			source = withBCP47(source)
		}
		fill(&out, k.kind, source, alternates)
		any = true
	}

	pattern, err := v[langTree].Get("localeDisplayPattern")
	if err != nil {
		return nil, err
	}
	if pattern != nil {
		if p := pattern.Get("pattern"); p != nil {
			out.Pattern = p.Value
		}
		if p := pattern.Get("separator"); p != nil {
			out.Separator = p.Value
		}
		any = true
	}

	fields, err := v[localesTree].Get("fields")
	if err != nil {
		return nil, err
	}
	if fields != nil {
		byKey := map[string]string{}
		for _, field := range fields.Children {
			dn := field.Get("dn")
			if dn == nil || dn.Value == "" {
				continue
			}
			// The widths are their own keys here rather than tables.
			base, suffix := field.Key, ""
			if at := strings.LastIndexByte(field.Key, '-'); at >= 0 {
				if s := field.Key[at+1:]; s == "short" || s == "narrow" {
					base, suffix = field.Key[:at], s
				}
			}
			if suffix == "" {
				byKey[base] = dn.Value
			} else {
				byKey[base+"-alt-"+suffix] = dn.Value
			}
		}
		fill(&out, namedata.DateTimeField, byKey, alternates)
		any = true
	}

	if !any {
		return nil, nil
	}
	return &out, nil
}

// fill sorts the names into the widths, by the alternate suffix each key
// carries: a plain name is the long one, and an alternate fills its width
// where nothing has, the first of alts to that wins. Nothing depends on the
// order source's keys come in.
func fill(out *namedata.Built, kind int, source map[string]string, alts []alternate) {
	byWidth := make([]map[string]string, namedata.Widths)
	for i := range byWidth {
		byWidth[i] = map[string]string{}
	}
	for key, name := range source {
		if name != "" && !strings.Contains(key, "-alt-") {
			byWidth[namedata.Long][key] = name
		}
	}
	for _, a := range alts {
		for key, name := range source {
			code, suffix, found := strings.Cut(key, "-alt-")
			if !found || suffix != a.suffix || name == "" {
				continue
			}
			if _, taken := byWidth[a.width][code]; !taken {
				byWidth[a.width][code] = name
			}
		}
	}
	for w := range byWidth {
		entries := make([]namedata.Entry, 0, len(byWidth[w]))
		for code, name := range byWidth[w] {
			// A width holds only what differs from the one above it.
			if w != namedata.Long && byWidth[namedata.Long][code] == name {
				continue
			}
			entries = append(entries, namedata.Entry{Code: code, Name: name})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Code < entries[j].Code })
		out.Sets[kind*namedata.Widths+w].Entries = entries
	}
}

// calendarAliases are the calendars BCP-47 spells differently from CLDR,
// as V8 maps them before it asks ICU for a name: ECMA-402 asks for "gregory"
// and CLDR files it under "gregorian". "islamicc", a deprecated alias of
// "islamic-civil", is not among them, and has no name.
var calendarAliases = map[string]string{
	"gregorian":           "gregory",
	"ethiopic-amete-alem": "ethioaa",
}

// withBCP47 adds the spelling ECMA-402 uses beside CLDR's own, so that a
// lookup finds a calendar under either name.
func withBCP47(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+len(calendarAliases))
	for key, name := range in {
		out[key] = name
		base, alt, found := strings.Cut(key, "-alt-")
		alias, ok := calendarAliases[base]
		if !ok {
			continue
		}
		if found {
			out[alias+"-alt-"+alt] = name
		} else {
			out[alias] = name
		}
	}
	return out
}
