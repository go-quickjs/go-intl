// Command rbnfgen writes the rules of ICU's algorithmic numbering systems:
// Roman, Hebrew, Armenian and the rest, and the Chinese and Japanese ones
// written by spell-out rules. It reads the pinned ICU data sources,
// misc/numberingSystems.txt for which rule set each system names and
// rbnf/*.txt for the rules.
//
//	go run ./internal/rbnfgen <icu4c-78.3-data.zip>
//
// It writes data/rbnf.bin.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/rbnfdata"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/rbnfgen <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "rbnfgen:", err)
		os.Exit(1)
	}
}

func run(zipPath string) error {
	archive, err := icusrc.OpenArchive(zipPath)
	if err != nil {
		return err
	}
	defer archive.Close()
	read := func(name string) (*icutxt.Node, error) {
		return archive.Bundle("data/" + name + ".txt")
	}
	systems, err := read("misc/numberingSystems")
	if err != nil {
		return err
	}
	table := systems.Get("numberingSystems")
	if table == nil {
		return fmt.Errorf("numberingSystems.txt has no numberingSystems table")
	}

	var d rbnfdata.Data
	groups := map[string]bool{}
	for _, s := range table.Children {
		if a := s.Get("algorithmic"); a == nil || a.Value != "1" {
			continue
		}
		desc := s.Get("desc")
		if desc == nil {
			return fmt.Errorf("%s has no rules", s.Key)
		}
		// "%hebrew" is the root's numbering-system rules; "ja/SpelloutRules/
		// %spellout-numbering-year-latn" names a locale and a group.
		group, set := "root/NumberingSystemRules", desc.Value
		if cut := strings.LastIndexByte(desc.Value, '/'); cut >= 0 {
			group, set = desc.Value[:cut], desc.Value[cut+1:]
		}
		d.Systems = append(d.Systems, rbnfdata.System{Name: s.Key, Group: group, Set: set})
		groups[group] = true
	}
	sort.Slice(d.Systems, func(i, j int) bool { return d.Systems[i].Name < d.Systems[j].Name })

	names := make([]string, 0, len(groups))
	for g := range groups {
		names = append(names, g)
	}
	sort.Strings(names)
	missing := map[string]bool{}
	for _, name := range names {
		locale, kind, _ := strings.Cut(name, "/")
		bundle, err := read("rbnf/" + locale)
		if errors.Is(err, fs.ErrNotExist) && archive.Filtered() {
			// A browser's data filter can leave a rules bundle out, root's
			// among them: the systems whose rules it holds have none.
			missing[name] = true
			continue
		}
		if err != nil {
			return err
		}
		rules := bundle.Get("RBNFRules", kind)
		if rules == nil || len(rules.Values) == 0 {
			return fmt.Errorf("rbnf/%s.txt has no %s", locale, kind)
		}
		d.Groups = append(d.Groups, rbnfdata.Group{Name: name, Rules: rules.Values})
	}

	kept := d.Systems[:0]
	for _, s := range d.Systems {
		if !missing[s.Group] {
			kept = append(kept, s)
		}
	}
	d.Systems = kept

	if err := writeset.WriteFile(filepath.Join("data", "rbnf.bin"), rbnfdata.Encode(&d)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rbnfgen: %d systems in %d rule groups\n", len(d.Systems), len(d.Groups))
	return nil
}
