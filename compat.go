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
// formats, the keyword value "yes", the currencies DisplayNames names, a
// duration's fraction of a second, the separator a digital duration's
// minutes take, the fields a date pattern's literals seem to write, the
// deprecated Islamic calendars, when a resolved locale keeps its hour cycle,
// the zone a plain Temporal value is read in, the time zone names a
// DateTimeFormat takes and reports, a coptic year before the era, the days
// of the Chinese and Korean calendars, and the region of a locale's hour
// cycles. Standard and NodeICU are the two ends.
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
	// does, rather than only where it stands for "true", and answers "yes"
	// for a keyword with no value where Intl.Locale's getCalendars and the
	// like give the keyword's value.
	YesValues
	// CurrencyNames names every currency CLDR has a name for, where the
	// standard names only those Intl.supportedValuesOf lists.
	CurrencyNames
	// DurationSeparator joins numeric minutes to whatever came before them
	// by the time separator, as V8 does, when the hours were left out.
	DurationSeparator
	// LiteralFields reads the fields DateTimeFormat's resolvedOptions
	// reports from its pattern's quoted literals as well as its fields.
	LiteralFields
	// IslamicFallback keeps the calendars islamic and islamic-rgsa for a
	// DateTimeFormat, where the standard settles them on islamic-civil.
	IslamicFallback
	// HourCycleKeyword keeps a DateTimeFormat's -u-hc keyword where
	// hour12 or hourCycle was given only if the hour cycle it settled on is
	// the keyword's.
	HourCycleKeyword
	// PlainValueZone writes a plain Temporal value as the instant it names
	// in a DateTimeFormat's zone, rather than as its wall-clock fields.
	PlainValueZone
	// ZoneIdentifiers takes any time zone ICU knows for a DateTimeFormat,
	// and reports ICU's canonical name for it rather than the one given.
	ZoneIdentifiers
	// CopticEra writes a coptic year before the era as ICU4C does, in an
	// era no data names.
	CopticEra
	// ChineseAstronomy reckons a DateTimeFormat's Chinese calendar and
	// Dangi by ICU4C's astronomy, rather than as Temporal does.
	ChineseAstronomy
	// SubdivisionHourCycles passes over a locale's "-u-sd-" subdivision
	// for the region of Intl.Locale's getHourCycles.
	SubdivisionHourCycles
)

