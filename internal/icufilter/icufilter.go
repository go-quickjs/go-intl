// Package icufilter applies an ICU data filter -- the filters file of ICU's
// data build tool, which a browser builds its ICU data with -- to ICU's
// resource bundle sources, as the build tool and genrb do: whole files are
// left out by category, and within a file each resource is kept or dropped
// by rules on its path.
//
// The semantics are ICU 78's, ported: the file filters from
// python/icutools/databuilder/filtration.py, and the resource rules from
// tools/genrb/filterrb.cpp, which genrb applies to a bundle's tables
// (TableResource::applyFilter in reslist.cpp).
//
// A category is the build tool's name for a set of files: "locales_tree"
// for data/locales/*.txt, "curr_tree" for data/curr, "misc" for
// data/misc, and so on. A file is named by its stem, "en_GB".
package icufilter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// A Config is one filters file.
type Config struct {
	additive  bool
	locale    *fileFilter            // localeFilter, for the *_tree categories
	features  map[string]*fileFilter // featureFilters, by category
	resources []resourceFilter
}

// A resourceFilter is one entry of resourceFilters: rules for the files of
// some categories that a file filter keeps.
type resourceFilter struct {
	categories []string
	files      *fileFilter // nil: every file
	rules      []string
}

// Parse reads a filters file. A line that begins with "//" is a comment, as
// the build tool's CommentStripper has it; the rest is JSON.
func Parse(b []byte) (*Config, error) {
	var lines []string
	for _, line := range strings.SplitAfter(string(b), "\n") {
		if !strings.HasPrefix(line, "//") {
			lines = append(lines, line)
		} else if strings.HasSuffix(line, "\n") {
			lines = append(lines, "\n")
		}
	}
	var raw struct {
		Strategy        string                     `json:"strategy"`
		LocaleFilter    json.RawMessage            `json:"localeFilter"`
		FeatureFilters  map[string]json.RawMessage `json:"featureFilters"`
		ResourceFilters []struct {
			Categories []string        `json:"categories"`
			Files      json.RawMessage `json:"files"`
			Rules      []string        `json:"rules"`
		} `json:"resourceFilters"`
	}
	if err := json.Unmarshal([]byte(strings.Join(lines, "")), &raw); err != nil {
		return nil, fmt.Errorf("icufilter: %w", err)
	}
	c := &Config{features: map[string]*fileFilter{}}
	switch raw.Strategy {
	case "", "subtractive":
	case "additive":
		c.additive = true
	default:
		return nil, fmt.Errorf("icufilter: the strategy %q", raw.Strategy)
	}
	var err error
	if len(raw.LocaleFilter) > 0 {
		if c.locale, err = parseFilter(raw.LocaleFilter); err != nil {
			return nil, fmt.Errorf("icufilter: localeFilter: %w", err)
		}
	}
	for category, f := range raw.FeatureFilters {
		if c.features[category], err = parseFeature(f); err != nil {
			return nil, fmt.Errorf("icufilter: featureFilters %s: %w", category, err)
		}
	}
	for i, r := range raw.ResourceFilters {
		rf := resourceFilter{categories: r.Categories, rules: r.Rules}
		if len(r.Files) > 0 {
			if rf.files, err = parseFilter(r.Files); err != nil {
				return nil, fmt.Errorf("icufilter: resourceFilters[%d]: %w", i, err)
			}
		}
		for _, rule := range r.Rules {
			if _, _, err := parseRule(rule); err != nil {
				return nil, fmt.Errorf("icufilter: resourceFilters[%d]: %w", i, err)
			}
		}
		c.resources = append(c.resources, rf)
	}
	return c, nil
}

// parseFeature reads a featureFilters entry, which may also be the word
// "include" or "exclude".
func parseFeature(raw json.RawMessage) (*fileFilter, error) {
	var word string
	if json.Unmarshal(raw, &word) == nil {
		switch word {
		case "include":
			return &fileFilter{kind: "include"}, nil
		case "exclude":
			return &fileFilter{kind: "exclude"}, nil
		}
		return nil, fmt.Errorf("the filter %q", word)
	}
	return parseFilter(raw)
}

// KeepFile reports whether a category's file is built at all.
func (c *Config) KeepFile(category, stem string) bool {
	f := c.fileFilter(category)
	return f == nil || f.match(stem)
}

