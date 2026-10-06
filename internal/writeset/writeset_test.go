package writeset

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contents is every file under dir, by slash-separated path.
func contents(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// populate writes files under dir.
func populate(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

var old = map[string]string{
	"a.bin":              "old a",
	"tz/x.bin":           "old x",
	"tz/gone.bin":        "old gone",
	"tz/region/y.bin":    "old y",
	"tz/emptied/z.bin":   "old z",
	"untouched/keep.bin": "kept",
}

func newSet(dir string) *Set {
	s := New()
	s.File(filepath.Join(dir, "a.bin"), []byte("new a"))
	s.File(filepath.Join(dir, "b.bin"), []byte("new b"))
	s.Tree(filepath.Join(dir, "tz"), map[string][]byte{
		"x.bin":        []byte("new x"),
		"region/y.bin": []byte("new y"),
		"added/w.bin":  []byte("new w"),
	})
	return s
}

// A set replaces its files, removes those of its directories it does not
// write, and the directories that leaves empty, and touches nothing else.
func TestCommit(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, old)
	if err := newSet(dir).Commit(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"a.bin":              "new a",
		"b.bin":              "new b",
		"tz/x.bin":           "new x",
		"tz/region/y.bin":    "new y",
		"tz/added/w.bin":     "new w",
		"untouched/keep.bin": "kept",
	}
	if got := contents(t, dir); !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "tz", "emptied")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("tz/emptied is still there: %v", err)
	}
}

// A run that fails, staging or swapping, leaves the files it found and
// nothing of its own: datawrite had deleted the old files before writing,
// and tzgen had removed data/tz before a rename Windows refused, leaving
// no zones at all (ISSUES.md RU-2).
func TestFailureLeavesTheOldFiles(t *testing.T) {
	t.Run("staging", func(t *testing.T) {
		dir := t.TempDir()
		populate(t, dir, old)
		s := newSet(dir)
		// A file where a directory has to be made.
		s.File(filepath.Join(dir, "a.bin", "under.bin"), []byte("impossible"))
		if err := s.Commit(); err == nil || !strings.Contains(err.Error(), "nothing was replaced") {
			t.Fatalf("Commit: %v", err)
		}
		if got := contents(t, dir); !equal(got, old) {
			t.Errorf("got %v, want %v", got, old)
		}
	})
	for _, failAt := range []int{1, 2, 5, 9} {
		t.Run("swapping", func(t *testing.T) {
			dir := t.TempDir()
			populate(t, dir, old)
			s := newSet(dir)
			n := 0
			s.rename = func(from, to string) error {
				if n++; n == failAt {
					return errors.New("access is denied")
				}
				return os.Rename(from, to)
			}
			if err := s.Commit(); err == nil || !strings.Contains(err.Error(), "the old files are back") {
				t.Fatalf("Commit failing rename %d: %v", failAt, err)
			}
			if got := contents(t, dir); !equal(got, old) {
				t.Errorf("failing rename %d: got %v, want %v", failAt, got, old)
			}
		})
	}
}

// A staged file a failed run left is cleared; an old file it kept means it
// could not put the data back, and is refused.
func TestLeftovers(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, old)
	populate(t, dir, map[string]string{"a.bin.tmp": "stale", "tz/x.bin.tmp": "stale"})
	if err := newSet(dir).Commit(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.bin.tmp", "tz/x.bin.tmp"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s is still there", name)
		}
	}

	populate(t, dir, map[string]string{"tz/x.bin.old": "kept by a failed run"})
	before := contents(t, dir)
	if err := newSet(dir).Commit(); err == nil || !strings.Contains(err.Error(), "left from a run that failed") {
		t.Fatalf("Commit: %v", err)
	}
	if got := contents(t, dir); !equal(got, before) {
		t.Errorf("got %v, want %v", got, before)
	}
}