const (
	// Standard is ECMA-402 as written, in every divergence.
	Standard Compat = 0
	// NodeICU reproduces Node's and ICU4C's observable behavior in every
	// divergence.
	NodeICU = NarrowSpace | TwoLetterTags | CollationKeyword | DurationOverflow |
		TwelveHourCycle | IslamicEras | TemporalFormats | YesValues | CurrencyNames |
		DurationSeparator | LiteralFields | IslamicFallback | HourCycleKeyword | PlainValueZone |
		ZoneIdentifiers | CopticEra | ChineseAstronomy | SubdivisionHourCycles
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
			"\"und-u-ka-yes\" is \"und-u-ka\"; and where a key has no value, which stands " +
			"for \"true\", getCalendars and the like answer ICU's own spelling of it: " +
			"\"en-u-ca\" has [\"yes\"], where its calendar is \"true\"",
	},
	{
		Name: "CurrencyNames", Flag: CurrencyNames,
		Area: "DisplayNames",
		What: "a currency Intl.supportedValuesOf does not list",
		Standard: "ECMA-402's AvailableCurrencies are the currencies DisplayNames and NumberFormat " +
			"provide for, so DisplayNames names only those: the Andorran peseta, ADP, has no name " +
			"(test262's currencies-accepted-by-DisplayNames)",
		Node: "V8 lists ICU's current currencies but names every one CLDR does: " +
			"\"Andorran Peseta\"",
	},
	{
		Name: "DurationSeparator", Flag: DurationSeparator,
		Area: "DurationFormat",
		What: "numeric minutes when numeric hours are left out",
		Standard: "the proposal's PartitionDurationFormatPattern joins the minutes to the hours " +
			"only where the hours were written, so the minutes start a group of their own: " +
			"\"1 day, 01:02\" (test262's digital-style-with-hours-display-auto-with-zero-hour)",
		Node: "V8 joins them to whatever was written last by the time separator: " +
			"\"1 day:01:02\"",
	},
	{
		Name: "LiteralFields", Flag: LiteralFields,
		Area: "DateTimeFormat",
		What: "resolvedOptions of a pattern whose quoted literal holds a field's letter",
		Standard: "CreateDateTimeFormat keeps the fields of the format it chose, and resolvedOptions " +
			"reports those: Portuguese {month: \"long\", year: \"numeric\"}, written " +
			"\"MMMM 'de' y\", reports a month and a year",
		Node: "V8 looks for each field's letters anywhere in the pattern's text, and finds the d " +
			"of 'de': it reports day: \"numeric\" too",
	},
	{
		Name: "IslamicFallback", Flag: IslamicFallback,
		Area: "DateTimeFormat",
		What: "the calendars islamic and islamic-rgsa",
		Standard: "CreateDateTimeFormat settles either on a calendar AvailableCalendars lists, " +
			"implementation-defined; go-intl takes islamic-civil, and writes in it " +
			"(test262's constructor-options-calendar-islamic-fallback)",
		Node: "V8 keeps them, and ICU writes in its astronomical Islamic calendar: " +
			"resolvedOptions().calendar is \"islamic\"",
	},
	{
		Name: "HourCycleKeyword", Flag: HourCycleKeyword,
		Area: "DateTimeFormat",
		What: "the -u-hc keyword of a resolved locale where hour12 or hourCycle was given",
		Standard: "ResolveLocale drops it for any hour12, which makes the option null, and for an " +
			"hourCycle that is another: \"en-u-hc-h23\" with {hour: \"numeric\", hour12: false} " +
			"is \"en\", and with {hourCycle: \"h23\"} alone \"en-u-hc-h23\"",
		Node: "V8 drops it where the hour cycle the formatter settled on, none where it writes " +
			"no hour, is another: the first is \"en-u-hc-h23\" and the second \"en\"",
	},
	{
		Name: "PlainValueZone", Flag: PlainValueZone,
		Area: "DateTimeFormat",
		What: "a plain Temporal value whose wall-clock time the formatter's zone skips",
		Standard: "the proposal writes a plain value's fields as they are, whatever the zone: " +
			"PlainDate 2011-12-30 in Pacific/Apia, a day Samoa skipped, is \"12/30/2011\" " +
			"(test262's PlainDate/prototype/toLocaleString/ignore-timezone)",
		Node: "V8 reads the value as the instant it names in the zone, with \"compatible\", " +
			"and writes that instant there: \"12/31/2011\"",
	},
	{
		Name: "ZoneIdentifiers", Flag: ZoneIdentifiers,
		Area: "DateTimeFormat",
		What: "the time zone names a DateTimeFormat takes, and the one resolvedOptions reports",
		Standard: "the IANA database's names, reported as given in its case: \"Asia/Calcutta\" is " +
			"\"Asia/Calcutta\", and \"ACT\" a RangeError (test262's timezone-not-canonicalized, " +
			"canonicalize-timezone and timezone-legacy-non-iana)",
		Node: "V8 takes any name ICU knows and reports ICU's canonical one: \"Asia/Kolkata\" is " +
			"\"Asia/Calcutta\", and \"ACT\" is \"Australia/Darwin\"",
	},
	{
		Name: "CopticEra", Flag: CopticEra,
		Area: "DateTimeFormat",
		What: "a coptic year before the era",
		Standard: "the calendar has one era, as Temporal and the era and month code proposal count " +
			"it, and a year before its first is 0 or below: 250 CE is \"-34 Anno Martyrum\" " +
			"(test262's formatToParts/era)",
		Node: "ICU4C writes an era before it, which neither ICU nor CLDR names: \"35\" and " +
			"no era",
	},
	{
		Name: "ChineseAstronomy", Flag: ChineseAstronomy,
		Area: "DateTimeFormat",
		What: "the days of the Chinese calendar and Dangi",
		Standard: "a formatter's days are Temporal's, which are ICU4X's: its tables from 1912 to " +
			"2102 and its mean-motion approximation outside them: ISO 2030-03-03 is the 29th " +
			"of the first month (test262's compare-to-temporal-lunisolar)",
		Node: "V8 formats with ICU4C, whose calendar computes the astronomy, and whose day is " +
			"not Temporal's there: the 30th",
	},
	{
		Name: "SubdivisionHourCycles", Flag: SubdivisionHourCycles,
		Area: "Locale",
		What: "getHourCycles of a locale with a \"-u-sd-\" subdivision and no region subtag",
		Standard: "RegionPreference takes the subdivision's region before the likely one: " +
			"\"en-u-sd-gbeng\" has Britain's [\"h23\"] (test262's getHourCycles/region-priority " +
			"and subdivision-region)",
		Node: "ICU's pattern generator passes over the subdivision, and V8 answers America's: " +
			"[\"h12\"]",
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
