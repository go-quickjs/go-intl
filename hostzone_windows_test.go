//go:build windows

package intl_test

import (
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// On Windows, where ICU does not read TZ, Node sets ICU's default zone from
// it when it starts, with createTimeZone: a name exactly as ICU spells it, a
// custom ID in any of ICU's forms, or else Etc/Unknown. The expectations are
// Node 26's, run with TZ set.
func TestHostTimeZoneFromTZ(t *testing.T) {
	for tz, want := range map[string]struct{ id, canonical string }{
		"Europe/Paris": {"Europe/Paris", "Europe/Paris"},
		"europe/paris": {"Etc/Unknown", "Etc/Unknown"},
		"GMT+05:30":    {"GMT+05:30", "+05:30"},
		"gmt-5":        {"GMT-05:00", "-05:00"},
		"GMT+0530":     {"GMT+05:30", "+05:30"},
		"GMT+5:30:15":  {"GMT+05:30:15", "+05:30:15"},
		"GMT+24":       {"Etc/Unknown", "Etc/Unknown"},
		"UTC":          {"UTC", "UTC"},
		"Etc/UTC":      {"Etc/UTC", "UTC"},
		"bogus":        {"Etc/Unknown", "Etc/Unknown"},
		"EST":          {"EST", "America/Panama"},
	} {
		t.Setenv("TZ", tz)
		z := intl.HostTimeZone(intl.Embedded)
		canonical, _ := z.Canonical()
		if z.ID() != want.id || canonical != want.canonical {
			t.Errorf("TZ=%s: %s, %s; want %s, %s", tz, z.ID(), canonical, want.id, want.canonical)
		}
	}
	t.Setenv("TZ", "GMT+5:30:15")
	if got := intl.HostTimeZone(intl.Embedded).Offset(0).Total(); got != 5*3600+30*60+15 {
		t.Errorf("GMT+5:30:15 is %d seconds east", got)
	}
}
