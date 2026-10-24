// Command numbergen writes the number tables the intl package carries.
//
// It reads ICU's locale and currency bundles, data/locales/*.txt and
// data/curr/*.txt in the data archive SOURCES.md pins: each locale's
// numbering systems, the marks and patterns it writes numbers with in each,
// and its currencies' symbols and names, as ICU resolves them, and what the
// root says about writing numbers in each numbering system.
//
//	go run ./internal/numbergen [-seed pool.bin] icu4c-78.3-data.zip
//
// -seed numbers the pool's parts after an earlier pool's (datawrite.NewPool).
//
// The two small supplemental files it also needs -- which digits a numbering
// system writes, and how many decimals a currency is written with -- are
// vendored beside this command, being a few tens of kilobytes each.
//
// What is written is what CLDR says: the symbols a locale writes numbers with
// and the patterns it writes them in, kept as patterns. Nothing here formats a
// number, and no table holds one.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/numdata"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

func main() {
	seed := flag.String("seed", "", "a pool to number the parts after")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/numbergen [-seed pool.bin] <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *seed); err != nil {
		fmt.Fprintln(os.Stderr, "numbergen:", err)
		os.Exit(1)
	}
}

func run(icuData, seed string) error {
	icu, err := icusrc.OpenLocales(icuData)
	if err != nil {
		return err
	}
	defer icu.Close()
	curr, err := icusrc.OpenTree(icuData, "curr")
	if err != nil {
		return err
	}
	defer curr.Close()
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}
	digits, err := numericSystems(icu)
	if err != nil {
		return err
	}
	currencies, err := curr.Supplemental()
	if err != nil {
		return err
	}

	// Everything is built before anything is written, so a failure partway
	// leaves the tracked tables as a matched set. The locales share one
	// pool of strings, systems and currencies, written beside them;
	// EachLocale visits the bundles in order, so the pool is the same every
	// run.
	pool, err := datawrite.NewPool(numdata.Version, seed)
	if err != nil {
		return err
	}
	built, err := icusrc.EachLocale(icu, fb, func(v *icusrc.View) ([]byte, error) {
		name := v.Name()
		cv, err := curr.View(name, fb)
		if err != nil {
			return nil, err
		}
		l, err := readLocale(v, cv, digits)
		if err != nil || l == nil {
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			return nil, nil
		}
		if l.Partials, err = partials(icu, name, l, digits); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if l.TimeSeparators, err = timeSeparators(icu, name, digits); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if l.CurrencyFormats, err = currencyFormats(curr, fb, name); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if l.UnitPatterns, err = currencyUnitPatterns(curr, fb, name); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		systems := []*numdata.System{&l.System}
		for i := range l.Others {
			systems = append(systems, &l.Others[i])
		}
		for _, s := range systems {
			if s.CompactLong, err = compactLong(icu, fb, name, s); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
		}
		return numdata.Encode(l, pool), nil
	})
	if err != nil {
		return err
	}

	fractions, err := currencyDigitsTable(currencies)
	if err != nil {
		return err
	}
	regions, err := currencyRegionsTable(currencies)
	if err != nil {
		return err
	}
	rootSystems, err := readRootSystems(icu, digits)
	if err != nil {
		return err
	}
	systemsTable := numdata.EncodeSystems(rootSystems)

	set := writeset.New()
	set.File(filepath.Join("data", "currencydigits.bin"), fractions)
	set.File(filepath.Join("data", "currencyregions.bin"), regions)
	set.File(filepath.Join("data", "numberingsystems.bin"), systemsTable)
	// Anything left from a previous run for a locale ICU no longer has would
	// otherwise be served forever.
	if err := datawrite.AddLocales(set, filepath.Join("data", "numbers"), built); err != nil {
		return err
	}
	set.File(filepath.Join("data", "numbersshared.bin"), pool.Bytes())
	if err := set.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "numbergen: %d locales\n", len(built))
	return nil
}