// fileFilter is the filter a category's files are kept by, as
// _preprocess_file_filters chooses it, or nil for all of them.
func (c *Config) fileFilter(category string) *fileFilter {
	var f *fileFilter
	if c.additive {
		f = &fileFilter{kind: "exclude"}
	}
	// The LSTM and AdaBoost break models are left out unless asked for.
	if category == "brkitr_lstm" || category == "brkitr_adaboost" {
		f = &fileFilter{kind: "exclude"}
	}
	if ff, ok := c.features[category]; ok {
		f = ff
	}
	if (f == nil || f.kind == "include") && c.locale != nil && strings.HasSuffix(category, "_tree") {
		f = c.locale
	}
	if f != nil && f.kind == "include" {
		return nil
	}
	return f
}

// Rules is the resource filter of a category's file: the build tool's
// starting rule, "+/" (or, additive, "-/" keeping the bundle's alias and
// parent), then the rules of every resourceFilters entry that names the
// category and whose file filter keeps the file, in order. It is nil where
// no entry names the category, and genrb filters nothing.
func (c *Config) Rules(category, stem string) *PathFilter {
	var rules []string
	named := false
	for _, r := range c.resources {
		has := false
		for _, cat := range r.categories {
			has = has || cat == category
		}
		if !has {
			continue
		}
		named = true
		if r.files == nil || r.files.match(stem) {
			rules = append(rules, r.rules...)
		}
	}
	if !named {
		return nil
	}
	start := []string{"+/"}
	if c.additive {
		start = []string{"-/", "+/%%ALIAS", "+/%%Parent"}
	}
	f := &PathFilter{}
	for _, rule := range append(start, rules...) {
		path, include, _ := parseRule(rule)
		f.root.apply(path, include)
	}
	return f
}

// A fileFilter is one of the build tool's file filters, which match files
// by their stems.
type fileFilter struct {
	kind     string // file-stem, language, regex, include, exclude, union, intersection, complement
	include  bool   // an includelist rather than an excludelist
	list     map[string]bool
	patterns []*regexp.Regexp
	subs     []*fileFilter
}

