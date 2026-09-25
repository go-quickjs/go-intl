package intl

import "strings"

// Which behavior to reproduce where Node and the standard disagree.
//
// Node, and so ICU4C, differs from ECMA-402 as written in a handful of
// observable places. A host that cares about conformance wants the standard; a
// host that cares about matching Node wants Node. Both are legitimate, so the
// choice is made when a formatter is built, and it is not ambient state: two
// formatters in one program may answer differently.
//
// The choice is made divergence by divergence, because a host may want Node
// in most places and the standard in a few: go-quickjs answers as Node does
// but for the Japanese twelve-hour clock, the Islamic eras, Temporal's
// formats and the keyword value "yes". Standard and NodeICU are the two ends.
//
// The rule that keeps this honest is that every divergence is a named,
// documented entry with a test on both sides. It is a short list, not a
// licence to be bug-compatible with all of ICU.
//
// If the list ever passes about twenty entries, that is a sign the standard
// behavior is wrong somewhere and this is hiding it, rather than a sign that
// Node has drifted further.

// Compat is the divergences in which to answer as Node does rather than as
// the standard: a set of the flags below. The zero value is the standard in
// every one.
type Compat uint32

const (
	// NarrowSpace writes a plain space where ICU writes U+202F.
	NarrowSpace Compat = 1 << iota
	// TwoLetterTags leaves a tag of two lowercase letters as it came.
	TwoLetterTags
	// CollationKeyword gives a collation chosen by option to the resolved
	// locale.
	CollationKeyword
	// DurationOverflow sums a duration's fraction of a second as V8 does,
	// in an int64.
	DurationOverflow
	// TwelveHourCycle takes V8's twelve-hour clock for hour12: h11 in
	// Japan's region and h12 everywhere else.
	TwelveHourCycle
	// IslamicEras writes an Islamic year before the Hijrah as ICU4C does,
	// a negative year of the era after it.
	IslamicEras
	// TemporalFormats writes a Temporal value with V8's formats, which
	// count an era as a field asked for and keep no hour cycle.
	TemporalFormats
	// YesValues leaves "yes" out of any Unicode extension keyword, as ICU
	// does, rather than only where it stands for "true".
	YesValues
)

const (
	// Standard is ECMA-402 as written, in every divergence.
	Standard Compat = 0
	// NodeICU reproduces Node's and ICU4C's observable behavior in every
	// divergence.
	NodeICU = NarrowSpace | TwoLetterTags | CollationKeyword | DurationOverflow |
		TwelveHourCycle | IslamicEras | TemporalFormats | YesValues
)

// Has reports whether Node's behavior is chosen for a divergence.
func (c Compat) Has(d Compat) bool { return c&d != 0 }

func (c Compat) String() string {
	switch c {
	case Standard:
		return "Standard"
	case NodeICU:
		return "NodeICU"
	}
	var names []string
	for _, d := range Divergences {
		if c.Has(d.Flag) {
			names = append(names, d.Name)
		}
	}
	return strings.Join(names, "|")
}

