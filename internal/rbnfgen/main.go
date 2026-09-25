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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/rbnfdata"
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
	z, err := icusrc.Open(zipPath, icusrc.DataSHA256)
	if err != nil {
		return err
	}
	defer z.Close()
	read := func(name string) (*icutxt.Node, error) {
		b, err := icusrc.ReadFile(z, "data/"+name+".txt")
		if err != nil {
			return nil, err
		}
		return icutxt.Parse(string(b))
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
	for _, name := range names {
		locale, kind, _ := strings.Cut(name, "/")
		bundle, err := read("rbnf/" + locale)
		if err != nil {
			return err
		}
		rules := bundle.Get("RBNFRules", kind)
		if rules == nil || len(rules.Values) == 0 {
			return fmt.Errorf("rbnf/%s.txt has no %s", locale, kind)
		}
		d.Groups = append(d.Groups, rbnfdata.Group{Name: name, Rules: rules.Values})
	}

	out := filepath.Join("data", "rbnf.bin")
	if err := os.WriteFile(out, rbnfdata.Encode(&d), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rbnfgen: %d systems in %d rule groups\n", len(d.Systems), len(d.Groups))
	return nil
}
