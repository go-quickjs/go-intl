package intl

import (
	"fmt"
	"sort"
	"strings"
)

// The lists Intl.supportedValuesOf answers with. Calendars, numbering
// systems and units are go-intl's own; collations, currencies and time zones
// are V8's, built from ICU's (data/values.bin; see internal/availgen):
// every collation ICU's collation data names, the common current ISO
// currencies with an English name, and ICU's canonical time zones that are
// in a region.

// Calendars lists the calendars a date can be written in, sorted.
func Calendars() []string {
	out := make([]string, 0, len(implemented))
	for c := range implemented {
		out = append(out, string(c))
	}
	sort.Strings(out)
	return out
}

// Collations lists the collations, sorted.
func Collations() ([]string, error) { return CollationsFrom(Embedded) }

// CollationsFrom is Collations for a source of the caller's own.
func CollationsFrom(src Source) ([]string, error) { return values(src, "collation") }

// Currencies lists the currencies, by ISO code, sorted.
func Currencies() ([]string, error) { return CurrenciesFrom(Embedded) }

// CurrenciesFrom is Currencies for a source of the caller's own.
func CurrenciesFrom(src Source) ([]string, error) { return values(src, "currency") }

// TimeZones lists the time zones, by their canonical identifiers, sorted.
func TimeZones() ([]string, error) { return TimeZonesFrom(Embedded) }

// TimeZonesFrom is TimeZones for a source of the caller's own.
func TimeZonesFrom(src Source) ([]string, error) { return values(src, "timezone") }

func values(src Source, key string) ([]string, error) {
	b, err := src.Open(MarkerValues, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the supported values: %w", err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, " "); ok && k == key {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("intl: no supported values for %s", key)
	}
	return out, nil
}
