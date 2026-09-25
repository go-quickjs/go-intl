package datawrite

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// TestLocales writes a data set with copies in it and reads every locale
// back through a Source, which must find each copy's data where it went.
func TestLocales(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"und":         []byte("root"),
		"en":          []byte("english"),
		"en-001":      []byte("world english"),
		"en-AG":       []byte("world english"),
		"en-BB":       []byte("world english"),
		"en-US":       []byte("english"),
		"fr":          []byte("root"),
		"en-US-posix": []byte("english"),
		"be-tarask":   []byte("unreadable"),
	}
	if err := Locales(filepath.Join(root, "things"), files); err != nil {
		t.Fatal(err)
	}

	// The copies are written once: under the root, the shortest tag, and
	// the variant, which same.bin cannot name.
	var own []string
	paths, _ := filepath.Glob(filepath.Join(root, "things", "*.bin"))
	for _, p := range paths {
		own = append(own, filepath.Base(p))
	}
	sort.Strings(own)
	want := []string{"en-AG.bin", "en-US-posix.bin", "en.bin", "same.bin", "und.bin"}
	if !reflect.DeepEqual(own, want) {
		t.Errorf("wrote %v, want %v", own, want)
	}

	src := intl.NewFS(os.DirFS(root))
	for tag, data := range files {
		if tag == "be-tarask" {
			continue
		}
		l, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		got, err := src.Open("things", l.Data())
		if err != nil {
			t.Errorf("%s: %v", tag, err)
			continue
		}
		if string(got) != string(data) {
			t.Errorf("%s: got %q, want %q", tag, got, data)
		}
	}
	// A locale with no data of its own or another's is not found, which a
	// fallback chain goes on from.
	l, _ := intl.ParseLocale("de")
	if _, err := src.Open("things", l.Data()); err == nil {
		t.Error("de: found data that was never written")
	}

	tags, err := Tags(filepath.Join(root, "things"))
	if err != nil {
		t.Fatal(err)
	}
	wantTags := []string{"en", "en-001", "en-AG", "en-BB", "en-US", "en-US-posix", "fr", "und"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Errorf("Tags = %v, want %v", tags, wantTags)
	}
}
