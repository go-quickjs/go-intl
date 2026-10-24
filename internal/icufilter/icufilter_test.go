package icufilter

import (
	"strings"
	"testing"

	"github.com/go-quickjs/go-intl/internal/icutxt"
)

func keys(n *icutxt.Node) string {
	var out []string
	var walk func(prefix string, n *icutxt.Node)
	walk = func(prefix string, n *icutxt.Node) {
		for _, c := range n.Children {
			p := prefix + "/" + c.Key
			if c.Table && len(c.Children) > 0 {
				walk(p, c)
			} else {
				out = append(out, p)
			}
		}
	}
	walk("", n)
	return strings.Join(out, " ")
}

func filterOf(t *testing.T, rules ...string) *PathFilter {
	t.Helper()
	f := &PathFilter{}
	for _, r := range rules {
		path, inc, err := parseRule(r)
		if err != nil {
			t.Fatal(err)
		}
		f.root.apply(path, inc)
	}
	return f
}

const bundle = `en{
	%%Parent{"root"}
	Version{"48"}
	calendar{ gregorian{ dayNames{ x{"a"} } eras{ y{"b"} } } }
	fields{
		mon{ dn{"Monday"} }
		mon-short{ dn{"Mon"} }
		tue{ dn{"Tuesday"} }
		year{ dn{"year"} relative{ "0"{"this year"} } }
	}
	NumberElements{ latn{ symbols{ decimal{"."} } } minimalPairs{ one{"1"} } }
	units{ length{ meter{ one{"m"} } foot{ one{"ft"} } } }
}`

// genrb keeps or drops each table entry by the rules on its path: the last
// rule that covers it wins, a wildcard matches any one key, and a key a rule
// names after a wildcard starts with the wildcard's rules.
func TestPathFilter(t *testing.T) {
	n, err := icutxt.Parse(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		rules []string
		want  string
	}{
		{[]string{"+/"}, keys(n)},
		{[]string{"+/", "-/fields"}, "/%%Parent /Version /calendar/gregorian/dayNames/x /calendar/gregorian/eras/y /NumberElements/latn/symbols/decimal /NumberElements/minimalPairs/one /units/length/meter/one /units/length/foot/one"},
		{[]string{"+/", "-/fields", "+/fields/year"}, "/%%Parent /Version /calendar/gregorian/dayNames/x /calendar/gregorian/eras/y /fields/year/dn /fields/year/relative/0 /NumberElements/latn/symbols/decimal /NumberElements/minimalPairs/one /units/length/meter/one /units/length/foot/one"},
		// Chromium's: no weekday fields, no minimal pairs.
		{[]string{"+/", "-/*/mon", "-/*/mon-short", "-/*/tue", "-/NumberElements/minimalPairs"},
			"/%%Parent /Version /calendar/gregorian/dayNames/x /calendar/gregorian/eras/y /fields/year/dn /fields/year/relative/0 /NumberElements/latn/symbols/decimal /units/length/meter/one /units/length/foot/one"},
		// Its locales of the least data: nothing but the bundle's parent and
		// version.
		{[]string{"+/", "-/*", "+/%%ALIAS", "+/%%Parent", "+/LocaleScript", "+/layout", "+/Version"}, "/%%Parent /Version"},
		// Its units: a few, without their case.
		{[]string{"+/", "-/units", "+/*/*/meter"}, "/%%Parent /Version /calendar/gregorian/dayNames/x /calendar/gregorian/eras/y /fields/mon/dn /fields/mon-short/dn /fields/tue/dn /fields/year/dn /fields/year/relative/0 /NumberElements/latn/symbols/decimal /NumberElements/minimalPairs/one /units/length/meter/one"},
		// A rule on a type-suffixed key, as ICU writes in a source file,
		// matches nothing: the key a bundle holds has no suffix.
		{[]string{"+/", "-/*/mon-short:alias"}, keys(n)},
		// A rule overrides what the rules said below it before.
		{[]string{"+/", "-/fields/year/relative", "+/fields"}, keys(n)},
		{[]string{"-/", "+/fields/*/dn"}, "/fields/mon/dn /fields/mon-short/dn /fields/tue/dn /fields/year/dn"},
	} {
		if got := keys(filterOf(t, c.rules...).Apply(n)); got != c.want {
			t.Errorf("%v:\n got  %s\n want %s", c.rules, got, c.want)
		}
	}
}

