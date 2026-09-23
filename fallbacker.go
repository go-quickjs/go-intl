package intl

import "fmt"

// The fallback chain, once CLDR has a say in it.
//
// DataLocale.Fallback walks the identifier alone, which is right for the
// common shapes and wrong where CLDR says so. A Fallbacker carries the two
// tables that say so:
//
//   - The parent locales redirect a chain that truncation would send somewhere
//     wrong. "zh-Hant" truncates to "zh", but traditional Chinese inheriting
//     from simplified is worse than inheriting from the root, so CLDR gives it
//     the root as its parent instead. "en-AU" goes to "en-001" rather than
//     straight to "en", so it keeps the spellings the world outside America
//     shares.
//   - The likely subtags fill in what an identifier left out, so that "zh-TW"
//     and "zh-Hant-TW" are looked for in one place rather than two.
//
// The two compose: maximize first if the identifier is partial, then chain.

// A Fallbacker answers where to look for data. It is read-only once built and
// safe for concurrent use.
type Fallbacker struct {
	parents pairTable
	likely  pairTable
}

// NewFallbacker reads the tables it needs from a source.
func NewFallbacker(src Source) (*Fallbacker, error) {
	parents, err := loadPairs(src, MarkerParentLocales)
	if err != nil {
		return nil, err
	}
	likely, err := loadPairs(src, MarkerLikelySubtags)
	if err != nil {
		return nil, err
	}
	return &Fallbacker{parents: parents, likely: likely}, nil
}

func loadPairs(src Source, m Marker) (pairTable, error) {
	b, err := src.Open(m, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", m, err)
	}
	t, err := newPairTable(b)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", m, err)
	}
	return t, nil
}

// maxChain is a stop for a parent table that pointed in a circle. CLDR's does
// not, but a table is data and data can be wrong, and looping forever is a
// worse way to find out than returning a chain that is merely too short.
const maxChain = 16

// Chain returns the data locales to try, most specific first and the root
// last, following CLDR's parents where it has one and truncating where it does
// not.
func (f *Fallbacker) Chain(d DataLocale) []DataLocale {
	chain := []DataLocale{d}
	seen := map[DataLocale]bool{d: true}
	for cur := d; !cur.IsRoot() && len(chain) < maxChain; {
		next, ok := f.parents.lookup(cur)
		if !ok {
			next = truncate(cur)
		}
		if seen[next] {
			break
		}
		seen[next] = true
		chain = append(chain, next)
		cur = next
	}
	// A parent table could in principle stop somewhere other than the root.
	// The root always has an answer, so the chain always reaches it.
	if last := chain[len(chain)-1]; !last.IsRoot() {
		chain = append(chain, DataLocale{})
	}
	return chain
}

// truncate drops the most specific part that is still there.
func truncate(d DataLocale) DataLocale {
	switch {
	case !d.Region.IsZero():
		d.Region = Region{}
	case !d.Script.IsZero():
		d.Script = Script{}
	default:
		d.Language = Und
	}
	return d
}

// Maximize fills in what an identifier left out, by UTS #35's likely-subtags
// lookup: the most specific key first, then progressively less of it. What the
// identifier did say is kept, so maximizing never overrules the caller.
func (f *Fallbacker) Maximize(d DataLocale) (DataLocale, bool) {
	keys := [...]DataLocale{
		d,
		{Language: d.Language, Region: d.Region},
		{Language: d.Language, Script: d.Script},
		{Language: d.Language},
		{},
	}
	for _, key := range keys {
		got, ok := f.likely.lookup(key)
		if !ok {
			continue
		}
		if d.Language != Und {
			got.Language = d.Language
		}
		if !d.Script.IsZero() {
			got.Script = d.Script
		}
		if !d.Region.IsZero() {
			got.Region = d.Region
		}
		return got, true
	}
	return d, false
}

// Minimize removes what maximizing would put back, which is how a locale is
// written in its shortest unambiguous form: "zh-Hans-CN" is just "zh".
func (f *Fallbacker) Minimize(d DataLocale) (DataLocale, bool) {
	full, ok := f.Maximize(d)
	if !ok {
		return d, false
	}
	// The shortest form whose maximization is the same locale wins.
	for _, try := range [...]DataLocale{
		{Language: d.Language},
		{Language: d.Language, Region: d.Region},
		{Language: d.Language, Script: d.Script},
	} {
		if got, ok := f.Maximize(try); ok && got == full {
			return try, true
		}
	}
	return d, true
}
