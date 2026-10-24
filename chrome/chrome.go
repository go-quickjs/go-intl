// Package chrome answers as a version of Chrome's Intl does: with the data
// its ICU carries, an overlay of the intl package's own (intl.NewOverlay),
// and the divergences its V8 answers in.
//
//	p, err := chrome.ForVersion("155.0.8059.84")
//	f, err := intl.NewDateTimeFormatFrom(p.Source, loc,
//		intl.DateTimeFormatOptions{Compat: p.Compat})
//
// Every stable Chrome for Windows from 140 is known, each by the ICU it
// builds with: Chromium's copy of ICU at the commit its DEPS pins, whose
// sources Chrome builds its data from through a data filter that leaves out
// much of what Node carries -- the names of most currencies and units, whole
// languages' data -- and patches in a little of its own. The data is made
// from those sources (internal/chromegen), and every profile is held to what
// a Chrome for Testing build of its ICU answers (testdata/chrome).
//
// A profile is Chrome running in English: ICU reads its default locale's
// data for a locale it has none for, and Chrome makes ICU's default its own
// locale, which for the recordings is en-US.
package chrome

import (
	"fmt"
	"strings"

	intl "github.com/go-quickjs/go-intl"
)

// A Profile is one Chrome's Intl.
type Profile struct {
	// Version is the Chrome version, "155.0.8059.84", and ICU the
	// chromium/deps/icu commit it builds with.
	Version, ICU string
	// Source is the data its ICU carries.
	Source intl.Source
	// Compat is the divergences its V8 answers in.
	Compat intl.Compat
}

// ForVersion is the profile of a Chrome version: a full one,
// "155.0.8059.84", or the start of one, "155" or "155.0.8059", which names
// the newest version known that it starts.
func ForVersion(version string) (Profile, error) {
	var found []string
	for _, line := range strings.Split(strings.TrimSpace(versions), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			return Profile{}, fmt.Errorf("chrome: versions.txt: %q", line)
		}
		if (f[0] == version || strings.HasPrefix(f[0], version+".")) && (found == nil || newer(f[0], found[0])) {
			found = f
		}
	}
	if found == nil {
		return Profile{}, fmt.Errorf("chrome: no Chrome %s is known", version)
	}
	pack, ok := packOf(found[2])
	if !ok {
		return Profile{}, fmt.Errorf("chrome: the data of Chrome %s, %s, is missing", found[0], found[2])
	}
	overlay, err := intl.NewPack(pack)
	if err != nil {
		return Profile{}, fmt.Errorf("chrome: %s: %w", found[2], err)
	}
	src, err := intl.NewOverlay(intl.Embedded, overlay)
	if err != nil {
		return Profile{}, fmt.Errorf("chrome: %s: %w", found[2], err)
	}
	major, err := majorOf(found[0])
	if err != nil {
		return Profile{}, err
	}
	return Profile{Version: found[0], ICU: found[1], Source: src, Compat: compatOf(major, found[1])}, nil
}

// newer reports whether version a comes after b.
func newer(a, b string) bool {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(x) && i < len(y); i++ {
		var m, n int
		fmt.Sscan(x[i], &m)
		fmt.Sscan(y[i], &n)
		if m != n {
			return m > n
		}
	}
	return len(x) > len(y)
}

func majorOf(version string) (int, error) {
	var major int
	if _, err := fmt.Sscanf(version, "%d.", &major); err != nil {
		return 0, fmt.Errorf("chrome: the version %q", version)
	}
	return major, nil
}

// standardFrom are the divergences of Node's that Chrome answers as the
// standard does, and the first Chrome that does; 0 is every Chrome. Each was
// found by asking a Chrome for Testing build of each ICU pin what Node and
// the standard disagree on (testdata/chrome).
var standardFrom = []struct {
	flag  intl.Compat
	first int
}{
	// Chrome's data filter keeps the names of the currencies it lists.
	{intl.CurrencyNames, 0},
	{intl.PlainValueZone, 148},
	// Chrome's temporal_rs takes the standard's fixes Node's predates.
	{intl.RoundingWindow, 150},
	{intl.RepeatedMidnight, 150},
	{intl.SubdivisionHourCycles, 155},
}

// browserFrom are Chrome's divergences that Node does not share, and the
// first Chrome that answers so.
var browserFrom = []struct {
	flag  intl.Compat
	first int
}{
	{intl.ThinSpace, 148},
}

// noISOEraRules are the ICU pins whose data gives the ISO 8601 calendar no
// era rules, as ICU 78's does; Chromium patches them in from 3859e64eed.
// Over such data, the V8 of Chrome 148 on writes no ISO month names.
var noISOEraRules = map[string]bool{
	"ff7995a708a10ab44db101358083c7f74752da9f": true,
}

// compatOf is the divergences a Chrome answers in, by its version and its
// ICU pin: Node's, but for those it answers as the standard does, and its
// own.
func compatOf(major int, pin string) intl.Compat {
	c := intl.NodeICU
	// Chrome 142's V8 summed a duration's fractions of a second exactly.
	if major < 146 {
		c &^= intl.DurationOverflow
	}
	for _, s := range standardFrom {
		if major >= s.first {
			c &^= s.flag
		}
	}
	for _, b := range browserFrom {
		if major >= b.first {
			c |= b.flag
		}
	}
	if major >= 148 && noISOEraRules[pin] {
		c |= intl.ISOMonthNames
	}
	// Chrome 148 is the first over ICU 78.
	if major < 148 {
		c |= intl.AmeteMihretExtendedYear
	}
	return c
}
