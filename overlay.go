package intl

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/datapack"
)

// Overlays: data that differs from other data in a few files.
//
// A browser's ICU is not the ICU this package's data is made from, but it
// is close to it: of the thousands of tables, a few hundred differ, and of
// a pool's tens of thousands of strings, a few dozen are new. So its data
// is kept as an overlay of this package's rather than as a data set of its
// own: a data directory that holds only the files that differ, which
// replace the base's of the same name, and a manifest naming the base's
// files it removes. A shared pool can be an extension of the base's
// (blob.Extension), holding only the strings it adds, so that every table
// the overlay did not change still reads the base's pool. An overlay can be
// laid over another.

// overlayManifest is the file at an overlay's root that makes it one: the
// line "overlay 1", then the base's files the overlay removes, one to a
// line, sorted.
const overlayManifest = "overlay.bin"

const overlayHeader = "overlay 1"

// NewPack returns a Source that serves a data directory packed into one
// file, as data.pack is (internal/packgen), where it lies. The pack must
// not change for as long as anything read from it lives.
func NewPack(pack string) (Source, error) {
	if !datapack.Valid(pack) {
		return nil, fmt.Errorf("intl: not a data pack of version %d", datapack.Version)
	}
	return packSource{pack}, nil
}

// NewOverlay returns a Source that serves overlay's files over base's.
// overlay is a data directory holding the files that differ and the
// manifest naming those it removes; both it and base are a data directory
// this package reads -- Embedded, NewFS's, NewPack's, or another overlay.
func NewOverlay(base, overlay Source) (Source, error) {
	b, ok := base.(fileSource)
	if !ok {
		return nil, fmt.Errorf("intl: an overlay over a %T, which is not a data directory", base)
	}
	top, ok := overlay.(fileSource)
	if !ok {
		return nil, fmt.Errorf("intl: an overlay of a %T, which is not a data directory", overlay)
	}
	manifest, err := top.readFile(overlayManifest)
	if err != nil {
		return nil, fmt.Errorf("intl: an overlay without its %s: %w", overlayManifest, err)
	}
	lines := strings.Split(string(manifest), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || lines[0] != overlayHeader {
		return nil, fmt.Errorf("intl: %s does not begin %q", overlayManifest, overlayHeader)
	}
	removed := lines[1:]
	for i, name := range removed {
		if name == "" || i > 0 && removed[i-1] >= name {
			return nil, fmt.Errorf("intl: %s's removed files are not sorted and distinct at %q", overlayManifest, name)
		}
	}
	return &overlaySource{base: b, top: top, removed: removed}, nil
}

// A fileSource is a Source that is a data directory: one openFiles can
// read file by file.
type fileSource interface {
	Source
	files
}

type overlaySource struct {
	base, top fileSource
	removed   []string // sorted
}

func (s *overlaySource) Open(m Marker, d DataLocale) ([]byte, error) {
	return openFiles(s, m, d)
}

// readFile serves the overlay's file, then the base's where the overlay
// neither has the file nor removes it. The manifest is not a data file.
func (s *overlaySource) readFile(name string) ([]byte, error) {
	if name == overlayManifest {
		return nil, fs.ErrNotExist
	}
	b, err := s.top.readFile(name)
	if !errors.Is(err, fs.ErrNotExist) {
		return b, err
	}
	if s.removes(name) {
		return nil, fs.ErrNotExist
	}
	return s.base.readFile(name)
}

func (s *overlaySource) removes(name string) bool {
	i := sort.SearchStrings(s.removed, name)
	return i < len(s.removed) && s.removed[i] == name
}

// openPool opens a shared pool: the overlay's, read over the base's where
// it extends it, or the base's where the overlay leaves it be.
func (s *overlaySource) openPool(m Marker, version byte) (blob.Shared, error) {
	name := string(m) + ".bin"
	b, err := s.top.readFile(name)
	switch {
	case err == nil && blob.IsExtension(b):
		base, err := openShared(s.base, m, version)
		if err != nil {
			return blob.Shared{}, err
		}
		pool, err := blob.Extend(base, b, version)
		if err != nil {
			return blob.Shared{}, fmt.Errorf("intl: %s: %w", m, err)
		}
		return pool, nil
	case err == nil:
		pool, err := blob.ReadShared(b, version)
		if err != nil {
			return blob.Shared{}, fmt.Errorf("intl: %s: %w", m, err)
		}
		return pool, nil
	case !errors.Is(err, fs.ErrNotExist):
		return blob.Shared{}, fmt.Errorf("intl: %s: %w", m, err)
	case s.removes(name):
		return blob.Shared{}, fmt.Errorf("intl: %s: %w: the overlay removes it", m, ErrNotFound)
	}
	return openShared(s.base, m, version)
}
