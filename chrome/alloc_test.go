package chrome

import (
	"strings"
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// depthOf is how many overlays a Chrome's data is laid over the intl
// package's with: its own and the newer ones'.
func depthOf(t *testing.T, version string) int {
	t.Helper()
	p, err := ForVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(versions), "\n") {
		if f := strings.Fields(line); len(f) == 3 && f[0] == p.Version {
			for i, name := range chain() {
				if name == f[2] {
					return i + 1
				}
			}
		}
	}
	t.Fatalf("Chrome %s has no overlay in the chain", version)
	return 0
}

// Finding a profile reads versions.txt and the overlays' manifests where
// they lie: what it allocates is each layer's source, the pack and the
// overlay over it, and nothing for the hundreds of versions it reads past.
func TestForVersionAllocs(t *testing.T) {
	for _, v := range []string{"157", "150", "142"} {
		depth := depthOf(t, v)
		allocs := testing.AllocsPerRun(20, func() {
			if _, err := ForVersion(v); err != nil {
				t.Fatal(err)
			}
		})
		if want := float64(2 * depth); allocs != want {
			t.Errorf("Chrome %s, %d overlays deep: %v allocations, want %v", v, depth, allocs, want)
		}
	}
}

// A service built over a profile allocates what it does over the intl
// package's data, but for a little per overlay it reads through: a pool
// extended over its base's, at most two allocations a layer. A lookup that
// allocated, by file or by layer, would pass that at once.
func TestProfileServiceAllocs(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range profileServices {
		base := testing.AllocsPerRun(10, func() {
			if err := svc.build(intl.Embedded, loc); err != nil {
				t.Fatal(err)
			}
		})
		for _, v := range []string{"157", "142"} {
			p, err := ForVersion(v)
			if err != nil {
				t.Fatal(err)
			}
			depth := depthOf(t, v)
			allocs := testing.AllocsPerRun(10, func() {
				if err := svc.build(p.Source, loc); err != nil {
					t.Fatal(err)
				}
			})
			if limit := base + float64(2*depth); allocs > limit {
				t.Errorf("%s over Chrome %s, %d overlays deep: %v allocations, over the %v of the package's data and two a layer",
					svc.name, v, depth, allocs, base)
			}
		}
	}
}

// profileServices build each service once over a source.
var profileServices = []struct {
	name  string
	build func(intl.Source, intl.Locale) error
}{
	{"NumberFormat", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewNumberFormatFrom(s, l, intl.NumberFormatOptions{})
		return err
	}},
	{"DateTimeFormat", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewDateTimeFormatFrom(s, l, intl.DateTimeFormatOptions{
			DateStyle: intl.LengthFull, TimeStyle: intl.LengthFull, TimeZone: "America/New_York"})
		return err
	}},
	{"Collator", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewCollatorFrom(s, l, intl.CollatorOptions{})
		return err
	}},
	{"PluralRules", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewPluralRulesFrom(s, l, intl.PluralRulesOptions{})
		return err
	}},
	{"ListFormat", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewListFormatFrom(s, l, intl.ListFormatOptions{})
		return err
	}},
	{"RelativeTimeFormat", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewRelativeTimeFormatFrom(s, l, intl.RelativeTimeFormatOptions{})
		return err
	}},
	{"DisplayNames", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewDisplayNamesFrom(s, l, intl.DisplayNamesOptions{Kind: intl.DisplayRegion})
		return err
	}},
	{"DurationFormat", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewDurationFormatFrom(s, l, intl.DurationFormatOptions{})
		return err
	}},
	{"Segmenter", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewSegmenterFrom(s, l, intl.SegmenterOptions{Granularity: intl.GranularityWord})
		return err
	}},
	{"LocaleMatcher", func(s intl.Source, l intl.Locale) error {
		_, err := intl.NewLocaleMatcher(s, intl.ServiceNumberFormat)
		return err
	}},
}
