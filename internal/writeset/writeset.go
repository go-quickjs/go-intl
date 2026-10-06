// Package writeset replaces the files a generator writes as one set, so
// that a run that fails, at any point, leaves the data it found.
//
// A generator adds everything it writes to a Set -- single files, and
// directories it owns whole -- and commits it once everything is built.
// Commit stages every file beside its target, then swaps each in, keeping
// the old one until all are in; a failure swapping puts the old ones back.
// Renames are retried for a while before they fail: on Windows a file a
// scanner or an indexer has open cannot be renamed, and a moment later it
// can be. Nothing here knows what the files hold, so packgen, which must
// build without the data it packs, can use it too.
package writeset

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The suffixes of a staged file and of an old one kept while a set is
// swapped in. packgen refuses to pack either.
const (
	StagedSuffix = ".tmp"
	OldSuffix    = ".old"
)

// A Set is the files a generator writes.
type Set struct {
	files map[string][]byte
	// trees are directories the set owns whole: a file in one the set
	// does not write is removed.
	trees []string
	// rename is os.Rename, retried; a test fails it.
	rename func(from, to string) error
}

// New returns an empty set.
func New() *Set { return &Set{files: map[string][]byte{}, rename: rename} }

// File adds a file.
func (s *Set) File(path string, data []byte) {
	s.files[filepath.Clean(path)] = data
}

// Tree adds a directory whose files, and only those, are files: each is
// named by its slash-separated path under dir.
func (s *Set) Tree(dir string, files map[string][]byte) {
	dir = filepath.Clean(dir)
	s.trees = append(s.trees, dir)
	for name, data := range files {
		s.files[filepath.Join(dir, filepath.FromSlash(name))] = data
	}
}

// WriteFile replaces one file, as a set of one.
func WriteFile(path string, data []byte) error {
	s := New()
	s.File(path, data)
	return s.Commit()
}

// A change replaces or removes one file.
type change struct {
	target string
	data   []byte
	remove bool
	// staged and kept say how far Commit took it.
	staged, kept, placed bool
}

// Commit writes the set. On an error the files it found are where they
// were, unless putting them back failed too, which the error says.
func (s *Set) Commit() error {
	changes, err := s.plan()
	if err != nil {
		return err
	}

	// Stage everything before replacing anything.
	for _, c := range changes {
		if c.remove {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(c.target), 0o755); err != nil {
			return s.abandon(changes, err)
		}
		if err := os.WriteFile(c.target+StagedSuffix, c.data, 0o644); err != nil {
			return s.abandon(changes, err)
		}
		c.staged = true
	}

	// Swap each in, keeping the old file.
	for _, c := range changes {
		if _, err := os.Lstat(c.target); err == nil {
			if err := s.rename(c.target, c.target+OldSuffix); err != nil {
				return s.rollBack(changes, err)
			}
			c.kept = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return s.rollBack(changes, err)
		}
		if !c.remove {
			if err := s.rename(c.target+StagedSuffix, c.target); err != nil {
				return s.rollBack(changes, err)
			}
			c.staged, c.placed = false, true
		}
	}

	// All are in: the old files can go, and the directories they leave
	// empty.
	var failed []string
	for _, c := range changes {
		if c.kept {
			if err := remove(c.target + OldSuffix); err != nil {
				failed = append(failed, c.target+OldSuffix)
			}
		}
	}
	for _, dir := range s.trees {
		removeEmptyDirs(dir)
	}
	if len(failed) > 0 {
		return fmt.Errorf("writeset: the new files are in, but %d old ones could not be removed, %s first; "+
			"delete them before packing", len(failed), failed[0])
	}
	return nil
}

// plan lists the changes, in order: the files written, and the files in a
// tree the set does not write. A file a failed run left staged is removed
// first; one it left kept means that run could not put the data back, and
// is refused.
func (s *Set) plan() ([]*change, error) {
	var changes []*change
	for path, data := range s.files {
		changes = append(changes, &change{target: path, data: data})
	}
	check := func(path string) error {
		switch {
		case strings.HasSuffix(path, OldSuffix):
			return fmt.Errorf("writeset: %s is left from a run that failed; restore the data "+
				"(git checkout -- data) and delete it", path)
		case strings.HasSuffix(path, StagedSuffix):
			return remove(path)
		}
		return nil
	}
	for path := range s.files {
		for _, p := range []string{path + StagedSuffix, path + OldSuffix} {
			if _, err := os.Lstat(p); err == nil {
				if err := check(p); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, dir := range s.trees {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return nil
			}
			if err != nil || d.IsDir() {
				return err
			}
			if strings.HasSuffix(path, StagedSuffix) || strings.HasSuffix(path, OldSuffix) {
				return check(path)
			}
			if _, ok := s.files[path]; !ok {
				changes = append(changes, &change{target: path, remove: true})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].target < changes[j].target })
	return changes, nil
}

// abandon removes what was staged, when staging failed.
func (s *Set) abandon(changes []*change, cause error) error {
	for _, c := range changes {
		if c.staged {
			remove(c.target + StagedSuffix)
		}
	}
	return fmt.Errorf("writeset: nothing was replaced: %w", cause)
}

// rollBack puts back the files a failed swap replaced.
func (s *Set) rollBack(changes []*change, cause error) error {
	var lost []string
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		if c.placed {
			if err := remove(c.target); err != nil {
				lost = append(lost, c.target)
				continue
			}
		}
		if c.kept {
			if err := rename(c.target+OldSuffix, c.target); err != nil {
				lost = append(lost, c.target)
			}
		}
		if c.staged {
			remove(c.target + StagedSuffix)
		}
	}
	for _, dir := range s.trees {
		removeEmptyDirs(dir)
	}
	if len(lost) > 0 {
		return fmt.Errorf("writeset: %w, and %d files could not be put back, %s first; "+
			"restore the data (git checkout -- data)", cause, len(lost), lost[0])
	}
	return fmt.Errorf("writeset: the old files are back: %w", cause)
}

// retryFor is how long a rename or a removal is tried before it fails.
const retryFor = 5 * time.Second

func rename(from, to string) error {
	return retry(func() error { return os.Rename(from, to) })
}

func remove(path string) error {
	return retry(func() error {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
}

// retry tries f until it succeeds or retryFor passes.
func retry(f func() error) error {
	deadline := time.Now().Add(retryFor)
	for wait := 10 * time.Millisecond; ; wait = min(2*wait, 500*time.Millisecond) {
		err := f()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(wait)
	}
}

// removeEmptyDirs removes the directories under dir, and dir, that a set
// left empty.
func removeEmptyDirs(dir string) {
	var dirs []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Remove(dirs[i]) // fails, as it should, on one that is not empty
	}
}
