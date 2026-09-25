package intl

// Which behavior to reproduce where Node and the standard disagree.
//
// Node, and so ICU4C, differs from ECMA-402 as written in a handful of
// observable places. A host that cares about conformance wants the standard; a
// host that cares about matching Node wants Node. Both are legitimate, so the
// choice is made when a formatter is built, and it is not ambient state: two
// formatters in one program may answer differently.
//
// The rule that keeps this honest is that every divergence is a named,
// documented entry with a test on both sides. It is a short list, not a
// licence to be bug-compatible with all of ICU.
//
// If the list ever passes about twenty entries, that is a sign the standard
// behavior is wrong somewhere and this is hiding it, rather than a sign that
// Node has drifted further.

// Compat chooses between the standard and Node's observable behavior. The zero
// value is the standard.
type Compat int

const (
	// Standard is ECMA-402 as written.
	Standard Compat = iota
	// NodeICU reproduces Node's and ICU4C's observable behavior where it
	// differs from the standard.
	NodeICU
)

func (c Compat) String() string {
	if c == NodeICU {
		return "NodeICU"
	}
	return "Standard"
}

// Divergences are the places the two profiles differ, listed so that the set
// can be read rather than discovered. Nothing consults this; it is here to be
// kept honest and to be printed when someone asks what NodeICU changes.
//
// Number formatting has none so far. The ones go-quickjs carries today in
// date formatting -- proleptic Islamic era names, the Japanese hour cycle, and
// Temporal's handling of a standalone era -- arrive with the calendars.
var Divergences = []Divergence{
	{
		Area:     "DateTimeFormat",
		What:     "the space before a day period, and every other narrow no-break space",
		Standard: "CLDR's character, U+202F, as ICU writes it: \"3:04\u202fPM\"",
		Node: "a plain space: V8 replaces U+202F in everything it formats, " +
			"reverting ICU 72's change for the web's sake",
	},
	{
		Area: "Locale",
		What: "canonicalizing a tag that is two lowercase letters alone",
		Standard: "its aliases are replaced like any other tag's: \"bh\" is \"bho\" and " +
			"\"tw\" is \"ak\"",
		Node: "V8 answers it as it came, without consulting ICU, unless it is one of in, iw, ji, " +
			"jw, mo, sh, tl and no: \"bh\" stays \"bh\"",
	},
	{
		Area: "Collator",
		What: "a collation chosen by option, in the resolved locale",
		Standard: "the locale gains no keyword: ResolveLocale keeps a keyword only for " +
			"a value the locale itself asked for",
		Node: "unless the locale had a -u-co keyword it honoured, the locale gains one " +
			"for the option: new Intl.Collator(\"de\", {collation: \"eor\"}) resolves " +
			"to \"de-u-co-eor\"",
	},
	{
		Area: "DurationFormat",
		What: "a fraction of a second summed past 2**63 nanoseconds",
		Standard: "the sum is exact, as the proposal's arithmetic is: 1e20 nanoseconds in " +
			"digital style are 0:00:100000000000",
		Node: "V8 sums the smaller units in a double and converts it to an int64, which C++ " +
			"leaves undefined past 2**63 and x86-64 makes INT64_MIN: " +
			"0:00:9223372036.854775808, with a minus sign where it is the first unit written",
	},
}

// A Divergence is one named difference between the profiles.
type Divergence struct {
	// Area is the service it belongs to, "DateTimeFormat".
	Area string
	// What is the behavior, in one line.
	What string
	// Standard and Node say what each profile does.
	Standard string
	Node     string
}
