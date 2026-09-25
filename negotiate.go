package intl

import (
	"fmt"
	"strings"
)

// Locale negotiation: ECMA-402's ResolveLocale and SupportedLocales, which
// settle a list of requested locales on one a service is available in.
//
// Which locales a service is available in is V8's answer, built from ICU's
// (data/available.bin; see internal/availgen): a locale ICU has no data for
// is not available, so "az-Arab" resolves as "az", and "ht" as the default.

// A Service is one of ECMA-402's services, by its constructor's name.
type Service string

// The services, whose available locales differ.
const (
	ServiceCollator           Service = "Collator"
	ServiceDateTimeFormat     Service = "DateTimeFormat"
	ServiceDisplayNames       Service = "DisplayNames"
	ServiceDurationFormat     Service = "DurationFormat"
	ServiceListFormat         Service = "ListFormat"
	ServiceNumberFormat       Service = "NumberFormat"
	ServicePluralRules        Service = "PluralRules"
	ServiceRelativeTimeFormat Service = "RelativeTimeFormat"
	ServiceSegmenter          Service = "Segmenter"
)

// availableList is the list in data/available.bin each service reads, as V8
// shares them: DurationFormat reads NumberFormat's, RelativeTimeFormat and
// DisplayNames DateTimeFormat's.
var availableList = map[Service]string{
	ServiceCollator:           "collator",
	ServiceDateTimeFormat:     "date",
	ServiceDisplayNames:       "date",
	ServiceDurationFormat:     "number",
	ServiceListFormat:         "list",
	ServiceNumberFormat:       "number",
	ServicePluralRules:        "plural",
	ServiceRelativeTimeFormat: "date",
	ServiceSegmenter:          "all",
}

// MatcherKind is ECMA-402's localeMatcher option. Both kinds match by
// lookup: "best fit" is the implementation's to define, and V8 defines it as
// lookup unless --harmony_intl_best_fit_matcher is set, which Node does not
// set. Across every locale ICU has data for, alone and in pairs, Node's two
// matchers answer alike.
type MatcherKind int

const (
	// BestFit is ECMA-402's default.
	BestFit MatcherKind = iota
	// Lookup is BCP 47's lookup, by cutting the tag short.
	Lookup
)

// A LocaleMatcher resolves requested locales among one service's available
// locales. It never changes after it is built and is safe for any number of
// goroutines to share.
type LocaleMatcher struct {
	service   Service
	available map[string]bool
}

// NewLocaleMatcher reads a service's available locales from a source.
func NewLocaleMatcher(src Source, service Service) (*LocaleMatcher, error) {
	list, ok := availableList[service]
	if !ok {
		return nil, fmt.Errorf("intl: %q is not a service", service)
	}
	b, err := src.Open(MarkerAvailable, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the available locales: %w", err)
	}
	m := &LocaleMatcher{service: service, available: map[string]bool{}}
	for _, line := range strings.Split(string(b), "\n") {
		name, tag, ok := strings.Cut(line, " ")
		if ok && name == list {
			m.available[tag] = true
		}
	}
	if len(m.available) == 0 {
		return nil, fmt.Errorf("intl: no available locales for %s", service)
	}
	return m, nil
}

// Available reports whether the service is available in exactly this tag.
func (m *LocaleMatcher) Available(tag string) bool { return m.available[tag] }

// bestAvailable is ECMA-402's BestAvailableLocale: the tag, or the longest
// part of it the service is available in, cutting it short a subtag at a
// time and a singleton with the subtag after it; empty if none.
func (m *LocaleMatcher) bestAvailable(candidate string) string {
	for {
		if m.available[candidate] {
			return candidate
		}
		pos := strings.LastIndexByte(candidate, '-')
		if pos < 0 {
			return ""
		}
		if pos >= 2 && candidate[pos-2] == '-' {
			pos -= 2
		}
		candidate = candidate[:pos]
	}
}

// splitUnicodeExtension is V8's ParseBCP47Locale: the tag without its
// Unicode extension, and the extension, "-u-co-emoji". An extension inside
// private use does not count.
func splitUnicodeExtension(tag string) (base, extension string) {
	start := strings.Index(tag, "-u-")
	if start < 0 {
		return tag, ""
	}
	if x := strings.Index(tag, "-x-"); x >= 0 && x < start {
		return tag, ""
	}
	end := len(tag)
	for i := start + 1; i < len(tag)-2; i++ {
		if tag[i] != '-' {
			continue
		}
		if tag[i+2] == '-' {
			end = i
			break
		}
		i += 2
	}
	return tag[:start] + tag[end:], tag[start:end]
}

// Resolve is ECMA-402's ResolveLocale as far as the locale: the first
// requested locale the service is available in, as the available locale
// that matched with the request's Unicode extension, or the default when
// none is. The requested locales are canonical, as a Canonicalizer gives
// them; the service reads the extension's keywords itself.
func (m *LocaleMatcher) Resolve(requested []Locale, kind MatcherKind, def Locale) Locale {
	for _, r := range requested {
		base, extension := splitUnicodeExtension(r.String())
		if found := m.bestAvailable(base); found != "" {
			if l, err := ParseLocale(found + extension); err == nil {
				return l
			}
		}
	}
	return def
}

// Supported is ECMA-402's SupportedLocales: the requested locales the
// service is available in, cut short or not, in the order asked.
func (m *LocaleMatcher) Supported(requested []Locale, kind MatcherKind) []Locale {
	var out []Locale
	for _, r := range requested {
		base, _ := splitUnicodeExtension(r.String())
		if m.bestAvailable(base) != "" {
			out = append(out, r)
		}
	}
	return out
}
