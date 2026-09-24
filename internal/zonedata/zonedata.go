// Package zonedata is the model layer for time-zone names: what a locale calls
// a zone, and how it writes one it has no name for.
//
// A locale does not name zones one by one. It names *metazones* -- the Eastern
// Time that a dozen American zones share -- and CLDR keeps the mapping from a
// zone to the metazone it belongs to, which has changed over the years. A zone
// with no metazone, and a locale with no name for the one it has, falls back
// to writing the offset from UTC.
package zonedata

import (
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// A Names is what a locale calls one metazone.
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
	// itself, "+HH:mm;-HH:mm". GMTZero is what is written at UTC, "GMT".
	GMTFormat  string
	HourFormat string
	GMTZero    string

	// Metazones is sorted by name.
	Metazones []Entry
	// Zones are the few zones a locale names itself rather than through a
	// metazone, sorted by zone.
	Zones []ZoneEntry
}

// A ZoneEntry is one named zone.
type ZoneEntry struct {
	Zone  string
	Names Names
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

// Zone finds a zone the locale names itself.
func (l *Locale) Zone(name string) (Names, bool) {
	i := sort.Search(len(l.Zones), func(i int) bool { return l.Zones[i].Zone >= name })
	if i < len(l.Zones) && l.Zones[i].Zone == name {
		return l.Zones[i].Names, true
	}
	return Names{}, false
}

// Encode writes a locale's zone names.
func Encode(l *Locale) []byte {
	b := blob.NewWriter(Version)
	b.String(l.GMTFormat)
	b.String(l.HourFormat)
	b.String(l.GMTZero)
	b.Uint(len(l.Metazones))
	for _, e := range l.Metazones {
		b.String(e.Metazone)
		writeNames(b, e.Names)
	}
	b.Uint(len(l.Zones))
	for _, e := range l.Zones {
		b.String(e.Zone)
		writeNames(b, e.Names)
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
	l.GMTZero = r.String()

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
			name := r.String()
			l.Zones = append(l.Zones, ZoneEntry{Zone: name, Names: readNames(r)})
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
