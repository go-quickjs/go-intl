// Command numbergen writes the number tables the intl package carries.
//
// It reads CLDR's cldr-numbers-full package, which is 37 MB unpacked and so is
// not vendored: SOURCES.md pins the release and its checksum, and the path to
// an unpacked copy is given here.
//
//	curl -sLO https://registry.npmjs.org/cldr-numbers-full/-/cldr-numbers-full-48.2.0.tgz
//	tar xzf cldr-numbers-full-48.2.0.tgz
//	go run ./internal/numbergen package icu4c-78.3-data.zip
//
// The second argument is ICU's data sources, pinned likewise, for the one
// thing cldr-json leaves out: what the root says about writing numbers in each
// numbering system. CLDR's root.xml gives Arabic digits Arabic separators in
// every locale, and cldr-json's root has only the Latin entries.
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
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/numdata"
)

//go:embed numberingSystems.json
var numberingSystemsJSON []byte

//go:embed currencyData.json
var currencyDataJSON []byte

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/numbergen <cldr-numbers-full/package> <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "numbergen:", err)
		os.Exit(1)
	}
}

type numberingSystems struct {
	Supplemental struct {
		NumberingSystems map[string]struct {
			Type   string `json:"_type"`
			Digits string `json:"_digits"`
		} `json:"numberingSystems"`
	} `json:"supplemental"`
}

// numbersFile is the shape of one locale's numbers.json. The interesting keys
// carry the numbering system in their name -- "symbols-numberSystem-latn" --
// so they are read from a loose map rather than named here.
type numbersFile struct {
	Main map[string]struct {
		Numbers map[string]json.RawMessage `json:"numbers"`
	} `json:"main"`
}

// currenciesFile reads a locale's currency names. The spelled-out names carry
// their plural category in the key -- "displayName-count-one" -- so they come
// out of a loose map rather than being named here.
type currenciesFile struct {
	Main map[string]struct {
		Numbers struct {
			Currencies map[string]map[string]string `json:"currencies"`
		} `json:"numbers"`
	} `json:"main"`
}

func run(root, icuData string) error {
	icu, err := icusrc.OpenLocales(icuData)
	if err != nil {
		return err
	}
	defer icu.Close()
	var systems numberingSystems
	if err := json.Unmarshal(numberingSystemsJSON, &systems); err != nil {
		return fmt.Errorf("reading numberingSystems.json: %w", err)
	}
	digits := map[string]string{}
	for name, s := range systems.Supplemental.NumberingSystems {
		if s.Type == "numeric" {
			digits[name] = s.Digits
		}
	}

	main := filepath.Join(root, "main")
	entries, err := os.ReadDir(main)
	if err != nil {
		return fmt.Errorf("reading %s: %w", main, err)
	}

	// Everything is built before anything is written, so a failure partway
	// leaves the tracked tables as a matched set.
	built := map[string][]byte{}
	locales := map[string]*numdata.Locale{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		l, err := readLocale(main, e.Name(), digits)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if l == nil {
			continue
		}
		if l.Partials, err = partials(icu, e.Name(), l, digits); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if l.TimeSeparators, err = timeSeparators(icu, e.Name(), digits); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		built[e.Name()] = numdata.Encode(l)
		locales[e.Name()] = l
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}
	posix, err := posixLocale(icu, locales)
	if err != nil {
		return fmt.Errorf("en-US-posix: %w", err)
	}
	built["en-US-posix"] = numdata.Encode(posix)

	fractions, err := currencyDigitsTable()
	if err != nil {
		return err
	}
	rootSystems, err := readRootSystems(icu, digits)
	if err != nil {
		return err
	}
	systemsTable := numdata.EncodeSystems(rootSystems)

	out := filepath.Join("data", "numbers")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "currencydigits.bin"), fractions, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "numberingsystems.bin"), systemsTable, 0o644); err != nil {
		return err
	}
	// Anything left from a previous run for a locale CLDR no longer has would
	// otherwise be served forever.
	if err := datawrite.Locales(out, built); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "numbergen: %d locales\n", len(built))
	return nil
}

