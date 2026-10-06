package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// tarball writes an npm-style package tarball of files.
func tarball(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	tw := tar.NewWriter(z)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// unpack does not trust a tree because a marker is in it: a file changed
// or removed since, or another archive, extracts it again. It had skipped
// any directory with a marker, whatever was in it (ISSUES.md RU-7).
func TestUnpackChecksTheTree(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "pkg.tgz")
	tree := filepath.Join(dir, "pkg")
	tarball(t, archive, map[string]string{"package/a.json": "A", "package/sub/b.json": "B"})
	a, b := filepath.Join(tree, "package", "a.json"), filepath.Join(tree, "package", "sub", "b.json")

	if err := unpack(archive, "sum1", tree); err != nil {
		t.Fatal(err)
	}
	if read(t, a) != "A" || read(t, b) != "B" {
		t.Fatal("not extracted")
	}

	// Nothing changed: the tree is left as it is, the extra file too.
	extra := filepath.Join(tree, "package", "extra")
	stamp := read(t, filepath.Join(tree, ".unpacked"))
	if err := unpack(archive, "sum1", tree); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(tree, ".unpacked")) != stamp {
		t.Error("an unchanged tree was extracted again")
	}

	// A file edited, with its size changed, is put back.
	if err := os.WriteFile(a, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unpack(archive, "sum1", tree); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a); got != "A" {
		t.Errorf("an edited file is %q", got)
	}

	// A file edited to the same size, later, is put back.
	later := time.Now().Add(time.Hour)
	if err := os.WriteFile(a, []byte("Z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(a, later, later); err != nil {
		t.Fatal(err)
	}
	if err := unpack(archive, "sum1", tree); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a); got != "A" {
		t.Errorf("a file edited to its own size is %q", got)
	}

	// A file removed, or one added, is noticed.
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	if err := unpack(archive, "sum1", tree); err != nil {
		t.Fatal(err)
	}
	if got := read(t, b); got != "B" {
		t.Errorf("a removed file is %q", got)
	}
	if err := os.WriteFile(extra, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unpack(archive, "sum1", tree); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Error("a file added to the tree survived")
	}

	// Another archive is extracted, though the tree is as the last left it.
	tarball(t, archive, map[string]string{"package/a.json": "A2"})
	if err := unpack(archive, "sum2", tree); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a); got != "A2" {
		t.Errorf("after another archive a.json is %q", got)
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Error("a file of the old archive survived")
	}

	// A tree without its marker, as an extraction cut short leaves, is
	// extracted again.
	if err := os.Remove(filepath.Join(tree, ".unpacked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unpack(archive, "sum2", tree); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a); got != "A2" {
		t.Errorf("after an extraction cut short a.json is %q", got)
	}
}

// Neither a download nor a generator can hang regen for ever (RU-7).
func TestTimeouts(t *testing.T) {
	if client.Timeout <= 0 {
		t.Error("downloads have no timeout")
	}
	if generatorTimeout <= 0 {
		t.Error("generators have no timeout")
	}
}
