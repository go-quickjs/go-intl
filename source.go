package intl

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
)

// Where the data comes from.
//
// This interface is the whole reusability story. Every table this library
// reads is asked for through it, so a caller can supply their own, load from a
// directory instead of the binary, or carry only the locales they need. The
// package it replaces welded its tables in with go:embed at package scope,
// which is why importing it cost fourteen megabytes whether or not anything
// was formatted.
//
// A Source hands back bytes, not decoded values. Decoding is the model
// layer's business, and keeping it out of here means a source can be a
// directory, an archive or a network cache without knowing what any of it
// means.

// A Marker names one data set.
type Marker string

const (
	// MarkerLikelySubtags is CLDR's likely-subtag table: what an identifier
	// that leaves out a script or a region most probably meant.
	MarkerLikelySubtags Marker = "likelysubtags"
	// MarkerParentLocales is CLDR's parent-locale table, which redirects a
	// fallback that truncation would send somewhere wrong.
	MarkerParentLocales Marker = "parentlocales"
	// MarkerNumbers is one locale's number symbols and patterns.
	MarkerNumbers Marker = "numbers"
	// MarkerCurrencyDigits is how many decimals each currency is written
	// with, which does not vary by locale.
	MarkerCurrencyDigits Marker = "currencydigits"
	// MarkerPlurals is one language's plural rules.
	MarkerPlurals Marker = "plurals"
	// MarkerLists is one locale's list-joining patterns.
	MarkerLists Marker = "lists"
	// MarkerUnits is one locale's measurement patterns.
	MarkerUnits Marker = "units"
	// MarkerRelativeTime is one locale's relative-time wordings.
	MarkerRelativeTime Marker = "reltime"
	// MarkerNames is one locale's display names. It is much the largest of
	// these, and a program that never asks for one never reads it.
	MarkerNames Marker = "names"
	// MarkerDates is one locale's calendar names and date patterns.
	MarkerDates Marker = "dates"
	// MarkerZoneNames is what one locale calls the time zones.
	MarkerZoneNames Marker = "zonenames"
	// MarkerMetazones maps a zone to the metazone it belongs to, which is not
	// a matter of language and so is one table for every locale.
	MarkerMetazones Marker = "metazones"
	// MarkerCalendarPrefs maps a region to the calendar it reckons in, which
	// is likewise not a matter of language.
	MarkerCalendarPrefs Marker = "calendarprefs"
	// MarkerTimeData is CLDR's hour-cycle preferences, by region or by
	// language and region.
	MarkerTimeData Marker = "timedata"
	// MarkerJapaneseEras is the first day of each era of the Japanese
	// calendar.
	MarkerJapaneseEras Marker = "japaneseeras"
	// MarkerAvailable is the locales each service is available in.
	MarkerAvailable Marker = "available"
	// MarkerAliases is CLDR's aliases for locale subtags and extension types,
	// as ICU holds them.
	MarkerAliases Marker = "aliases"
	// MarkerUmmAlQura is the Umm al-Qura calendar's table of month lengths.
	MarkerUmmAlQura Marker = "ummalqura"
	// MarkerRBNF is ICU's rules for its algorithmic numbering systems, the
	// Roman and Hebrew numerals among them.
	MarkerRBNF Marker = "rbnf"
	// MarkerWeekData is CLDR's week conventions by region: the first day of
	// the week, the days the first week of a year needs, the weekend.
	MarkerWeekData Marker = "weekdata"
	// MarkerNormalization is how characters come apart and go back together,
	// which is Unicode's rather than any locale's.
	MarkerNormalization Marker = "normalization"
	// MarkerNumberingSystems is CLDR's numeric numbering systems, with their
	// digits and what the root says about writing numbers in each.
	MarkerNumberingSystems Marker = "numberingsystems"
	// MarkerCollation is the collations one locale defines: what it changes
	// about the root order, and what it sorts by default.
	MarkerCollation Marker = "collation"
	// MarkerCollationRoot is the root collation table, which every
	// collation falls back to.
	MarkerCollationRoot Marker = "collationroot"
	// MarkerCollationTree is how collation locales inherit from one
	// another, which differs from the ordinary fallback.
	MarkerCollationTree Marker = "collationtree"
)

// ErrNotFound reports that a source has no data under a marker and locale. It
// is not a failure on its own: a lookup walks a fallback chain and expects to
// be told no on the way.
var ErrNotFound = errors.New("no data")

// A Source supplies the bytes of one data set.
//
// Data that is not kept per locale -- the likely subtags, which are one table
// for everything -- is asked for at the root, and a source that has it answers
// there.
type Source interface {
	Open(m Marker, d DataLocale) ([]byte, error)
}

//go:embed data
var embeddedData embed.FS

// Embedded is the data built into this package. It is the default, so that the
// simple path needs no setting up, and it is only a default: anything taking a
// Source can be given another.
var Embedded Source = mustSub(embeddedData, "data")

func mustSub(fsys fs.FS, dir string) Source {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("intl: the embedded data is not where it should be: " + err.Error())
	}
	return NewFS(sub)
}

// NewFS returns a Source that reads from a file system, which is how both the
// embedded data and a directory on disk are served -- the same reader over a
// different backing.
//
// A data set kept for one locale is the file <marker>/<locale>.bin, and one
// kept for everything is <marker>.bin.
func NewFS(fsys fs.FS) Source { return fsSource{fsys} }

type fsSource struct{ fsys fs.FS }

func (s fsSource) Open(m Marker, d DataLocale) ([]byte, error) {
	name := string(m) + ".bin"
	if !d.IsRoot() {
		name = path.Join(string(m), d.String()+".bin")
	}
	b, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s for %s", ErrNotFound, m, d)
		}
		return nil, err
	}
	return b, nil
}

// pairTable is a generated table of data locales mapped to data locales, laid
// out as records of two of them, sorted by the first.
//
// It is read where it lies. Nothing is decoded until something is looked up,
// and a lookup is a binary search over the bytes, so a table costs what it
// takes up and no more.
type pairTable []byte

const pairSize = 2 * DataLocaleSize

func newPairTable(b []byte) (pairTable, error) {
	if len(b)%pairSize != 0 {
		return nil, fmt.Errorf("a table of %d bytes does not divide into %d-byte records",
			len(b), pairSize)
	}
	return pairTable(b), nil
}

func (t pairTable) len() int { return len(t) / pairSize }

// at returns the key and value of one record.
func (t pairTable) at(i int) (key, value DataLocale) {
	rec := t[i*pairSize : (i+1)*pairSize]
	_ = key.UnmarshalBinary(rec[:DataLocaleSize])
	_ = value.UnmarshalBinary(rec[DataLocaleSize:])
	return key, value
}

// lookup finds what a data locale maps to.
func (t pairTable) lookup(want DataLocale) (DataLocale, bool) {
	key, _ := want.MarshalBinary()
	lo, hi := 0, t.len()
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch compareBytes(t[mid*pairSize:mid*pairSize+DataLocaleSize], key) {
		case 0:
			_, v := t.at(mid)
			return v, true
		case -1:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return DataLocale{}, false
}

func compareBytes(a, b []byte) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}