// Divergences are the places the two profiles differ, listed so that the set
// can be read rather than discovered, with the flag that chooses Node's side
// of each.
var Divergences = []Divergence{
	{
		Name: "NarrowSpace", Flag: NarrowSpace,
		Area:     "DateTimeFormat",
		What:     "the space before a day period, and every other narrow no-break space",
		Standard: "CLDR's character, U+202F, as ICU writes it: \"3:04\u202fPM\"",
		Node: "a plain space: V8 replaces U+202F in everything it formats, " +
			"reverting ICU 72's change for the web's sake",
	},
	{
		Name: "TwoLetterTags", Flag: TwoLetterTags,
		Area: "Locale",
		What: "canonicalizing a tag that is two lowercase letters alone",
		Standard: "its aliases are replaced like any other tag's: \"bh\" is \"bho\" and " +
			"\"tw\" is \"ak\"",
		Node: "V8 answers it as it came, without consulting ICU, unless it is one of in, iw, ji, " +
			"jw, mo, sh, tl and no: \"bh\" stays \"bh\"",
	},
	{
		Name: "CollationKeyword", Flag: CollationKeyword,
		Area: "Collator",
		What: "a collation chosen by option, in the resolved locale",
		Standard: "the locale gains no keyword: ResolveLocale keeps a keyword only for " +
			"a value the locale itself asked for",
		Node: "unless the locale had a -u-co keyword it honoured, the locale gains one " +
			"for the option: new Intl.Collator(\"de\", {collation: \"eor\"}) resolves " +
			"to \"de-u-co-eor\"",
	},
	{
		Name: "DurationOverflow", Flag: DurationOverflow,
		Area: "DurationFormat",
		What: "a fraction of a second summed past 2**63 nanoseconds",
		Standard: "the sum is exact, as the proposal's arithmetic is: 1e20 nanoseconds in " +
			"digital style are 0:00:100000000000",
		Node: "V8 sums the smaller units in a double and converts it to an int64, which C++ " +
			"leaves undefined past 2**63 and x86-64 makes INT64_MIN: " +
			"0:00:9223372036.854775808, with a minus sign where it is the first unit written",
	},
	{
		Name: "TwelveHourCycle", Flag: TwelveHourCycle,
		Area: "DateTimeFormat",
		What: "the hour cycle hour12 chooses",
		Standard: "ECMA-402's [[HourCycle12]] and [[HourCycle24]] of the locale's data: the " +
			"first twelve- or twenty-four-hour cycle CLDR's time data allows for the locale's " +
			"region, so Japanese counts twelve hours from 0, h11: \"午前0:00\"",
		Node: "V8 takes h11 only for a locale whose region is Japan and h12 for every " +
			"other, so \"ja\" alone is h12: \"午前12:00\"",
	},
	{
		Name: "IslamicEras", Flag: IslamicEras,
		Area: "DateTimeFormat",
		What: "an Islamic year before the Hijrah",
		Standard: "CLDR's era before the Hijrah, the year counted back from it: 500 CE is " +
			"\"127 Before Hijrah\" (UTS #35, CLDR 48.2)",
		Node: "ICU4C's Islamic calendars have one era, and write the year before 1 AH " +
			"as a year of it: \"-126 Anno Hegirae\"",
	},
	{
		Name: "TemporalFormats", Flag: TemporalFormats,
		Area: "DateTimeFormat",
		What: "the format a Temporal value is written with",
		Standard: "the Temporal proposal's GetDateTimeFormat: an era alone is not a field " +
			"asked for, so the kind's defaults are written with it (\"5/2/2000 A\"), and the " +
			"resolved hour cycle is the value's too (hour12: false writes \"00:00:00\")",
		Node: "V8 counts an era among the fields asked for, writing \"A\" alone, and makes " +
			"the value's format with no hour cycle, writing \"12:00:00 AM\" for hour12: false",
	},
	{
		Name: "YesValues", Flag: YesValues,
		Area: "Locale",
		What: "a Unicode extension keyword whose value is \"yes\"",
		Standard: "UTS #35 replaces \"yes\" by its canonical \"true\", and leaves \"true\" out, " +
			"only for the keys whose BCP 47 data has that alias -- kb, kc, kh, kk and kn -- " +
			"so \"und-u-ka-yes\" stays as it is",
		Node: "ICU takes \"yes\" for any key as \"true\" and leaves it out: " +
			"\"und-u-ka-yes\" is \"und-u-ka\"",
	},
}

// A Divergence is one named difference between the profiles.
type Divergence struct {
	// Name is the flag's name, and Flag the flag.
	Name string
	Flag Compat
	// Area is the service it belongs to, "DateTimeFormat".
	Area string
	// What is the behavior, in one line.
	What string
	// Standard and Node say what each side does.
	Standard string
	Node     string
}
