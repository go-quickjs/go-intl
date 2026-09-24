package intl

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-quickjs/go-intl/internal/zonedata"
)

// What a locale calls a time zone.
//
// A locale does not name zones one by one. It names metazones -- the Eastern
// Time that a dozen American zones share -- and a zone is written with the
// name of the metazone it belongs to. Which of that metazone's names is used
// depends on the instant: Eastern Standard Time in winter and Eastern Daylight
// Time in summer.
//
// A zone with no metazone, and a locale with no name for the one it has, falls
// back to the offset from UTC, which is what ICU does and is why every zone
// can be written even when nothing has a name for it.

// zoneNames is a locale's zone data together with the mapping every locale
// shares.
type zoneNames struct {
	locale     *zonedata.Locale
	metazones  metazoneTable
	gmtFormat  string
	hourFormat string
	gmtZero    string
}

// metazoneTable maps a zone to the metazone it belongs to now. It is one table
// for every locale, since which metazone a zone is in is not a matter of
// language.
type metazoneTable string

func (t metazoneTable) of(zone string) (string, bool) {
	for line := range strings.SplitSeq(strings.TrimRight(string(t), "\n"), "\n") {
		name, metazone, ok := strings.Cut(line, " ")
		if ok && name == zone {
			return metazone, true
		}
	}
	return "", false
}

func loadZoneNames(src Source, loc Locale) (*zoneNames, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.Chain(loc.Data())
	}
	var data *zonedata.Locale
	for _, d := range chain {
		b, err := src.Open(MarkerZoneNames, d)
		if err != nil {
			continue
		}
		data, err = zonedata.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the zone names for %s: %w", d, err)
		}
		break
	}
	if data == nil {
		// Without names every zone is written as its offset, which is a worse
		// answer than the locale's own but not a wrong one.
		data = &zonedata.Locale{}
	}

	out := &zoneNames{locale: data}
	if b, err := src.Open(MarkerMetazones, DataLocale{}); err == nil {
		out.metazones = metazoneTable(b)
	}
	out.gmtFormat, out.hourFormat, out.gmtZero = data.GMTFormat, data.HourFormat, data.GMTZero
	if out.gmtFormat == "" {
		out.gmtFormat = "GMT{0}"
	}
	if out.hourFormat == "" {
		out.hourFormat = "+HH:mm;-HH:mm"
	}
	return out, nil
}

// namesForZone finds the names for the formatter's zone, once, when it is
// built. The mapping is a table scan and there is no reason to do it again for
// every instant.
func (z *zoneNames) namesForZone(zone string) (zonedata.Names, bool) {
	var names zonedata.Names
	found := false
	if metazone, ok := z.metazones.of(zone); ok {
		names, found = z.locale.Metazone(metazone)
	}
	// A zone named in its own right overrides the metazone field by field
	// rather than wholesale. London carries only "British Summer Time" and
	// takes "Greenwich Mean Time" from the GMT metazone it belongs to; reading
	// its entry as the whole answer leaves it nameless all winter.
	if own, ok := z.locale.Zone(zone); ok {
		found = true
		for _, pair := range [][2]*string{
			{&names.LongGeneric, &own.LongGeneric},
			{&names.LongStandard, &own.LongStandard},
			{&names.LongDaylight, &own.LongDaylight},
			{&names.ShortGeneric, &own.ShortGeneric},
			{&names.ShortStandard, &own.ShortStandard},
			{&names.ShortDaylight, &own.ShortDaylight},
		} {
			if *pair[1] != "" {
				*pair[0] = *pair[1]
			}
		}
	}
	return names, found && !names.Empty()
}

// zoneNamesFor returns the short and long names for an instant in the
// formatter's zone, either of which may be empty.
func (f *DateTimeFormat) zoneNamesFor(local time.Time, abbr string, offset int) (short, long string) {
	if f.zones == nil || !f.zoneKnown {
		return "", ""
	}
	names := f.zoneNames

	// Which half of the year decides which name: a zone in summer time is
	// called something else from the same zone in winter.
	daylight := local.IsDST()
	switch {
	case daylight && names.LongDaylight != "":
		long = names.LongDaylight
	case !daylight && names.LongStandard != "":
		long = names.LongStandard
	default:
		long = names.LongGeneric
	}
	switch {
	case daylight && names.ShortDaylight != "":
		short = names.ShortDaylight
	case !daylight && names.ShortStandard != "":
		short = names.ShortStandard
	default:
		short = names.ShortGeneric
	}
	return short, long
}
