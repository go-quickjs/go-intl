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
const Version = 2

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

// Encode writes a locale's zone names.
func Encode(l *Locale) []byte {
	b := blob.NewWriter(Version)
	b.String(l.GMTFormat)
	b.String(l.HourFormat)
	b.String(l.RegionFormat)
	b.String(l.FallbackFormat)
	b.Uint(len(l.Metazones))
	for _, e := range l.Metazones {
		b.String(e.Metazone)
		writeNames(b, e.Names)
	}
	b.Uint(len(l.Zones))
	for _, e := range l.Zones {
		b.String(e.Zone)
		writeNames(b, e.Names)
		b.String(e.City)
	}
	b.Uint(len(l.Regions))
	for _, e := range l.Regions {
		b.String(e.Region)
		b.String(e.Name)
	}
	return b.Bytes()
}

func writeNames(b *blob.Writer, n Names) {
	for _, s := range [...]string{
		n.LongGeneric, n.LongStandard, n.LongDaylight,
		n.ShortGeneric, n.ShortStandard, n.ShortDaylight,
	} {
		b.String(s)
	}
}

func readNames(r *blob.Reader) Names {
	var n Names
	for _, p := range []*string{
		&n.LongGeneric, &n.LongStandard, &n.LongDaylight,
		&n.ShortGeneric, &n.ShortStandard, &n.ShortDaylight,
	} {
		*p = r.String()
	}
	return n
}

// Decode reads what Encode wrote.
func Decode(data []byte) (*Locale, error) {
	r, err := blob.NewReader(data, Version)
	if err != nil {
		return nil, err
	}
	var l Locale
	l.GMTFormat = r.String()
	l.HourFormat = r.String()
	l.RegionFormat = r.String()
	l.FallbackFormat = r.String()

	if n := r.Uint(); n >= 0 && n <= r.Left() {
		l.Metazones = make([]Entry, 0, n)
		for i := 0; i < n; i++ {
			name := r.String()
			l.Metazones = append(l.Metazones, Entry{Metazone: name, Names: readNames(r)})
		}
	}
	if n := r.Uint(); n >= 0 && n <= r.Left() {
		l.Zones = make([]ZoneEntry, 0, n)
		for i := 0; i < n; i++ {
			e := ZoneEntry{Zone: r.String()}
			e.Names = readNames(r)
			e.City = r.String()
			l.Zones = append(l.Zones, e)
		}
	}
	if n := r.Uint(); n >= 0 && n <= r.Left() {
		l.Regions = make([]RegionEntry, 0, n)
		for i := 0; i < n; i++ {
			e := RegionEntry{Region: r.String()}
			e.Name = r.String()
			l.Regions = append(l.Regions, e)
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
