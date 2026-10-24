package icusrc

import (
	"fmt"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// A View is one locale's resources as ICU resolves them: each resource
// looked for in the locale's bundle and then in each one its fallback
// reaches, the root last, and an alias followed where one is met.
//
// ICU's bundles hold only what differs from the bundle they fall back to,
// and a table is resolved key by key, as ures_getByKeyWithFallback reads
// it: a table the locale has some keys of has the rest from its parents.
// The aliases in the locale trees are all "/LOCALE/...", which name a
// resource of the locale asking, not of the bundle that holds the alias:
// the root's "listPattern/or-short/2" is an alias of "/LOCALE/listPattern/
// or/2", which is French in French.
type View struct {
	name  string
	chain []*icutxt.Node
	// views of the locale's aliases are read again for each, as ICU
	// does; depth bounds an alias that leads back to itself.
	c  *Locales
	fb Fallback
}

// maxAliasDepth bounds how many aliases one lookup follows.
const maxAliasDepth = 16

// View is the resolved view of a locale's resources.
func (c *Locales) View(name string, fb Fallback) (*View, error) {
	chain, err := c.Resolve(name, fb)
	if err != nil {
		return nil, err
	}
	return &View{name: name, chain: chain, c: c, fb: fb}, nil
}

// Name is the locale's ICU name, "zh_Hant_HK".
func (v *View) Name() string { return v.name }

// Get returns the resource at path, or nil where neither the locale nor its
// fallback has one. A table comes back merged: each of its keys that any
// bundle of the chain has, in the order the most specific bundle that has
// it writes it, each resolved in turn.
func (v *View) Get(path ...string) (*icutxt.Node, error) {
	return v.get(path, 0)
}

func (v *View) get(path []string, depth int) (*icutxt.Node, error) {
	if depth > maxAliasDepth {
		return nil, fmt.Errorf("icusrc: %s: aliases past %d deep at /%s", v.name, maxAliasDepth, strings.Join(path, "/"))
	}
	var tables []*icutxt.Node
	for _, b := range v.chain {
		n, alias, rest := walk(b, path)
		if alias != "" {
			// The alias stands in for the resource and everything below
			// it, in this bundle; a more specific bundle already had its
			// say, so what the alias resolves to is the answer, merged
			// under any keys the more specific bundles' tables had.
			target, err := aliasPath(alias)
			if err != nil {
				return nil, fmt.Errorf("icusrc: %s: %w", v.name, err)
			}
			resolved, err := v.get(append(target, rest...), depth+1)
			if err != nil {
				return nil, err
			}
			return v.merge(path, tables, resolved, depth)
		}
		if n == nil {
			continue
		}
		if !n.Table {
			if len(tables) > 0 {
				// A table in a more specific bundle over a leaf in a less
				// specific one: the table is what ICU finds.
				break
			}
			return n, nil
		}
		tables = append(tables, n)
	}
	if len(tables) == 0 {
		return nil, nil
	}
	return v.merge(path, tables, nil, depth)
}

// merge is the table at path whose keys are those of tables, most specific
// first, and of under, an alias's resolution, each resolved through the
// view.
func (v *View) merge(path []string, tables []*icutxt.Node, under *icutxt.Node, depth int) (*icutxt.Node, error) {
	if len(tables) == 0 {
		return under, nil
	}
	var keys []string
	seen := map[string]bool{}
	add := func(t *icutxt.Node) {
		for _, c := range t.Children {
			if !seen[c.Key] {
				seen[c.Key] = true
				keys = append(keys, c.Key)
			}
		}
	}
	for _, t := range tables {
		add(t)
	}
	if under != nil && under.Table {
		add(under)
	}
	out := &icutxt.Node{Key: tables[0].Key, Table: true}
	for _, k := range keys {
		child, err := v.get(append(append([]string(nil), path...), k), depth)
		if err != nil {
			return nil, err
		}
		if child == nil && under != nil {
			child = under.Get(k)
		}
		if child != nil {
			copied := *child
			copied.Key = k
			out.Children = append(out.Children, &copied)
		}
	}
	return out, nil
}

// walk follows path down a bundle. It returns the node at the end, or, where
// an alias stands on the way, the alias and the path left below it.
func walk(b *icutxt.Node, path []string) (n *icutxt.Node, alias string, rest []string) {
	n = b
	for i, key := range path {
		if n.Alias {
			return nil, n.Value, path[i:]
		}
		if !n.Table {
			return nil, "", nil
		}
		var next *icutxt.Node
		for _, c := range n.Children {
			if c.Key == key {
				next = c
				break
			}
		}
		if next == nil {
			return nil, "", nil
		}
		n = next
	}
	if n.Alias {
		return nil, n.Value, nil
	}
	return n, "", nil
}

// aliasPath is the path an alias of the locale trees names.
func aliasPath(alias string) ([]string, error) {
	p, ok := strings.CutPrefix(alias, "/LOCALE/")
	if !ok {
		return nil, fmt.Errorf("an alias %q, which is not /LOCALE/", alias)
	}
	return strings.Split(p, "/"), nil
}