// readLocale builds one locale's data: its default numbering system and every
// other system its file has, each with the marks and patterns CLDR gives it.
// A locale whose file names a numbering system with no digits is skipped
// rather than written half-formed.
func readLocale(main, name string, digits map[string]string) (*numdata.Locale, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, "numbers.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var file numbersFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("numbers.json: %w", err)
	}
	entry, ok := file.Main[name]
	if !ok {
		return nil, fmt.Errorf("numbers.json names no %s", name)
	}

	var system string
	if err := json.Unmarshal(entry.Numbers["defaultNumberingSystem"], &system); err != nil {
		return nil, fmt.Errorf("the default numbering system: %w", err)
	}
	def, err := readSystem(entry.Numbers, system, digits)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", system, err)
	}
	out := &numdata.Locale{System: *def}

	// The file is resolved, so it carries every system the locale or its
	// parents define: usually Latin digits and the locale's native ones.
	var others []string
	for key := range entry.Numbers {
		if other, ok := strings.CutPrefix(key, "symbols-numberSystem-"); ok && other != system {
			others = append(others, other)
		}
	}
	sort.Strings(others)
	for _, other := range others {
		s, err := readSystem(entry.Numbers, other, digits)
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

	if raw, ok := entry.Numbers["minimumGroupingDigits"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("minimumGroupingDigits: %w", err)
		}
		if out.MinimumGroupingDigits, err = strconv.Atoi(s); err != nil {
			return nil, fmt.Errorf("minimumGroupingDigits: %w", err)
		}
	}
	if out.MinimumGroupingDigits < 1 {
		out.MinimumGroupingDigits = 1
	}

	// The patterns that join an amount to a spelled-out currency name do not
	// vary by numbering system in any locale, so the default's are kept.
	var currencyPatterns map[string]json.RawMessage
	if err := unmarshalKey(entry.Numbers, "currencyFormats-numberSystem-"+system,
		&currencyPatterns); err != nil {
		return nil, err
	}
	for key, raw := range currencyPatterns {
		count, ok := strings.CutPrefix(key, "unitPattern-count-")
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil || text == "" {
			continue
		}
		out.UnitPatterns = append(out.UnitPatterns,
			numdata.CountedText{Count: count, Text: text})
	}
	sort.Slice(out.UnitPatterns, func(i, j int) bool {
		return out.UnitPatterns[i].Count < out.UnitPatterns[j].Count
	})

	if out.Currencies, err = readCurrencies(main, name); err != nil {
		return nil, err
	}
	return out, nil
}

// cldrSymbols is how CLDR's JSON writes a system's marks.
type cldrSymbols struct {
	Decimal         string `json:"decimal"`
	Group           string `json:"group"`
	PercentSign     string `json:"percentSign"`
	PlusSign        string `json:"plusSign"`
	MinusSign       string `json:"minusSign"`
	Exponential     string `json:"exponential"`
	NaN             string `json:"nan"`
	Infinity        string `json:"infinity"`
	CurrencyDecimal string `json:"currencyDecimal"`
	CurrencyGroup   string `json:"currencyGroup"`
	Approximately   string `json:"approximatelySign"`
}

func (c cldrSymbols) symbols() numdata.Symbols {
	return numdata.Symbols{
		Decimal: c.Decimal, Group: c.Group,
		PercentSign: c.PercentSign, PlusSign: c.PlusSign,
		MinusSign: c.MinusSign, Exponential: c.Exponential,
		NaN: c.NaN, Infinity: c.Infinity,
		CurrencyDecimal: c.CurrencyDecimal, CurrencyGroup: c.CurrencyGroup,
		ApproximatelySign: c.Approximately,
	}
}

