package icusrc

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/icufilter"
	"github.com/go-quickjs/go-intl/internal/icutxt"
)

// An Archive is ICU's data sources: the data archive SOURCES.md pins, its
// files under data/, or a directory holding a tree of the same sources at
// data/ -- a browser's ICU, whose sources it filters and patches -- with
// the data filter its build applies at filter.json, if it applies one.
//
// What is read of a directory's tree goes through its filter, as ICU's data
// build tool and genrb put the sources through it: a file the filter leaves
// out is not there, and a bundle has only the resources it keeps
// (icufilter). The pinned archive is checked against its checksum; a
// directory is trusted, having been checked as it was fetched.
type Archive struct {
	zip    *zip.ReadCloser
	files  map[string]*zip.File
	dir    string
	filter *icufilter.Config
}

// OpenArchive opens the data archive at path, or the tree in the directory
// at path.
func OpenArchive(name string) (*Archive, error) {
	info, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		z, err := Open(name, DataSHA256)
		if err != nil {
			return nil, err
		}
		a := &Archive{zip: z, files: map[string]*zip.File{}}
		for _, f := range z.File {
			a.files[f.Name] = f
		}
		return a, nil
	}
	a := &Archive{dir: name}
	if _, err := os.Stat(filepath.Join(name, "data")); err != nil {
		return nil, fmt.Errorf("icusrc: %s has no data directory", name)
	}
	b, err := os.ReadFile(filepath.Join(name, "filter.json"))
	switch {
	case err == nil:
		if a.filter, err = icufilter.Parse(b); err != nil {
			return nil, fmt.Errorf("icusrc: %s: %w", name, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	return a, nil
}

// Filtered reports whether the archive is read through a data filter.
func (a *Archive) Filtered() bool { return a.filter != nil }

// Close closes the archive.
func (a *Archive) Close() error {
	if a.zip != nil {
		return a.zip.Close()
	}
	return nil
}

// category is the data build tool's category of a file of the sources,
// "data/locales/en.txt" being in "locales_tree", and the file's stem, as
// ICU's filtration.py names them. Two files of a tree are no locale's and
// have a category of their own, curr/supplementalData.txt and
// zone/tzdbNames.txt; the break iterator's sources beneath brkitr are
// brkitr_rules, brkitr_dictionaries and the like.
func category(name string) (cat, stem string) {
	dir, file := path.Split(strings.TrimPrefix(name, "data/"))
	dir = strings.TrimSuffix(dir, "/")
	stem = strings.TrimSuffix(file, path.Ext(file))
	switch {
	case dir == "curr" && stem == "supplementalData":
		return "curr_supplemental", stem
	case dir == "zone" && stem == "tzdbNames":
		return "zone_supplemental", stem
	case strings.HasPrefix(dir, "brkitr/"):
		return "brkitr_" + strings.TrimPrefix(dir, "brkitr/"), stem
	}
	switch dir {
	case "locales", "curr", "lang", "region", "zone", "unit", "coll", "brkitr", "rbnf":
		return dir + "_tree", stem
	}
	return dir, stem
}

// kept reports whether the filter keeps a file. It filters the resource
// bundle sources, the .txt files; what else the build reads, a tree's
// LOCALE_DEPS.json, it reads whatever the filter says.
func (a *Archive) kept(name string) bool {
	if a.filter == nil || path.Ext(name) != ".txt" {
		return true
	}
	cat, stem := category(name)
	return a.filter.KeepFile(cat, stem)
}

// Names lists the files directly in a directory of the sources,
// "data/locales", sorted. A tree of sources that lacks the directory, or
// cannot be read, is an error, not an empty tree: a tree fetched whole
// has every directory the archive does.
func (a *Archive) Names(dir string) ([]string, error) {
	dir = strings.TrimSuffix(dir, "/") + "/"
	var out []string
	if a.zip != nil {
		for name := range a.files {
			if rest, ok := strings.CutPrefix(name, dir); ok && rest != "" && !strings.Contains(rest, "/") {
				out = append(out, name)
			}
		}
	} else {
		entries, err := os.ReadDir(filepath.Join(a.dir, filepath.FromSlash(dir)))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				out = append(out, dir+e.Name())
			}
		}
	}
	kept := out[:0]
	for _, name := range out {
		if a.kept(name) {
			kept = append(kept, name)
		}
	}
	sort.Strings(kept)
	return kept, nil
}

// ReadFile reads a file of the sources as it is: one the filter leaves out
// is not there, and nothing within one is filtered. A bundle is read with
// Bundle.
func (a *Archive) ReadFile(name string) ([]byte, error) {
	if !a.kept(name) {
		return nil, fmt.Errorf("%s: %w: the data filter leaves it out", name, fs.ErrNotExist)
	}
	return a.readRaw(name)
}

// readRaw reads a file whatever the filter says of it.
func (a *Archive) readRaw(name string) ([]byte, error) {
	if a.zip == nil {
		return os.ReadFile(filepath.Join(a.dir, filepath.FromSlash(name)))
	}
	f, ok := a.files[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w in the archive", name, fs.ErrNotExist)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Bundle reads a resource bundle source, "data/misc/supplementalData.txt",
// with what the filter drops of it dropped.
func (a *Archive) Bundle(name string) (*icutxt.Node, error) {
	b, err := a.ReadFile(name)
	if err != nil {
		return nil, err
	}
	n, err := icutxt.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if a.filter != nil {
		cat, stem := category(name)
		n = a.filter.Rules(cat, stem).Apply(n)
	}
	return n, nil
}
