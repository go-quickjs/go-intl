package intl_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/chrome"
)

// A recording is what one implementation answered to one of the testdata
// scripts: Node's, which the package's own data and NodeICU reproduce, or a
// Chrome's (testdata/chrome), which that Chrome's profile reproduces.
type recording struct {
	// who is the implementation, "Node" or "Chrome 155.0.8059.84".
	who string
	// src and compat are what reproduces it.
	src    intl.Source
	compat intl.Compat
	// header is the recording's first line, its maker's version.
	header string
	lines  []string
}

// recordings are a recording's versions: Node's, then the Chromes' that
// INTL_CHROME asks for -- versions separated by commas, or "all" -- each
// made from Node's and the edits Chrome's differs by. A Chrome a recording
// was not made with, because it predates what the script asks for, is left
// out.
func recordings(t *testing.T, file string) []recording {
	t.Helper()
	lines := strings.Split(string(readMaybeGzipped(t, file)), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	out := []recording{{who: "Node", src: intl.Embedded, compat: intl.NodeICU, lines: lines}}
	want := os.Getenv("INTL_CHROME")
	if want == "" {
		return withHeaders(out)
	}
	dirs, err := filepath.Glob(filepath.FromSlash("testdata/chrome/[0-9]*"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(dirs, func(i, j int) bool { return versionLess(filepath.Base(dirs[i]), filepath.Base(dirs[j])) })
	for _, dir := range dirs {
		version := filepath.Base(dir)
		if want != "all" && !contains(strings.Split(want, ","), version) {
			continue
		}
		delta := filepath.Join(dir, filepath.Base(file)+".delta.gz")
		if _, err := os.Stat(delta); err != nil {
			continue
		}
		p, err := chrome.ForVersion(version)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, recording{
			who: "Chrome " + version, src: p.Source, compat: p.Compat,
			lines: applyDelta(t, lines, delta),
		})
	}
	return withHeaders(out)
}

// replay runs a test of a recording over each of its versions, a subtest
// each, its header checked first.
func replay(t *testing.T, file string, run func(t *testing.T, r recording)) {
	for _, r := range recordings(t, file) {
		t.Run(r.who, func(t *testing.T) {
			r.checkHeader(t)
			run(t, r)
		})
	}
}

// compatFor is the divergences to answer a recording in: those a test of
// Node's recording chose, node, or a Chrome's profile's.
func (r recording) compatFor(node intl.Compat) intl.Compat {
	if r.who == "Node" {
		return node
	}
	return r.compat
}

// withHeaders takes each recording's header line off its lines.
func withHeaders(rs []recording) []recording {
	for i := range rs {
		if len(rs[i].lines) > 0 && strings.HasPrefix(rs[i].lines[0], "#") {
			rs[i].header, rs[i].lines = rs[i].lines[0], rs[i].lines[1:]
		}
	}
	return rs
}

// checkHeader is that a recording was made by what it should be: Node
// over ICU 78.3, or the Chrome it is filed under. A recording that names no
// maker is taken as it is.
func (r recording) checkHeader(t *testing.T) {
	t.Helper()
	if r.header == "" {
		return
	}
	ok := strings.HasPrefix(r.header, "# "+r.who+", ICU ")
	if r.who == "Node" {
		ok = namesICU(r.header, "78.3")
	}
	if !ok {
		t.Fatalf("%s: the expectations come from %q", r.who, r.header)
	}
}

// namesICU reports whether a header names that ICU and no other: "ICU 78.3"
// ended by the header, a comma or a space, so that ICU 78.30 is not it.
func namesICU(header, version string) bool {
	at := strings.Index(header, "ICU "+version)
	if at < 0 {
		return false
	}
	rest := header[at+len("ICU "+version):]
	return rest == "" || rest[0] == ',' || rest[0] == ' '
}

func readMaybeGzipped(t *testing.T, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(file))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(file, ".gz") {
		return b
	}
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// applyDelta makes a Chrome recording from Node's lines and its edits, as
// testdata/chrome/delta.js writes them: "=" keeps n lines, "-" drops n, "+"
// adds the lines given.
func applyDelta(t *testing.T, base []string, file string) []string {
	t.Helper()
	var d struct {
		Edits []json.RawMessage `json:"edits"`
	}
	if err := json.Unmarshal(readMaybeGzipped(t, file), &d); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	// delta.js's lines are the text split at each newline, a last empty
	// one included, which base has had taken off.
	base = append(base[:len(base):len(base)], "")
	var out []string
	at := 0
	for _, e := range d.Edits {
		var op []json.RawMessage
		var kind string
		if json.Unmarshal(e, &op) != nil || len(op) != 2 || json.Unmarshal(op[0], &kind) != nil {
			t.Fatalf("%s: an edit %s", file, e)
		}
		var n int
		var lines []string
		switch kind {
		case "=", "-":
			if err := json.Unmarshal(op[1], &n); err != nil || at+n > len(base) {
				t.Fatalf("%s: an edit %s past the end", file, e)
			}
			if kind == "=" {
				out = append(out, base[at:at+n]...)
			}
			at += n
		case "+":
			if err := json.Unmarshal(op[1], &lines); err != nil {
				t.Fatalf("%s: an edit %s", file, e)
			}
			out = append(out, lines...)
		default:
			t.Fatalf("%s: an edit %s", file, e)
		}
	}
	if at != len(base) {
		t.Fatalf("%s: the edits cover %d of %d lines", file, at, len(base))
	}
	if n := len(out); n > 0 && out[n-1] == "" {
		out = out[:n-1]
	}
	return out
}

func versionLess(a, b string) bool {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(x) && i < len(y); i++ {
		var m, n int
		fmt.Sscan(x[i], &m)
		fmt.Sscan(y[i], &n)
		if m != n {
			return m < n
		}
	}
	return len(x) < len(y)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