func TestRuleErrors(t *testing.T) {
	for _, r := range []string{"", "fields", "+fields", "+/fields/", "-//x"} {
		if _, _, err := parseRule(r); err == nil {
			t.Errorf("%q parsed", r)
		}
	}
}

const filters = `// A comment, as the build tool strips it.
{
  "localeFilter": {
    "filterType": "language",
    "includelist": [
// Another.
      "root",
      "en",
      "ne"
    ]
  },
  "featureFilters": {
    "unit_tree": {"filterType": "language", "includelist": ["en"]},
    "rbnf_tree": {"includelist": ["ja"]},
    "stringprep": {"filterType": "exclude"},
    "coll_tree": {"filterType": "union", "unionOf": [{"includelist": ["en_US"]}, {"filterType": "language", "includelist": ["ne"]}]},
    "normalization": {"excludelist": ["nfkc_scf"]}
  },
  "resourceFilters": [
    {"categories": ["locales_tree"], "rules": ["-/*/mon", "-/NumberElements/minimalPairs"]},
    {"categories": ["locales_tree"], "files": {"includelist": ["ne"]}, "rules": ["-/*", "+/%%ALIAS", "+/%%Parent", "+/Version"]},
    {"categories": ["unit_tree"], "rules": ["-/units", "+/*/*/meter"]}
  ]
}`

// Files are kept by the category's feature filter, or for a tree by the
// locale filter, whose language filter keeps a language's every locale and
// the root.
func TestFiles(t *testing.T) {
	c, err := Parse([]byte(filters))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		category, stem string
		want           bool
	}{
		{"locales_tree", "root", true},
		{"locales_tree", "en_GB", true},
		{"locales_tree", "ne_IN", true},
		{"locales_tree", "dsb", false},
		{"unit_tree", "en_GB", true},
		{"unit_tree", "ne", false},
		{"unit_tree", "root", true},
		{"rbnf_tree", "ja", true},
		{"rbnf_tree", "root", false},
		{"coll_tree", "en_US", true},
		{"coll_tree", "en", false},
		{"coll_tree", "ne_IN", true},
		{"stringprep", "rfc3491", false},
		{"normalization", "nfkc", true},
		{"normalization", "nfkc_scf", false},
		{"misc", "supplementalData", true},
		{"brkitr_lstm", "Thai_codepoints_exclusive", false},
	} {
		if got := c.KeepFile(x.category, x.stem); got != x.want {
			t.Errorf("KeepFile(%s, %s) = %v, want %v", x.category, x.stem, got, x.want)
		}
	}
}

// A file's rules are the start rule and each entry's whose files it is
// among, in order.
func TestRules(t *testing.T) {
	c, err := Parse([]byte(filters))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := icutxt.Parse(bundle)
	for _, x := range []struct {
		category, stem, want string
	}{
		{"locales_tree", "en", "/%%Parent /Version /calendar/gregorian/dayNames/x /calendar/gregorian/eras/y /fields/mon-short/dn /fields/tue/dn /fields/year/dn /fields/year/relative/0 /NumberElements/latn/symbols/decimal /units/length/meter/one /units/length/foot/one"},
		{"locales_tree", "ne", "/%%Parent /Version"},
		{"unit_tree", "en", "/%%Parent /Version /calendar/gregorian/dayNames/x /calendar/gregorian/eras/y /fields/mon/dn /fields/mon-short/dn /fields/tue/dn /fields/year/dn /fields/year/relative/0 /NumberElements/latn/symbols/decimal /NumberElements/minimalPairs/one /units/length/meter/one"},
	} {
		if got := keys(c.Rules(x.category, x.stem).Apply(n)); got != x.want {
			t.Errorf("%s %s:\n got  %s\n want %s", x.category, x.stem, got, x.want)
		}
	}
	if c.Rules("zone_tree", "en") != nil {
		t.Error("rules for a category no entry names")
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{
		`{"strategy": "sideways"}`,
		`{"localeFilter": {"filterType": "language"}}`,
		`{"featureFilters": {"x": "sometimes"}}`,
		`{"resourceFilters": [{"categories": ["misc"], "rules": ["/x"]}]}`,
		`{"localeFilter": {"filterType": "locale", "includelist": ["en"]}}`,
		`  // an indented comment is not one
{}`,
	} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("%s parsed", s)
		}
	}
}
