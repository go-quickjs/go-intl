package zonedata

import (
	"fmt"
	"sort"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// MetaVersion is the encoding version of the shared zone table.
const MetaVersion = 2

// Meta is what every locale shares about zones: which name is the canonical
// one, where each zone is, which metazone it was in when, and which zone
// stands for a metazone in each region. It comes from ICU's metaZones.txt,
// timezoneTypes.txt and zoneinfo64.txt.
type Meta struct {
	// Zones are the canonical zones, sorted by identifier.
	Zones []MetaZone
	// Aliases map every other identifier to its canonical one, sorted.
	Aliases []Alias
	// Golden are the reference zones of the metazones -- the zone whose
	// offset a metazone's name means, in a region or in the world ("001")
	// -- sorted by metazone and region.
	Golden []Golden
	// Primary is the zone that stands for a region with several, sorted by
	// region.
	Primary []Alias
}

// A MetaZone is one canonical zone.
type MetaZone struct {
	Zone string
	// Region is where the zone is, "001" for none.
	Region string
	// Uses are the metazones it has belonged to, in order.
	Uses []Use
}

// A Use is a stretch of time a zone belonged to a metazone. From and To are
// minutes since 1970 in UTC, plus one; zero means unbounded.
type Use struct {
	Metazone string
	From, To int
}

// An Alias maps one name to another: an identifier to its canonical zone, or
// a region to its primary zone.
type Alias struct {
	From, To string
}

// Golden is a metazone's reference zone in one region.
type Golden struct {
	Metazone, Region, Zone string
}

// Canonical returns the canonical form of a zone identifier, and whether it
// is one ICU knows.
func (m *Meta) Canonical(id string) (string, bool) {
	if _, ok := m.Zone(id); ok {
		return id, true
	}
	i := sort.Search(len(m.Aliases), func(i int) bool { return m.Aliases[i].From >= id })
	if i < len(m.Aliases) && m.Aliases[i].From == id {
		return m.Aliases[i].To, true
	}
	return "", false
}

// Zone finds a canonical zone.
func (m *Meta) Zone(id string) (*MetaZone, bool) {
	i := sort.Search(len(m.Zones), func(i int) bool { return m.Zones[i].Zone >= id })
	if i < len(m.Zones) && m.Zones[i].Zone == id {
		return &m.Zones[i], true
	}
	return nil, false
}

// Reference returns a metazone's reference zone in a region, or in the
// world where the region has none of its own.
func (m *Meta) Reference(metazone, region string) string {
	find := func(region string) (string, bool) {
		i := sort.Search(len(m.Golden), func(i int) bool {
			g := m.Golden[i]
			return g.Metazone > metazone || g.Metazone == metazone && g.Region >= region
		})
		if i < len(m.Golden) && m.Golden[i].Metazone == metazone && m.Golden[i].Region == region {
			return m.Golden[i].Zone, true
		}
		return "", false
	}
	if z, ok := find(region); ok {
		return z
	}
	z, _ := find("001")
	return z
}

// PrimaryZone returns the zone that stands for a region with several.
func (m *Meta) PrimaryZone(region string) string {
	i := sort.Search(len(m.Primary), func(i int) bool { return m.Primary[i].From >= region })
	if i < len(m.Primary) && m.Primary[i].From == region {
		return m.Primary[i].To
	}
	return ""
}

// EncodeMeta writes the shared zone table.
func EncodeMeta(m *Meta) []byte {
	b := blob.NewWriter(MetaVersion)
	b.Uint(len(m.Zones))
	for _, z := range m.Zones {
		b.String(z.Zone)
		b.String(z.Region)
		b.Uint(len(z.Uses))
		for _, u := range z.Uses {
			b.String(u.Metazone)
			b.Uint(u.From)
			b.Uint(u.To)
		}
	}
	for _, list := range [][]Alias{m.Aliases, m.Primary} {
		b.Uint(len(list))
		for _, a := range list {
			b.String(a.From)
			b.String(a.To)
		}
	}
	b.Uint(len(m.Golden))
	for _, g := range m.Golden {
		b.String(g.Metazone)
		b.String(g.Region)
		b.String(g.Zone)
	}
	return b.Bytes()
}

// DecodeMeta reads what EncodeMeta wrote.
func DecodeMeta(data []byte) (*Meta, error) {
	r, err := blob.NewReader(data, MetaVersion)
	if err != nil {
		return nil, err
	}
	count := func() (int, error) {
		n := r.Uint()
		if n < 0 || n > r.Left() {
			return 0, fmt.Errorf("zonedata: a count of %d with %d bytes left", n, r.Left())
		}
		return n, nil
	}
	var m Meta
	n, err := count()
	if err != nil {
		return nil, err
	}
	m.Zones = make([]MetaZone, n)
	for i := range m.Zones {
		z := &m.Zones[i]
		z.Zone, z.Region = r.String(), r.String()
		uses, err := count()
		if err != nil {
			return nil, err
		}
		z.Uses = make([]Use, uses)
		for j := range z.Uses {
			z.Uses[j] = Use{Metazone: r.String(), From: r.Uint(), To: r.Uint()}
		}
	}
	for _, list := range []*[]Alias{&m.Aliases, &m.Primary} {
		n, err := count()
		if err != nil {
			return nil, err
		}
		*list = make([]Alias, n)
		for i := range *list {
			(*list)[i] = Alias{From: r.String(), To: r.String()}
		}
	}
	if n, err = count(); err != nil {
		return nil, err
	}
	m.Golden = make([]Golden, n)
	for i := range m.Golden {
		m.Golden[i] = Golden{Metazone: r.String(), Region: r.String(), Zone: r.String()}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &m, nil
}
