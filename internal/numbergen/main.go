// Command numbergen writes the number tables the intl package carries.
//
// It reads CLDR's cldr-numbers-full package, which is 37 MB unpacked and so is
// not vendored: SOURCES.md pins the release and its checksum, and the path to
// an unpacked copy is given here.
//
//	curl -sLO https://registry.npmjs.org/cldr-numbers-full/-/cldr-numbers-full-48.0.0.tgz
//	tar xzf cldr-numbers-full-48.0.0.tgz
//	go run ./internal/numbergen package
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

	"github.com/go-quickjs/go-intl/internal/numdata"
)

//go:embed numberingSystems.json
var numberingSystemsJSON []byte

//go:embed currencyData.json
var currencyDataJSON []byte

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/numbergen <cldr-numbers-full/package>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
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

type currenciesFile struct {
	Main map[string]struct {
		Numbers struct {
			Currencies map[string]struct {
				Symbol       string `json:"symbol"`
				SymbolNarrow string `json:"symbol-alt-narrow"`
			} `json:"currencies"`
		} `json:"numbers"`
	} `json:"main"`
}

func run(root string) error {
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
		built[e.Name()] = numdata.Encode(l)
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}

	fractions, err := currencyDigitsTable()
	if err != nil {
		return err
	}

	out := filepath.Join("data", "numbers")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("data", "currencydigits.bin"), fractions, 0o644); err != nil {
		return err
	}
	// Anything left from a previous run for a locale CLDR no longer has would
	// otherwise be served forever.
	old, _ := filepath.Glob(filepath.Join(out, "*.bin"))
	for _, name := range old {
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(built))
	for name := range built {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := filepath.Join(out, name+".bin")
		if err := os.WriteFile(target, built[name], 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "numbergen: %d locales\n", len(names))
	return nil
}

// readLocale builds one locale's data. A locale whose file names a numbering
// system with no digits is skipped rather than written half-formed.
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
	out := &numdata.Locale{NumberingSystem: system}
	if d, ok := digits[system]; ok && d != "0123456789" {
		out.Digits = d
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

	var symbols struct {
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
	}
	if err := unmarshalKey(entry.Numbers, "symbols-numberSystem-"+system, &symbols); err != nil {
		return nil, err
	}
	out.Symbols = numdata.Symbols{
		Decimal: symbols.Decimal, Group: symbols.Group,
		PercentSign: symbols.PercentSign, PlusSign: symbols.PlusSign,
		MinusSign: symbols.MinusSign, Exponential: symbols.Exponential,
		NaN: symbols.NaN, Infinity: symbols.Infinity,
		CurrencyDecimal: symbols.CurrencyDecimal, CurrencyGroup: symbols.CurrencyGroup,
	}

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
	if err := unmarshalKey(entry.Numbers, "decimalFormats-numberSystem-"+system, &decimal); err != nil {
		return nil, err
	}
	if err := unmarshalKey(entry.Numbers, "percentFormats-numberSystem-"+system, &percent); err != nil {
		return nil, err
	}
	if err := unmarshalKey(entry.Numbers, "currencyFormats-numberSystem-"+system, &currency); err != nil {
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

	if out.Currencies, err = readCurrencies(main, name); err != nil {
		return nil, err
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
	for code, c := range entry.Numbers.Currencies {
		// A currency whose symbol is its code carries no information: the
		// formatter writes the code when it finds nothing.
		if c.Symbol == code {
			c.Symbol = ""
		}
		if c.Symbol == "" && c.SymbolNarrow == "" {
			continue
		}
		out = append(out, numdata.Currency{
			Code: code, Symbol: c.Symbol, Narrow: c.SymbolNarrow,
		})
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
