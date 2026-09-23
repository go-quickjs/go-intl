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
// Number formatting has none so far. The ones go-quickjs carries today are in
// date formatting -- proleptic Islamic era names, the Japanese hour cycle, and
// Temporal's handling of a standalone era -- and they arrive with stage 5.
var Divergences = []Divergence{}

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