// readLocale builds one locale's data: its default numbering system and the
// others CLDR lists for it -- its native, traditional and finance systems
// where they write digits, and Latin -- each with the marks and patterns
// ICU resolves for it.
func readLocale(v, curr *icusrc.View, digits map[string]string) (*numdata.Locale, error) {
	def, err := v.Get("NumberElements", "default")
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, nil
	}
	system := def.Value
	spacing, err := currencySpacing(curr)
	if err != nil {
		return nil, err
	}
	d, err := readSystem(v, system, digits, spacing)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", system, err)
	}
	out := &numdata.Locale{System: *d}

	listed := map[string]bool{"latn": true}
	for _, key := range []string{"native", "traditional", "finance"} {
		n, err := v.Get("NumberElements", key)
		if err != nil {
			return nil, err
		}
		if _, numeric := digits[nodeValue(n)]; numeric {
			listed[n.Value] = true
		}
	}
	delete(listed, system)
	others := make([]string, 0, len(listed))
	for other := range listed {
		others = append(others, other)
	}
	sort.Strings(others)
	for _, other := range others {
		s, err := readSystem(v, other, digits, spacing)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", other, err)
		}
		out.Others = append(out.Others, *s)
	}
	// The root aliases every system's range pattern to the Latin one.
	latin := out.RangePattern
	if system != "latn" {
		for _, s := range out.Others {
			if s.NumberingSystem == "latn" {
				latin = s.RangePattern
			}
		}
	}
	if out.RangePattern == "" {
		out.RangePattern = latin
	}
	for i := range out.Others {
		if out.Others[i].RangePattern == "" {
			out.Others[i].RangePattern = latin
		}
	}
	// CompactData::populate: a system with no currency compact patterns of
	// its own takes the Latin system's.
	latinCompact := out.CurrencyCompact
	for _, s := range out.Others {
		if s.NumberingSystem == "latn" {
			latinCompact = s.CurrencyCompact
		}
	}
	if len(out.CurrencyCompact) == 0 {
		out.CurrencyCompact = latinCompact
	}
	for i := range out.Others {
		if len(out.Others[i].CurrencyCompact) == 0 {
			out.Others[i].CurrencyCompact = latinCompact
		}
	}

	grouping, err := v.Get("NumberElements", "minimumGroupingDigits")
	if err != nil {
		return nil, err
	}
	if grouping != nil {
		if out.MinimumGroupingDigits, err = strconv.Atoi(grouping.Value); err != nil {
			return nil, fmt.Errorf("minimumGroupingDigits: %w", err)
		}
	}
	if out.MinimumGroupingDigits < 1 {
		out.MinimumGroupingDigits = 1
	}

	if out.Currencies, err = readCurrencies(curr); err != nil {
		return nil, err
	}
	return out, nil
}

