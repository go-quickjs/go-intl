package temporal

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/chrome"
)

// What each side of AnnotationLookahead makes of a string. An empty answer
// is that the string is taken.
func TestAnnotationLookahead(t *testing.T) {
	for _, c := range []struct{ s, standard, node string }{
		// A part of one character, which Node's parser looks past.
		{"2020-06-15T12:00Z[a=b]", "", "Invalid annotation key character."},
		{"2020-06-15T12:00Z[!a=b]", "Unrecognized annoation is marked as critical.", "Invalid annotation key character."},
		{"2020-06-15T12:00Z[u-ca=x]", "", "Invalid annotation value character."},
		{"2020-06-15T12:00Z[aa=b-c]", "", "Expected annotation value character must exist after hyphen."},
		// A value's first character, and what follows a hyphen, which it
		// does not look at.
		{"2020-06-15T12:00Z[aa=-b]", "Invalid annotation value character.", ""},
		{"2020-06-15T12:00Z[aa=!bc]", "Invalid annotation value character.", ""},
		{"2020-06-15T12:00Z[aa=b--c]", "Expected annotation value character must exist after hyphen.", ""},
		// A zone's name, whose last character it does not look at.
		{"2020-06-15T12:00Z[Asia/Calcutta/]", "Expected time zone character after '/'.", ""},
		{"2020-06-15T12:00Z[UTC!]", "Invalid IANA time zone character after '/'.", ""},
		// Where a string ends.
		{"2020-06-15T12:00Z[aa=b", "Invalid annotation close character.", "Invalid annotation value character."},
		{"2020-06-15T12:00Z[aa=bb-]", "Expected annotation value character must exist after hyphen.", ""},
		// What both read alike.
		{"2020-06-15T12:00Z[aa=bb]", "", ""},
		{"2020-06-15T12:00Z[a]", "", ""},
		{"2020-06-15T12:00Z[aa", "Parsing ended abruptly.", "Parsing ended abruptly."},
		{"2020-06-15T12:00Z[aa=]", "Invalid annotation value character.", "Invalid annotation value character."},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.NodeICU, c.node}} {
			got := ""
			if _, err := ParseInstant([]byte(c.s), side.compat); err != nil {
				got = strings.TrimPrefix(err.Error(), "RangeError: ")
			}
			if !strings.HasSuffix(got, side.want) || (side.want == "") != (got == "") {
				t.Errorf("%s, %v: %q, want %q", c.s, side.compat, got, side.want)
			}
		}
	}
}

// TestAnnotationRecordings replays testdata/temporal_annotations_node.js in
// Node, which reads annotations a character ahead, and in Chrome 158, the
// first that does not, through every call that reads a string.
func TestAnnotationRecordings(t *testing.T) {
	const file = "temporal_annotations_node.txt.gz"
	b, err := os.ReadFile("../testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	node := strings.Split(string(gunzip(t, b)), "\n")
	b, err = os.ReadFile("../testdata/chrome/158.0.8127.18/" + file + ".delta.gz")
	if err != nil {
		t.Fatal(err)
	}
	var delta struct {
		Edits [][2]json.RawMessage `json:"edits"`
	}
	if err := json.Unmarshal(gunzip(t, b), &delta); err != nil {
		t.Fatal(err)
	}
	var edited []string
	at := 0
	for _, e := range delta.Edits {
		var op string
		json.Unmarshal(e[0], &op)
		if op == "+" {
			var added []string
			json.Unmarshal(e[1], &added)
			edited = append(edited, added...)
			continue
		}
		var n int
		json.Unmarshal(e[1], &n)
		if op == "=" {
			edited = append(edited, node[at:at+n]...)
		}
		at += n
	}
	p, err := chrome.ForVersion("158.0.8127.18")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		who    string
		lines  []string
		compat intl.Compat
	}{{"Node", node, intl.NodeICU}, {"Chrome 158", edited, p.Compat}} {
		t.Run(r.who, func(t *testing.T) {
			h, err := newHarness()
			if err != nil {
				t.Fatal(err)
			}
			h.compat = r.compat
			ran, bad := 0, 0
			for _, line := range r.lines {
				if line == "" || line[0] == '#' {
					continue
				}
				ran++
				if _, wrong := h.replay(t, []byte(line)); wrong != "" {
					if bad++; bad <= 12 {
						t.Error(wrong)
					}
				}
			}
			t.Logf("%d of %d cases differ", bad, ran)
			if ran != 4720 {
				t.Errorf("%d cases", ran)
			}
		})
	}
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	z, err := gzip.NewReader(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