// readSystem reads one numbering system's marks and patterns from a locale's
// numbers.
func readSystem(numbers map[string]json.RawMessage, system string, digits map[string]string) (*numdata.System, error) {
	out := &numdata.System{NumberingSystem: system}
	d, ok := digits[system]
	if !ok {
		return nil, fmt.Errorf("no digits for the numbering system")
	}
	if d != "0123456789" {
		out.Digits = d
	}

	var symbols cldrSymbols
	if err := unmarshalKey(numbers, "symbols-numberSystem-"+system, &symbols); err != nil {
		return nil, err
	}
	out.Symbols = symbols.symbols()

	var decimal struct {
		Standard string                       `json:"standard"`
		Short    map[string]map[string]string `json:"short"`
		Long     map[string]map[string]string `json:"long"`
	}
	var percent struct {
		Standard string `json:"standard"`
	}
	type spacing struct {
		CurrencyMatch    string `json:"currencyMatch"`
		SurroundingMatch string `json:"surroundingMatch"`
		InsertBetween    string `json:"insertBetween"`
	}
	var currency struct {
		Standard        string `json:"standard"`
		Accounting      string `json:"accounting"`
		CurrencySpacing struct {
			Before spacing `json:"beforeCurrency"`
			After  spacing `json:"afterCurrency"`
		} `json:"currencySpacing"`
	}
	if err := unmarshalKey(numbers, "decimalFormats-numberSystem-"+system, &decimal); err != nil {
		return nil, err
	}
	if err := unmarshalKey(numbers, "percentFormats-numberSystem-"+system, &percent); err != nil {
		return nil, err
	}
	if err := unmarshalKey(numbers, "currencyFormats-numberSystem-"+system, &currency); err != nil {
		return nil, err
	}
	out.DecimalPattern, out.PercentPattern = decimal.Standard, percent.Standard
	out.CurrencyPattern, out.AccountingPattern = currency.Standard, currency.Accounting
	out.BeforeCurrency = numdata.Spacing(currency.CurrencySpacing.Before)
	out.AfterCurrency = numdata.Spacing(currency.CurrencySpacing.After)
	if out.DecimalPattern == "" || out.Symbols.Decimal == "" {
		return nil, fmt.Errorf("no decimal pattern or separator")
	}
	out.CompactShort = compactPatterns(decimal.Short["decimalFormat"])
	out.CompactLong = compactPatterns(decimal.Long["decimalFormat"])
	// A system with no range pattern of its own takes the Latin one; the
	// caller fills that in.
	if raw, ok := numbers["miscPatterns-numberSystem-"+system]; ok {
		var misc struct {
			Range string `json:"range"`
		}
		if err := json.Unmarshal(raw, &misc); err != nil {
			return nil, fmt.Errorf("miscPatterns: %w", err)
		}
		out.RangePattern = misc.Range
	}
	return out, nil
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

func readCurrencies(main, name string) ([]numdata.Currency, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, "currencies.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var file currenciesFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("currencies.json: %w", err)
	}
	entry, ok := file.Main[name]
	if !ok {
		return nil, nil
	}
	out := make([]numdata.Currency, 0, len(entry.Numbers.Currencies))
	for code, fields := range entry.Numbers.Currencies {
		c := numdata.Currency{
			Code:   fields["symbol"],
			Symbol: fields["symbol"],
			Narrow: fields["symbol-alt-narrow"],
		}
		c.Code = code
		c.DisplayName = fields["displayName"]
		// A currency whose symbol is its code carries no information: the
		// formatter writes the code when it finds nothing.
		if c.Symbol == code {
			c.Symbol = ""
		}
		for key, text := range fields {
			count, ok := strings.CutPrefix(key, "displayName-count-")
			if !ok || text == "" {
				continue
			}
			c.Names = append(c.Names, numdata.CountedText{Count: count, Text: text})
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

func unmarshalKey(m map[string]json.RawMessage, key string, into any) error {
	raw, ok := m[key]
	if !ok {
		return fmt.Errorf("no %q", key)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

// currencyDigits reads how many decimals each currency is written with. It is
// supplemental rather than per-locale: the yen has none anywhere.
type currencyData struct {
	Supplemental struct {
		CurrencyData struct {
			Fractions map[string]struct {
				Digits string `json:"_digits"`
			} `json:"fractions"`
		} `json:"currencyData"`
	} `json:"supplemental"`
}

// CurrencyDigits is written as its own table, since it does not vary by
// locale. It is a sorted list of "CODE:digits" so the reader can search it.
func currencyDigitsTable() ([]byte, error) {
	var data currencyData
	if err := json.Unmarshal(currencyDataJSON, &data); err != nil {
		return nil, fmt.Errorf("reading currencyData.json: %w", err)
	}
	fractions := data.Supplemental.CurrencyData.Fractions
	codes := make([]string, 0, len(fractions))
	for code := range fractions {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	var b strings.Builder
	for _, code := range codes {
		digits := fractions[code].Digits
		if digits == "" {
			continue
		}
		fmt.Fprintf(&b, "%s:%s\n", code, digits)
	}
	return []byte(b.String()), nil
}

// compactPatterns reads the compact forms, whose keys carry both the magnitude
// and the plural category they apply to: "1000-count-one" is how one thousand
// is written. The magnitude is kept as its power of ten.
func compactPatterns(in map[string]string) []numdata.CompactPattern {
	out := make([]numdata.CompactPattern, 0, len(in))
	for key, pattern := range in {
		magnitude, count, ok := strings.Cut(key, "-count-")
		if !ok {
			continue
		}
		// A pattern of a bare "0" means the locale writes this magnitude out
		// in full rather than compacting it, and carries no information.
		if strings.TrimSpace(pattern) == "0" {
			continue
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

// posixLocale is ICU's one variant locale, en_US_POSIX, which cldr-json
// does not carry and "-u-va-posix" asks for: what en-US resolves to, with
// the Latin patterns and marks ICU's en_US_POSIX gives on top -- numbers
// written without grouping, "INF" for infinity. What it does not give is
// en-US's, as ICU inherits it.
func posixLocale(c *icusrc.Locales, locales map[string]*numdata.Locale) (*numdata.Locale, error) {
	base := locales["en-US"]
	if base == nil {
		base = locales["en"]
	}
	if base == nil {
		return nil, fmt.Errorf("no en-US or en to build on")
	}
	if base.NumberingSystem != "latn" {
		return nil, fmt.Errorf("en writes %s digits", base.NumberingSystem)
	}
	own, err := c.Get("en_US_POSIX")
	if err != nil {
		return nil, err
	}
	if own == nil {
		return nil, fmt.Errorf("ICU has no en_US_POSIX")
	}
	out := *base
	fields := map[string]*string{
		"symbols/decimal":           &out.Symbols.Decimal,
		"symbols/group":             &out.Symbols.Group,
		"symbols/percentSign":       &out.Symbols.PercentSign,
		"symbols/plusSign":          &out.Symbols.PlusSign,
		"symbols/minusSign":         &out.Symbols.MinusSign,
		"symbols/exponential":       &out.Symbols.Exponential,
		"symbols/nan":               &out.Symbols.NaN,
		"symbols/infinity":          &out.Symbols.Infinity,
		"symbols/currencyDecimal":   &out.Symbols.CurrencyDecimal,
		"symbols/currencyGroup":     &out.Symbols.CurrencyGroup,
		"patterns/decimalFormat":    &out.DecimalPattern,
		"patterns/percentFormat":    &out.PercentPattern,
		"patterns/currencyFormat":   &out.CurrencyPattern,
		"patterns/accountingFormat": &out.AccountingPattern,
	}
	for key, field := range fields {
		part, name, _ := strings.Cut(key, "/")
		if v := own.Get("NumberElements", "latn", part, name); v != nil {
			if v.Alias {
				return nil, fmt.Errorf("%s is an alias", key)
			}
			*field = v.Value
		}
	}
	return &out, nil
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
