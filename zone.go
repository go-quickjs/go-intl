package intl

import (
	"fmt"
	"strings"
	"time"
)

// A TimeZone is a time zone as ICU reckons it, which is what Node reckons
// with: one ICU knows by name, from ICU's zoneinfo64, or an offset from UTC.
// It is immutable and safe for concurrent use.
//
// Instants and local times are milliseconds from 1970, a local time being
// the wall clock read as if it were UTC, as ECMA-262 and ICU count them.
// Offsets are in seconds east of UTC.
type TimeZone struct {
	z  *timeZone
	id string
	// canonical is the name resolvedOptions reports; empty where ICU has
	// none, as for a host zone ICU does not know.
	canonical string
}

// LoadTimeZone finds a zone by a name ICU knows, in any case, or by an
// offset from UTC as ECMA-402 writes one: "+05:30", "+0530", "-08". It is
// an error for anything else, and for Etc/Unknown.
func LoadTimeZone(src Source, name string) (*TimeZone, error) {
	if seconds, resolved, ok := parseOffsetZone(name); ok {
		return &TimeZone{z: fixedZone(seconds), id: resolved, canonical: resolved}, nil
	}
	z, err := loadTimeZone(src, name)
	if err != nil {
		return nil, err
	}
	return &TimeZone{z: z, id: z.name, canonical: z.resolvedID()}, nil
}

// ID is the zone's name as ICU spells it, "Asia/Kolkata" for
// "asia/kolkata", or its offset, "+05:30".
func (z *TimeZone) ID() string { return z.id }

// Canonical is the name Intl.DateTimeFormat reports for the zone: ICU's
// canonical name, "Asia/Calcutta" for "Asia/Kolkata", "UTC" for Etc/UTC and
// Etc/GMT, or the offset. It is false for a host zone ICU does not know.
func (z *TimeZone) Canonical() (string, bool) { return z.canonical, z.canonical != "" }

// A ZoneOffset is the offset in force at an instant: the zone's raw offset
// and its daylight saving, apart, in seconds.
type ZoneOffset struct {
	Raw, DST int
}

// Total is the offset from UTC, in seconds.
func (o ZoneOffset) Total() int { return o.Raw + o.DST }

func exportOffset(o zoneOffset) ZoneOffset { return ZoneOffset{Raw: o.raw, DST: o.dst} }

// Offset is the offset in force at an instant.
func (z *TimeZone) Offset(ms int64) ZoneOffset { return exportOffset(z.z.offsetAt(ms)) }

// A LocalRule says which offset reads a local time that a transition skips
// or repeats.
type LocalRule int

const (
	// Former is the offset before the transition.
	Former LocalRule = tzLocalFormer
	// Latter is the offset after it.
	Latter LocalRule = tzLocalLatter
)

// OffsetFromLocal is the offset a local time is read with, as ICU's
// getOffsetFromLocal gives it: one a transition skips is read by the
// skipped rule, one it repeats by the repeated rule. ECMA-262's UTC(t) is
// OffsetFromLocal(t, Former, Former), as V8 reckons it.
func (z *TimeZone) OffsetFromLocal(local int64, skipped, repeated LocalRule) ZoneOffset {
	return exportOffset(z.z.offsetFromLocal(local, int(skipped), int(repeated)))
}

// A ZoneTransition is a change in a zone's offset: the instant it takes
// effect, and the offsets before and after it. As ICU counts them, a change
// of the daylight saving alone is one, a change of the total offset or not.
type ZoneTransition struct {
	At            int64
	Before, After ZoneOffset
}

func exportTransition(t zoneTransition) ZoneTransition {
	return ZoneTransition{At: t.at, Before: exportOffset(t.from), After: exportOffset(t.to)}
}

// NextTransition is the first transition after an instant, as ICU's
// getNextTransition finds it; false when there is none.
func (z *TimeZone) NextTransition(ms int64) (ZoneTransition, bool) {
	t, ok := z.z.nextTransition(ms, false)
	return exportTransition(t), ok
}

// PreviousTransition is the last transition before an instant, as ICU's
// getPreviousTransition finds it; false when there is none.
func (z *TimeZone) PreviousTransition(ms int64) (ZoneTransition, bool) {
	t, ok := z.z.previousTransition(ms, false)
	return exportTransition(t), ok
}

// HostTimeZone is the zone the machine is set to, as ICU's
// TimeZone::detectHostTimeZone finds it, which is Node's default zone.
//
// On Windows it is the zone Windows is set to, named by CLDR's mapping for
// the user's region, or Etc/GMT at the offset where daylight saving is
// turned off. Elsewhere it is TZ where that names a zone, then the zone
// /etc/localtime links to, then the zone file it is a copy of. A name ICU
// does not know is a zone of the host's standard offset, and no name at all
// is Etc/Unknown, which is UTC.
func HostTimeZone(src Source) *TimeZone {
	id, raw := hostZone(src)
	return detectHostZone(src, id, raw, time.Now().UnixMilli())
}

// detectHostZone is the rest of detectHostTimeZone: the zone the host's
// name and raw offset, in seconds east, make at an instant.
func detectHostZone(src Source, id string, raw int, now int64) *TimeZone {
	if id == "" {
		unknown := fixedZone(0)
		unknown.name, unknown.id = "Etc/Unknown", "Etc/Unknown"
		return &TimeZone{z: unknown, id: "Etc/Unknown", canonical: "Etc/Unknown"}
	}
	// createSystemTimeZone takes a name only as ICU spells it.
	z, err := loadTimeZone(src, id)
	if err == nil && z.name != id {
		z = nil
	}
	// A name of three or four letters at another offset is taken to be an
	// ambiguous abbreviation.
	if z != nil && (len(id) == 3 || len(id) == 4) && z.offsetAt(now).raw != raw {
		z = nil
	}
	if z == nil {
		custom := fixedZone(raw)
		custom.name = id
		return &TimeZone{z: custom, id: id}
	}
	return &TimeZone{z: z, id: z.name, canonical: z.resolvedID()}
}

// DefaultTimeZone is the zone Intl.DateTimeFormat takes when it is given
// none, as V8's Intl::DefaultTimeZone names it: the host's canonical name,
// or "UTC" where ICU has none.
func DefaultTimeZone(src Source) string {
	if name, ok := HostTimeZone(src).Canonical(); ok {
		return name
	}
	return "UTC"
}

// windowsZone is ICU's mapping of a Windows zone to one ICU knows: the
// first zone CLDR lists for the region, else for 001.
func windowsZone(src Source, windows, region string) (string, error) {
	b, err := src.Open(MarkerWindowsZones, DataLocale{})
	if err != nil {
		return "", err
	}
	world := ""
	for _, line := range strings.Split(string(b), "\n") {
		name, rest, ok := strings.Cut(line, "\t")
		if !ok || name != windows {
			continue
		}
		r, zones, _ := strings.Cut(rest, "\t")
		first, _, _ := strings.Cut(zones, " ")
		switch r {
		case region:
			if region != "" {
				return first, nil
			}
		case "001":
			world = first
		}
	}
	if world == "" {
		return "", fmt.Errorf("intl: %q is not a Windows time zone: %w", windows, errNoZone)
	}
	return world, nil
}