func parseFilter(raw json.RawMessage) (*fileFilter, error) {
	var j struct {
		FilterType     string            `json:"filterType"`
		Includelist    []string          `json:"includelist"`
		Whitelist      []string          `json:"whitelist"`
		Excludelist    []string          `json:"excludelist"`
		Blacklist      []string          `json:"blacklist"`
		UnionOf        []json.RawMessage `json:"unionOf"`
		IntersectionOf []json.RawMessage `json:"intersectionOf"`
		ComplementOf   json.RawMessage   `json:"complementOf"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	f := &fileFilter{kind: j.FilterType}
	if f.kind == "" {
		f.kind = "file-stem"
	}
	subs := func(list []json.RawMessage) error {
		for _, s := range list {
			sub, err := parseFilter(s)
			if err != nil {
				return err
			}
			f.subs = append(f.subs, sub)
		}
		return nil
	}
	switch f.kind {
	case "file-stem", "language", "regex":
		var list []string
		switch {
		case j.Whitelist != nil:
			f.include, list = true, j.Whitelist
		case j.Includelist != nil:
			f.include, list = true, j.Includelist
		case j.Blacklist != nil:
			list = j.Blacklist
		case j.Excludelist != nil:
			list = j.Excludelist
		default:
			return nil, fmt.Errorf("a %s filter without an includelist or excludelist", f.kind)
		}
		f.list = map[string]bool{}
		for _, s := range list {
			f.list[s] = true
			if f.kind == "regex" {
				// Python's re.match anchors at the start only.
				re, err := regexp.Compile("^(?:" + s + ")")
				if err != nil {
					return nil, err
				}
				f.patterns = append(f.patterns, re)
			}
		}
	case "exclude":
	case "union":
		if err := subs(j.UnionOf); err != nil {
			return nil, err
		}
	case "intersection":
		if err := subs(j.IntersectionOf); err != nil {
			return nil, err
		}
	case "complement":
		if err := subs([]json.RawMessage{j.ComplementOf}); err != nil {
			return nil, err
		}
	default:
		// "locale" reads each tree's LOCALE_DEPS.json, which nothing
		// filtered here has needed yet.
		return nil, fmt.Errorf("the filter type %q", f.kind)
	}
	return f, nil
}

// match reports whether the filter keeps a file.
func (f *fileFilter) match(stem string) bool {
	switch f.kind {
	case "include":
		return true
	case "exclude":
		return false
	case "file-stem":
		return f.list[stem] == f.include
	case "language":
		language, _, _ := strings.Cut(stem, "_")
		// The root is always kept.
		if language == "root" {
			return true
		}
		return f.list[language] == f.include
	case "regex":
		for _, re := range f.patterns {
			if re.MatchString(stem) {
				return f.include
			}
		}
		return !f.include
	case "union":
		for _, s := range f.subs {
			if s.match(stem) {
				return true
			}
		}
		return false
	case "intersection":
		for _, s := range f.subs {
			if !s.match(stem) {
				return false
			}
		}
		return true
	case "complement":
		return !f.subs[0].match(stem)
	}
	return false
}

// A PathFilter is genrb's SimpleRuleBasedPathFilter: a tree of the rules'
// paths, each node included, excluded or partly either.
type PathFilter struct {
	root ruleNode
}

type inclusion int

const (
	partial inclusion = iota
	include
	exclude
)

type ruleNode struct {
	inclusion inclusion
	children  map[string]*ruleNode
	wildcard  *ruleNode
}

// parseRule reads a rule: "+" or "-", then a path from "/", its keys
// separated by "/", "*" matching any one key.
func parseRule(rule string) ([]string, bool, error) {
	if rule == "" || rule[0] != '+' && rule[0] != '-' {
		return nil, false, fmt.Errorf("the rule %q starts with neither + nor -", rule)
	}
	path := rule[1:]
	if !strings.HasPrefix(path, "/") {
		return nil, false, fmt.Errorf("the rule %q: a path must start with /", rule)
	}
	if path == "/" {
		return nil, rule[0] == '+', nil
	}
	keys := strings.Split(path[1:], "/")
	for _, k := range keys {
		if k == "" {
			return nil, false, fmt.Errorf("the rule %q: empty keys and trailing slashes are not allowed", rule)
		}
	}
	return keys, rule[0] == '+', nil
}

func (n *ruleNode) isLeaf() bool { return len(n.children) == 0 && n.wildcard == nil }

func (n *ruleNode) clone() *ruleNode {
	c := &ruleNode{inclusion: n.inclusion}
	if n.children != nil {
		c.children = map[string]*ruleNode{}
		for k, v := range n.children {
			c.children[k] = v.clone()
		}
	}
	if n.wildcard != nil {
		c.wildcard = n.wildcard.clone()
	}
	return c
}

// apply adds a rule below the node: Tree::applyRule.
func (n *ruleNode) apply(path []string, inc bool) {
	if len(path) == 0 {
		// The rule overrides everything said below it before.
		n.inclusion = exclude
		if inc {
			n.inclusion = include
		}
		n.children, n.wildcard = nil, nil
		return
	}
	key := path[0]
	if key == "*" {
		// The wildcard's subtree, and every key already named.
		if n.wildcard == nil {
			n.wildcard = &ruleNode{}
		}
		n.wildcard.apply(path[1:], inc)
		for _, c := range n.children {
			c.apply(path[1:], inc)
		}
		return
	}
	child, ok := n.children[key]
	if !ok {
		// A key named after a wildcard starts as a copy of its subtree.
		if n.wildcard != nil {
			child = n.wildcard.clone()
		} else {
			child = &ruleNode{}
		}
		if n.children == nil {
			n.children = map[string]*ruleNode{}
		}
		n.children[key] = child
	}
	child.apply(path[1:], inc)
}

// match is what the filter says of a resource's path: kept whole, dropped
// whole, or partly, to be decided key by key below it.
func (f *PathFilter) match(path []string) inclusion {
	node := &f.root
	// The nearest definite rule above bubbles down.
	result := include
	if node.inclusion != partial {
		result = node.inclusion
	}
	leaf := false
	for _, key := range path {
		child, ok := node.children[key]
		if !ok {
			if node.wildcard == nil {
				leaf = true
				break
			}
			child = node.wildcard
		}
		node = child
		if node.inclusion != partial {
			result = node.inclusion
		}
	}
	if node.isLeaf() {
		leaf = true
	}
	if !leaf {
		return partial
	}
	if node.inclusion == partial {
		return result
	}
	return node.inclusion
}

// Apply returns a copy of a bundle without what the filter drops. Only a
// table's entries are filtered: an array is kept or dropped whole.
func (f *PathFilter) Apply(bundle *icutxt.Node) *icutxt.Node {
	if f == nil {
		return bundle
	}
	return f.applyTable(bundle, nil)
}

func (f *PathFilter) applyTable(n *icutxt.Node, path []string) *icutxt.Node {
	out := *n
	out.Children = nil
	for _, c := range n.Children {
		p := append(append([]string(nil), path...), c.Key)
		switch f.match(p) {
		case include:
			out.Children = append(out.Children, c)
		case exclude:
		default:
			if c.Table {
				out.Children = append(out.Children, f.applyTable(c, p))
			} else {
				out.Children = append(out.Children, c)
			}
		}
	}
	return &out
}
