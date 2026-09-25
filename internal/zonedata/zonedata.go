// Package zonedata is the model layer for time-zone names: what a locale calls
// a zone, and how it writes one it has no name for.
//
// A locale does not name zones one by one. It names *metazones* -- the Eastern
// Time that a dozen American zones share -- and CLDR keeps the mapping from a
// zone to the metazone it belongs to, which has changed over the years. A zone
// with no metazone, and a locale with no name for the one it has, is written
// by where it is -- "United Kingdom Time", "Los Angeles Time" -- or failing
// that by its offset from UTC.
//
// The names are ICU's, merged along ICU's fallback chain field by field, as
// ICU's TimeZoneNames reads them. The chain is not CLDR's: a locale ICU has no
// zone bundle for falls back by ICU's resource rules, and one field a locale
// leaves out is taken from its parent while the others stay its own.
package zonedata

import (
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 3

// A Names is what a locale calls one metazone or zone.
//
// Generic is the zone whatever the season -- "Eastern Time" -- while Standard
// and Daylight name the two halves of the year. Any of them may be empty.
type Names struct {
	LongGeneric   string
	LongStandard  string
	LongDaylight  string
	ShortGeneric  string
	ShortStandard string
	ShortDaylight string
}

// Empty reports whether the locale names this metazone at all.
func (n Names) Empty() bool {
	return n.LongGeneric == "" && n.LongStandard == "" && n.LongDaylight == "" &&
		n.ShortGeneric == "" && n.ShortStandard == "" && n.ShortDaylight == ""
}

// An Entry is one metazone and its names.
type Entry struct {
	Metazone string
	Names    Names
}

// Locale holds what one locale says about zones.
type Locale struct {
	// GMTFormat wraps an offset, "GMT{0}", and HourFormat writes the offset
	// itself, "+HH:mm;-HH:mm".
	GMTFormat  string
	HourFormat string
	// RegionFormat names a zone by where it is, "{0} Time", and
	// FallbackFormat qualifies a metazone's name by a place in it,
	// "{1} ({0})".
	RegionFormat   string
	FallbackFormat string

	// Metazones is sorted by name.
	Metazones []Entry
	// Zones are the zones the locale says something about itself -- a name
	// or the city it is written by -- sorted by zone, in CLDR's canonical
	// form ("Asia/Calcutta").
	Zones []ZoneEntry
	// Regions are the locale's names for the regions zones are in, sorted by
	// code. A region it has no name for is written as its code.
	Regions []RegionEntry
}

// A ZoneEntry is one named zone.
type ZoneEntry struct {
	Zone  string
	Names Names
	// City is the exemplar city, where the locale gives one; otherwise it
	// is made from the zone's identifier.
	City string
}

// A RegionEntry is what the locale calls one region.
type RegionEntry struct {
	Region string
	Name   string
}

// Metazone finds a metazone's names.
func (l *Locale) Metazone(name string) (Names, bool) {
	i := sort.Search(len(l.Metazones), func(i int) bool {
		return l.Metazones[i].Metazone >= name
	})
	if i < len(l.Metazones) && l.Metazones[i].Metazone == name {
		return l.Metazones[i].Names, true
	}
	return Names{}, false
}

// Zone finds what the locale says about one zone.
func (l *Locale) Zone(name string) (ZoneEntry, bool) {
	i := sort.Search(len(l.Zones), func(i int) bool { return l.Zones[i].Zone >= name })
	if i < len(l.Zones) && l.Zones[i].Zone == name {
		return l.Zones[i], true
	}
	return ZoneEntry{}, false
}

// Region finds the locale's name for a region.
func (l *Locale) Region(code string) (string, bool) {
	i := sort.Search(len(l.Regions), func(i int) bool { return l.Regions[i].Region >= code })
	if i < len(l.Regions) && l.Regions[i].Region == code {
		return l.Regions[i].Name, true
	}
	return "", false
}

// Encode writes a locale's zone names, with what it shares with other
// locales -- every string, each set of names and each list -- in pool,
// which the generator writes beside the locales and Decode is given.
func Encode(l *Locale, pool *blob.Pool) []byte {
	b := blob.NewPooledWriter(Version, pool)
	b.SharedString(l.GMTFormat)
	b.SharedString(l.HourFormat)
	b.SharedString(l.RegionFormat)
	b.SharedString(l.FallbackFormat)
	b.Shared(func(b *blob.Writer) {
		b.Uint(len(l.Metazones))
		for _, e := range l.Metazones {
			b.SharedString(e.Metazone)
			writeNames(b, e.Names)
		}
	})
	b.Shared(func(b *blob.Writer) {
		b.Uint(len(l.Zones))
		for _, e := range l.Zones {
			b.SharedString(e.Zone)
			writeNames(b, e.Names)
			b.SharedString(e.City)
		}
	})
	b.Shared(func(b *blob.Writer) {
		b.Uint(len(l.Regions))
		for _, e := range l.Regions {
			b.SharedString(e.Region)
			b.SharedString(e.Name)
		}
	})
	return b.Bytes()
}

func writeNames(b *blob.Writer, n Names) {
	b.Shared(func(b *blob.Writer) {
		for _, s := range [...]string{
			n.LongGeneric, n.LongStandard, n.LongDaylight,
			n.ShortGeneric, n.ShortStandard, n.ShortDaylight,
		} {
			b.SharedString(s)
		}
	})
}

func readNames(r *blob.Reader) Names {
	var n Names
	r.Shared(func(r *blob.Reader) {
		for _, p := range []*string{
			&n.LongGeneric, &n.LongStandard, &n.LongDaylight,
			&n.ShortGeneric, &n.ShortStandard, &n.ShortDaylight,
		} {
			*p = r.SharedString()
		}
	})
	return n
}

// count reads a count, which cannot exceed the bytes left to hold what it
// counts.
func count(r *blob.Reader) int {
	n := r.Uint()
	if n < 0 || n > r.Left() {
		return 0
	}
	return n
}

// Decode reads what Encode wrote, with the pool it wrote into.
func Decode(data []byte, pool blob.Shared) (*Locale, error) {
	r, err := blob.NewPooledReader(data, Version, pool)
	if err != nil {
		return nil, err
	}
	var l Locale
	l.GMTFormat = r.SharedString()
	l.HourFormat = r.SharedString()
	l.RegionFormat = r.SharedString()
	l.FallbackFormat = r.SharedString()
	r.Shared(func(r *blob.Reader) {
		l.Metazones = make([]Entry, count(r))
		for i := range l.Metazones {
			l.Metazones[i].Metazone = r.SharedString()
			l.Metazones[i].Names = readNames(r)
		}
	})
	r.Shared(func(r *blob.Reader) {
		l.Zones = make([]ZoneEntry, count(r))
		for i := range l.Zones {
			l.Zones[i].Zone = r.SharedString()
			l.Zones[i].Names = readNames(r)
			l.Zones[i].City = r.SharedString()
		}
	})
	r.Shared(func(r *blob.Reader) {
		l.Regions = make([]RegionEntry, count(r))
		for i := range l.Regions {
			l.Regions[i] = RegionEntry{Region: r.SharedString(), Name: r.SharedString()}
		}
	})
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
