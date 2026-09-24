package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// dateTimeGaps are cases the expectations hold go-intl to that it does not
// meet yet, each with the reason. A case in one that starts to pass is
// reported, so the entry goes.
var dateTimeGaps = map[string]string{
	// ICU's zone-name tree has no bundle for sr_Cyrl_ME, and ICU's fallback
	// for a missing bundle steps from a locale whose script is its
	// language's default to the language and region: sr_ME, an alias of
	// sr_Latn_ME. So Node names zones in Montenegrin Cyrillic Serbian in
	// Latin letters. go-intl falls back as CLDR does and writes Cyrillic.
	// Reproducing it needs ICU's per-tree fallback; see PLAN.md.
	"sr-Cyrl-ME": "ICU's zone-tree fallback",
}

// TestDateTimeFormatMatchesNode holds every locale with date data to what
// Node writes under a spread of styles and field sets, in two zones and two
// seasons. The expectations are written by testdata/datetime_node.js.
func TestDateTimeFormatMatchesNode(t *testing.T) {
	f, err := os.Open(filepath.FromSlash("testdata/datetime_node.txt.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(make([]byte, 1<<20), 1<<20)

	var ran, matched, gaps int
	var differences []string
	var lastKey string
	var last *intl.DateTimeFormat
	var lastErr error
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 5 {
			t.Fatalf("a line that is not five fields: %s", line)
		}
		var tag, want, cycle string
		var opts map[string]any
		var when float64
		for i, dst := range []any{&tag, &opts, &when, &want, &cycle} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		ran++
		name := fmt.Sprintf("%s %s %v", tag, fields[1], int64(when))
		// The cases come two instants to a formatter, in a row.
		if key := tag + string(fields[1]); key != lastKey {
			o, err := dateTimeOptions(opts)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			last, lastErr = intl.NewDateTimeFormat(loc, o)
			lastKey = key
		}
		if lastErr != nil {
			differences = append(differences, name+": "+lastErr.Error())
			continue
		}
		got := last.Format(time.UnixMilli(int64(when)))
		if why, gap := dateTimeGaps[tag]; gap && strings.Contains(string(fields[1]), `"timeZoneName":"long"`) {
			ran--
			gaps++
			if got == want {
				differences = append(differences, fmt.Sprintf("%s: passes, so %q is no longer a gap", name, why))
			}
			continue
		}
		if got == want {
			matched++
			continue
		}
		differences = append(differences, fmt.Sprintf("%s\n   got  %+q\n   want %+q", name, got, want))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly; %d left out as known gaps", matched, ran, gaps)
	if len(differences) > 0 {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 && os.Getenv("ALLDIFF") == "" {
			shown = shown[:25]
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(shown, "\n"))
	}
}