// nodeValue is a resource's text, or "" for none.
func nodeValue(n *icutxt.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

// currencySpacing is the spacing a locale's currency bundle puts between a
// symbol and the number, before and after it.
func currencySpacing(curr *icusrc.View) ([2]numdata.Spacing, error) {
	var out [2]numdata.Spacing
	for i, side := range []string{"beforeCurrency", "afterCurrency"} {
		t, err := curr.Get("currencySpacing", side)
		if err != nil {
			return out, err
		}
		if t == nil {
			continue
		}
		out[i] = numdata.Spacing{
			CurrencyMatch:    nodeValue(t.Get("currencyMatch")),
			SurroundingMatch: nodeValue(t.Get("surroundingMatch")),
			InsertBetween:    nodeValue(t.Get("insertBetween")),
		}
	}
	return out, nil
}

// readSystem reads one numbering system's marks and patterns from a locale's
// NumberElements.
func readSystem(v *icusrc.View, system string, digits map[string]string, spacing [2]numdata.Spacing) (*numdata.System, error) {
	out := &numdata.System{NumberingSystem: system}
	d, ok := digits[system]
	if !ok {
		return nil, fmt.Errorf("no digits for the numbering system")
	}
	if d != "0123456789" {
		out.Digits = d
	}
	elements, err := v.Get("NumberElements", system)
	if err != nil {
		return nil, err
	}
	if elements == nil {
		return nil, fmt.Errorf("no NumberElements")
	}
	// What a system does not give, ICU takes from the locale's Latin
	// system, as CLDR's root aliases each system's data to it.
	latin, err := v.Get("NumberElements", "latn")
	if err != nil {
		return nil, err
	}
	get := func(path ...string) string {
		if s := nodeValue(elements.Get(path...)); s != "" || latin == nil {
			return s
		}
		return nodeValue(latin.Get(path...))
	}
	out.Symbols = numdata.Symbols{
		Decimal: get("symbols", "decimal"), Group: get("symbols", "group"),
		PercentSign: get("symbols", "percentSign"), PlusSign: get("symbols", "plusSign"),
		MinusSign: get("symbols", "minusSign"), Exponential: get("symbols", "exponential"),
		NaN: get("symbols", "nan"), Infinity: get("symbols", "infinity"),
		CurrencyDecimal: get("symbols", "currencyDecimal"), CurrencyGroup: get("symbols", "currencyGroup"),
		ApproximatelySign: get("symbols", "approximatelySign"),
	}
	out.DecimalPattern, out.PercentPattern = get("patterns", "decimalFormat"), get("patterns", "percentFormat")
	out.CurrencyPattern, out.AccountingPattern = get("patterns", "currencyFormat"), get("patterns", "accountingFormat")
	out.BeforeCurrency, out.AfterCurrency = spacing[0], spacing[1]
	if out.DecimalPattern == "" || out.Symbols.Decimal == "" {
		return nil, fmt.Errorf("no decimal pattern or separator")
	}
	compact := func(key string) []numdata.CompactPattern {
		if p := compactPatterns(compactTable(elements.Get("patternsShort", key))); len(p) > 0 || latin == nil {
			return p
		}
		return compactPatterns(compactTable(latin.Get("patternsShort", key)))
	}
	out.CompactShort = compact("decimalFormat")
	out.CurrencyCompact = compact("currencyFormat")
	// A system with no range pattern of its own takes the Latin one; the
	// caller fills that in.
	out.RangePattern = get("miscPatterns", "range")
	return out, nil
}

// compactTable is a table of compact patterns, by magnitude and then plural
// category, keyed as compactPatterns takes them: "1000-count-one".
func compactTable(t *icutxt.Node) map[string]string {
	out := map[string]string{}
	if t == nil {
		return out
	}
	for _, magnitude := range t.Children {
		for _, c := range magnitude.Children {
			if !c.Table && !c.Alias {
				out[magnitude.Key+"-count-"+c.Key] = c.Value
			}
		}
	}
	return out
}

// readRootSystems reads what the root says about each numeric numbering system,
// from ICU's root.txt: CLDR's root.xml has it, and cldr-json does not.
//
// An entry either gives the system marks and patterns of its own or points,
// with a locale-relative alias, at the asking locale's Latin ones. An alias is
// kept as nothing, which the reader takes to mean exactly that.
func readRootSystems(icu *icusrc.Locales, digits map[string]string) ([]numdata.NumberingSystem, error) {
	root, err := icu.Get("root")
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("the data archive has no root.txt")
	}
	elements := root.Get("NumberElements")
	if elements == nil {
		return nil, fmt.Errorf("root.txt has no NumberElements")
	}

	names := make([]string, 0, len(digits))
	for name := range digits {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []numdata.NumberingSystem
	for _, name := range names {
		s := numdata.NumberingSystem{Name: name}
		if d := digits[name]; d != "0123456789" {
			s.Digits = d
		}
		entry := elements.Get(name)
		if entry == nil || name == "latn" {
			// Nothing of its own: the locale's Latin data serves.
			out = append(out, s)
			continue
		}
		if symbols := entry.Get("symbols"); symbols != nil && symbols.Table {
			get := func(key string) string {
				if n := symbols.Get(key); n != nil && !n.Alias {
					return n.Value
				}
				return ""
			}
			s.Symbols = &numdata.Symbols{
				Decimal: get("decimal"), Group: get("group"),
				PercentSign: get("percentSign"), PlusSign: get("plusSign"),
				MinusSign: get("minusSign"), Exponential: get("exponential"),
				NaN: get("nan"), Infinity: get("infinity"),
				ApproximatelySign: get("approximatelySign"),
			}
			if s.Symbols.Decimal == "" || s.Symbols.Group == "" {
				return nil, fmt.Errorf("root.txt: %s has symbols without separators", name)
			}
		}
		pattern := func(key string) (string, error) {
			n := entry.Get("patterns", key)
			for depth := 0; n != nil && n.Alias; depth++ {
				if depth > 4 {
					return "", fmt.Errorf("root.txt: %s %s aliases in a circle", name, key)
				}
				target, ok := strings.CutPrefix(n.Value, "/LOCALE/NumberElements/")
				if !ok {
					return "", fmt.Errorf("root.txt: %s %s aliases %q", name, key, n.Value)
				}
				parts := strings.Split(target, "/")
				if parts[0] == "latn" {
					return "", nil
				}
				if parts[0] != name || len(parts) != 3 || parts[1] != "patterns" {
					return "", fmt.Errorf("root.txt: %s %s aliases %q", name, key, n.Value)
				}
				n = entry.Get("patterns", parts[2])
			}
			if n == nil {
				return "", nil
			}
			return n.Value, nil
		}
		for _, f := range []struct {
			key string
			dst *string
		}{
			{"decimalFormat", &s.DecimalPattern},
			{"percentFormat", &s.PercentPattern},
			{"currencyFormat", &s.CurrencyPattern},
			{"accountingFormat", &s.AccountingPattern},
		} {
			if *f.dst, err = pattern(f.key); err != nil {
				return nil, err
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// readCurrencies reads a locale's currencies from its currency bundle: the
// symbol and the name Currencies gives each, the narrow symbol, and the
// names by plural category.
func readCurrencies(curr *icusrc.View) ([]numdata.Currency, error) {
	tables := map[string]*icutxt.Node{}
	codes := map[string]bool{}
	for _, key := range []string{"Currencies", "Currencies%narrow", "CurrencyPlurals"} {
		t, err := curr.Get(key)
		if err != nil {
			return nil, err
		}
		tables[key] = t
		if t != nil {
			for _, c := range t.Children {
				codes[c.Key] = true
			}
		}
	}
	out := make([]numdata.Currency, 0, len(codes))
	for code := range codes {
		c := numdata.Currency{Code: code}
		if e := tables["Currencies"].Get(code); e != nil {
			c.Symbol, c.DisplayName = element(e, 0), element(e, 1)
		}
		if n := tables["Currencies%narrow"].Get(code); n != nil {
			c.Narrow = n.Value
		}
		// A currency whose symbol or name is its code carries no
		// information: the formatter writes the code when it finds nothing.
		if c.Symbol == code {
			c.Symbol = ""
		}
		if c.DisplayName == code {
			c.DisplayName = ""
		}
		if names := tables["CurrencyPlurals"].Get(code); names != nil {
			for _, n := range names.Children {
				if n.Value != "" {
					c.Names = append(c.Names, numdata.CountedText{Count: n.Key, Text: n.Value})
				}
			}
		}
		sort.Slice(c.Names, func(i, j int) bool { return c.Names[i].Count < c.Names[j].Count })
		if c.Symbol == "" && c.Narrow == "" && c.DisplayName == "" && len(c.Names) == 0 {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

// element is the ith string of an array, whose strings are its values or,
// where it holds an array too, its children's.
func element(n *icutxt.Node, i int) string {
	if i < len(n.Values) {
		return n.Values[i]
	}
	if i < len(n.Children) && !n.Children[i].Table {
		return n.Children[i].Value
	}
	return ""
}

// numericSystems is the digits of each numbering system that has its own,
// misc/numberingSystems.txt's systems that are not algorithmic.
func numericSystems(icu *icusrc.Locales) (map[string]string, error) {
	file, err := icu.Misc("numberingSystems")
	if err != nil {
		return nil, err
	}
	systems := file.Get("numberingSystems")
	if systems == nil {
		return nil, fmt.Errorf("numberingSystems.txt has no numberingSystems")
	}
	digits := map[string]string{}
	for _, s := range systems.Children {
		algorithmic, desc := s.Get("algorithmic"), s.Get("desc")
		if algorithmic == nil || desc == nil {
			return nil, fmt.Errorf("numberingSystems.txt: %s", s.Key)
		}
		if algorithmic.Value == "0" {
			digits[s.Key] = desc.Value
		}
	}
	return digits, nil
}

// CurrencyDigits is written as its own table, since it does not vary by
// locale: the yen has none anywhere. It is a sorted list of "CODE:digits"
// so the reader can search it, from curr/supplementalData.txt's
// CurrencyMeta, whose first number is the digits.
func currencyDigitsTable(currencies *icutxt.Node) ([]byte, error) {
	meta := currencies.Get("CurrencyMeta")
	if meta == nil {
		return nil, fmt.Errorf("curr/supplementalData.txt has no CurrencyMeta")
	}
	var lines []string
	for _, c := range meta.Children {
		if len(c.Values) == 0 {
			return nil, fmt.Errorf("curr/supplementalData.txt: CurrencyMeta %s", c.Key)
		}
		lines = append(lines, c.Key+":"+c.Values[0])
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// currencyRegionsTable is each region's currency, as ICU's ucurr_forLocale
// takes it: the first of the region's currencies that is legal tender, or
// the first of them where none is. It is a sorted list of "REGION:CODE",
// from curr/supplementalData.txt's CurrencyMap, which lists a region's
// currencies the current first.
func currencyRegionsTable(currencies *icutxt.Node) ([]byte, error) {
	regions := currencies.Get("CurrencyMap")
	if regions == nil {
		return nil, fmt.Errorf("curr/supplementalData.txt has no CurrencyMap")
	}
	var lines []string
	for _, region := range regions.Children {
		code := ""
		for _, entry := range region.Children {
			id := entry.Get("id")
			if id == nil {
				return nil, fmt.Errorf("curr/supplementalData.txt: CurrencyMap %s", region.Key)
			}
			if code == "" {
				code = id.Value
			}
			if tender := entry.Get("tender"); tender == nil || tender.Value != "false" {
				code = id.Value
				break
			}
		}
		if code != "" {
			lines = append(lines, region.Key+":"+code)
		}
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// currencyUnitPatterns are the patterns that join an amount to a currency's
// name, by plural category, as ICU's LongNameHandler reads them: the curr
// tree's CurrencyUnitPatterns, each category from the first bundle that has
// it. cldr-json resolves them otherwise for a locale that does not give its
// own: Burmese, which inherits the root's "{0} {1}" in ICU, has "{1} {0}".
func currencyUnitPatterns(curr *icusrc.Locales, fb icusrc.Fallback, name string) ([]numdata.CountedText, error) {
	chain, err := curr.Resolve(strings.ReplaceAll(name, "-", "_"), fb)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []numdata.CountedText
	for _, n := range chain {
		patterns := n.Get("CurrencyUnitPatterns")
		if patterns == nil {
			continue
		}
		for _, p := range patterns.Children {
			if seen[p.Key] || p.Value == "" {
				continue
			}
			seen[p.Key] = true
			out = append(out, numdata.CountedText{Count: p.Key, Text: p.Value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count < out[j].Count })
	return out, nil
}

// currencyFormats are the currencies a locale writes by a pattern and
// separators of their own, as ICU's DecimalFormatSymbols::setCurrency reads
// them: the currency's entry in the first bundle of the curr tree that has
// one, where that entry has a third element, the pattern, the decimal
// separator and the group separator.
func currencyFormats(curr *icusrc.Locales, fb icusrc.Fallback, name string) ([]numdata.CurrencyFormat, error) {
	chain, err := curr.Resolve(strings.ReplaceAll(name, "-", "_"), fb)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []numdata.CurrencyFormat
	for _, n := range chain {
		currencies := n.Get("Currencies")
		if currencies == nil {
			continue
		}
		for _, e := range currencies.Children {
			if seen[e.Key] {
				continue
			}
			seen[e.Key] = true
			if len(e.Children) < 3 {
				continue
			}
			f := e.Children[2].Values
			if len(f) != 3 {
				return nil, fmt.Errorf("the format of %s has %d elements", e.Key, len(f))
			}
			out = append(out, numdata.CurrencyFormat{Code: e.Key, Pattern: f[0], Decimal: f[1], Group: f[2]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

// compactPatterns reads the compact forms, whose keys carry both the magnitude
// and the plural category they apply to: "1000-count-one" is how one thousand
// is written. The magnitude is kept as its power of ten.
func compactPatterns(in map[string]string) []numdata.CompactPattern {
	out := make([]numdata.CompactPattern, 0, len(in))
	for key, pattern := range in {
		magnitude, count, ok := strings.Cut(key, "-count-")
		// The alternates, "-alt-alphaNextToNumber", are not in ICU's data,
		// which keeps the plain patterns alone.
		if !ok || strings.Contains(count, "-alt-") {
			continue
		}
		// A pattern of a bare "0" means the locale writes this magnitude out
		// in full rather than compacting it. It is kept, as ICU keeps it
		// (USE_FALLBACK), so that the magnitude does not take the pattern of
		// one below it.
		if strings.TrimSpace(pattern) == "0" {
			pattern = "0"
		}
		exponent := len(magnitude) - 1
		if _, err := strconv.Atoi(magnitude); err != nil || exponent < 0 {
			continue
		}
		out = append(out, numdata.CompactPattern{
			Exponent: exponent, Count: count, Pattern: pattern,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Exponent != out[j].Exponent {
			return out[i].Exponent < out[j].Exponent
		}
		return out[i].Count < out[j].Count
	})
	return out
}

// compactLong is a system's long compact patterns as ICU's
// CompactData::populate reads them: the system's patternsLong, each
// magnitude and plural category from the first bundle along the chain that
// has it; where there are none, the Latin system's; and where there are
// none of those, the short ones. cldr-json fills a locale's long patterns
// out from the root's, which are its short ones, where ICU's root has no
// long patterns: Pashto writes 1234 in full, "۱۲۳۴", where it has a long
// pattern only for billions.
func compactLong(c *icusrc.Locales, fb icusrc.Fallback, name string, s *numdata.System) ([]numdata.CompactPattern, error) {
	chain, err := c.Resolve(strings.ReplaceAll(name, "-", "_"), fb)
	if err != nil {
		return nil, err
	}
	read := func(system string) (map[string]string, error) {
		merged := map[string]string{}
		for _, n := range chain {
			table := n.Get("NumberElements", system, "patternsLong", "decimalFormat")
			if table == nil {
				continue
			}
			if table.Alias {
				return nil, fmt.Errorf("%s's long patterns alias %s", system, table.Value)
			}
			for _, magnitude := range table.Children {
				for _, p := range magnitude.Children {
					key := magnitude.Key + "-count-" + p.Key
					if _, ok := merged[key]; !ok {
						merged[key] = p.Value
					}
				}
			}
		}
		return merged, nil
	}
	merged, err := read(s.NumberingSystem)
	if err != nil {
		return nil, err
	}
	if len(merged) == 0 && s.NumberingSystem != "latn" {
		if merged, err = read("latn"); err != nil {
			return nil, err
		}
	}
	if len(merged) == 0 {
		return s.CompactShort, nil
	}
	return compactPatterns(merged), nil
}

// timeSeparators finds the mark each numbering system puts between hours
// and minutes in a locale, as DateFormatSymbols::initializeData finds it:
// the first NumberElements/<system>/symbols table in the locale's chain,
// the root's included, following the root's "/LOCALE/" aliases back to the
// locale, and then that table's own timeSeparator, with no further fallback,
// or a colon. So Urdu in Persian digits writes a colon although the root
// gives those digits "٫": Urdu has an arabext symbols table of its own, and
// it names no separator. Only the systems whose mark is not a colon are
// kept.
func timeSeparators(c *icusrc.Locales, name string, digits map[string]string) ([]numdata.SystemText, error) {
	chain, err := c.Chain(strings.ReplaceAll(name, "-", "_"))
	if err != nil {
		return nil, err
	}
	root, err := c.Get("root")
	if err != nil {
		return nil, err
	}
	chain = append(chain, root)
	var lookup func(system string, depth int) (string, error)
	lookup = func(system string, depth int) (string, error) {
		for _, n := range chain {
			symbols := n.Get("NumberElements", system, "symbols")
			if symbols == nil {
				continue
			}
			if symbols.Alias {
				target, ok := strings.CutPrefix(symbols.Value, "/LOCALE/NumberElements/")
				target, ok2 := strings.CutSuffix(target, "/symbols")
				if !ok || !ok2 || depth > 4 {
					return "", fmt.Errorf("%s's symbols alias %s", system, symbols.Value)
				}
				return lookup(target, depth+1)
			}
			if t := symbols.Get("timeSeparator"); t != nil && !t.Alias {
				return t.Value, nil
			}
			return ":", nil
		}
		return ":", nil
	}
	names := make([]string, 0, len(digits))
	for system := range digits {
		names = append(names, system)
	}
	sort.Strings(names)
	var out []numdata.SystemText
	for _, system := range names {
		sep, err := lookup(system, 0)
		if err != nil {
			return nil, err
		}
		if sep != ":" {
			out = append(out, numdata.SystemText{System: system, Text: sep})
		}
	}
	return out, nil
}

// partials finds what a locale's ICU files say about numbering systems its
// CLDR file does not carry. cldr-json writes only the systems a locale uses,
// and drops the few marks and patterns a locale gives the others; ICU keeps
// them, and looks each field up through the locale before the root.
func partials(c *icusrc.Locales, name string, l *numdata.Locale, digits map[string]string) ([]numdata.System, error) {
	have := map[string]bool{l.NumberingSystem: true, "latn": true}
	for _, s := range l.Others {
		have[s.NumberingSystem] = true
	}
	chain, err := c.Chain(strings.ReplaceAll(name, "-", "_"))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(digits))
	for system := range digits {
		if !have[system] {
			names = append(names, system)
		}
	}
	sort.Strings(names)

	symbolKeys := []string{"decimal", "group", "percentSign", "plusSign", "minusSign",
		"exponential", "nan", "infinity", "currencyDecimal", "currencyGroup", "approximatelySign"}
	patternKeys := []string{"decimalFormat", "percentFormat", "currencyFormat", "accountingFormat"}
	var out []numdata.System
	for _, system := range names {
		p := numdata.System{NumberingSystem: system}
		found := false
		lookup := func(path ...string) (string, error) {
			for _, n := range chain {
				v := n.Get(append([]string{"NumberElements", system}, path...)...)
				if v == nil {
					continue
				}
				if v.Alias {
					return "", fmt.Errorf("%s aliases %s under %s", system, v.Value, path)
				}
				found = true
				return v.Value, nil
			}
			return "", nil
		}
		symbols := []*string{&p.Symbols.Decimal, &p.Symbols.Group, &p.Symbols.PercentSign,
			&p.Symbols.PlusSign, &p.Symbols.MinusSign, &p.Symbols.Exponential,
			&p.Symbols.NaN, &p.Symbols.Infinity, &p.Symbols.CurrencyDecimal, &p.Symbols.CurrencyGroup,
			&p.Symbols.ApproximatelySign}
		for i, key := range symbolKeys {
			if *symbols[i], err = lookup("symbols", key); err != nil {
				return nil, err
			}
		}
		patterns := []*string{&p.DecimalPattern, &p.PercentPattern, &p.CurrencyPattern, &p.AccountingPattern}
		for i, key := range patternKeys {
			if *patterns[i], err = lookup("patterns", key); err != nil {
				return nil, err
			}
		}
		if p.RangePattern, err = lookup("miscPatterns", "range"); err != nil {
			return nil, err
		}
		if found {
			out = append(out, p)
		}
	}
	return out, nil
}
