// Command profilegen writes an overlay of the intl package's data: what a
// different ICU's data sources -- a browser's -- make differently from the
// sources the package's own data is made from (intl.NewOverlay).
//
//	go run ./internal/profilegen -tree <sources dir> -sources <icu4c-78.3-sources.tgz> -out <overlay.pack> [-work <dir>]
//
// The tree is ICU's data sources as icusrc.OpenArchive reads a directory:
// the sources at data/, and the data filter the build applies at
// filter.json. Every generator that reads the sources runs over it, into
// a working directory, each pool seeded with the package's own (-seed), so
// that a table whose strings did not change is the same bytes. What it
// writes is then compared with data/: a file that differs is the
// overlay's, a pool is written as the extension of data/'s it is, and a
// file of a data set it wrote that it did not write again is named in the
// manifest as removed. The overlay is packed as data.pack is.
//
// Run it from the repository root, after the package's own data is
// current.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/go-quickjs/go-intl/internal/blob"
	"github.com/go-quickjs/go-intl/internal/datapack"
	"github.com/go-quickjs/go-intl/internal/datedata"
	"github.com/go-quickjs/go-intl/internal/namedata"
	"github.com/go-quickjs/go-intl/internal/numdata"
	"github.com/go-quickjs/go-intl/internal/reltimedata"
	"github.com/go-quickjs/go-intl/internal/unitdata"
	"github.com/go-quickjs/go-intl/internal/zonedata"
)

// pools are the shared pools the generators seed, and each one's version.
var pools = map[string]byte{
	"reltimeshared.bin":   reltimedata.Version,
	"unitsshared.bin":     unitdata.Version,
	"namesshared.bin":     namedata.Version,
	"numbersshared.bin":   numdata.Version,
	"datesshared.bin":     datedata.Version,
	"zonenamesshared.bin": zonedata.Version,
}

// A step is one generator's run: its arguments, with {tree} the sources,
// {dat} the data built from them, {base} the package's data, {sources} ICU
// 78.3's sources, {export} its collation export and {ucd} the Unicode
// Character Database, and the pool it seeds, if it has one.
type step struct {
	name string
	pool string
	args []string
}

// steps are the generators that read ICU's data sources, in an order that
// runs zonegen after dategen, whose locales it reads. A tree carries its own
// time zone files, so the time zone update is "-".
var steps = []step{
	{"listgen", "", []string{"{tree}"}},
	{"reltimegen", "reltimeshared.bin", []string{"{tree}"}},
	{"unitgen", "unitsshared.bin", []string{"{tree}"}},
	{"namegen", "namesshared.bin", []string{"{tree}"}},
	{"numbergen", "numbersshared.bin", []string{"{tree}"}},
	{"dategen", "datesshared.bin", []string{"{tree}"}},
	{"zonegen", "zonenamesshared.bin", []string{"{tree}", "-"}},
	{"aliasgen", "", []string{"{tree}", "-"}},
	{"availgen", "", []string{"{tree}", "-"}},
	{"rbnfgen", "", []string{"{tree}"}},
	{"pluralgen", "", []string{"{tree}"}},
	// The collations are ICU 78.3's export, of the locales and types the
	// tree's filter keeps.
	{"collgen", "", []string{"{export}", "{tree}", "{sources}"}},
	// The break rules and dictionaries, compiled, from the data built.
	{"segmentgen", "", []string{"{dat}", "{tree}", "{ucd}"}},
	// The zones' offsets, from the tree's own time zone files; the zones a
	// host's abbreviations stand for are ICU 78.3's.
	{"tzgen", "", []string{"{tree}", "{sources}"}},
}

func main() {
	tree := flag.String("tree", "", "the data sources to make the overlay from")
	out := flag.String("out", "", "the overlay pack to write")
	sources := flag.String("sources", "", "ICU 78.3's sources, for tzgen and collgen")
	export := flag.String("export", "", "ICU4X's export of ICU 78.3's collations, for collgen")
	dat := flag.String("dat", "", "the common data file built from the sources, for segmentgen")
	ucd := flag.String("ucd", "", "the Unicode Character Database 17.0.0's directory, for segmentgen")
	work := flag.String("work", "", "the working directory (default: a temporary one, removed after)")
	against := flag.String("against", "data", "the data the overlay is of, the intl package's or what another run made")
	flag.Parse()
	if *tree == "" || *out == "" || *sources == "" || *export == "" || *dat == "" || *ucd == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/profilegen -tree <sources dir> -dat <icudtl.dat> -sources <icu4c-78.3-sources.tgz> -export <icu4x-icuexportdata-78.3.zip> -ucd <ucd-17.0.0 dir> -out <overlay.pack> [-work <dir>]")
		os.Exit(2)
	}
	inputs := map[string]string{"{tree}": *tree, "{dat}": *dat, "{sources}": *sources, "{export}": *export, "{ucd}": *ucd}
	if err := run(inputs, *against, *out, *work); err != nil {
		fmt.Fprintln(os.Stderr, "profilegen:", err)
		os.Exit(1)
	}
}

func run(inputs map[string]string, against, out, work string) error {
	if b, err := os.ReadFile("go.mod"); err != nil || !bytes.HasPrefix(b, []byte("module github.com/go-quickjs/go-intl\n")) {
		return errors.New("run from go-intl's repository root")
	}
	base, err := filepath.Abs(against)
	if err != nil {
		return err
	}
	replace := []string{"{base}", base}
	for k, v := range inputs {
		abs, err := filepath.Abs(v)
		if err != nil {
			return err
		}
		replace = append(replace, k, abs)
	}
	replacer := strings.NewReplacer(replace...)
	if work == "" {
		if work, err = os.MkdirTemp("", "profilegen-"); err != nil {
			return err
		}
		defer os.RemoveAll(work)
	} else if work, err = filepath.Abs(work); err != nil {
		return err
	}
	// What a run for another tree wrote is not this one's.
	if err := os.RemoveAll(filepath.Join(work, "data")); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(work, "data"), 0o755); err != nil {
		return err
	}

	// The generators are built once and run in the working directory,
	// where each writes data/ as it would in the repository.
	bin := filepath.Join(work, "bin")
	for _, s := range steps {
		exe := filepath.Join(bin, s.name+exeSuffix())
		cmd := exec.Command("go", "build", "-o", exe, "./internal/"+s.name)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("building %s: %w", s.name, err)
		}
	}
	for _, s := range steps {
		var args []string
		if s.pool != "" {
			args = append(args, "-seed", filepath.Join(base, s.pool))
		}
		for _, a := range s.args {
			args = append(args, replacer.Replace(a))
		}
		cmd := exec.Command(filepath.Join(bin, s.name+exeSuffix()), args...)
		cmd.Dir = work
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	files, removed, err := overlay(base, filepath.Join(work, "data"))
	if err != nil {
		return err
	}
	fsys := fstest.MapFS{}
	for name, b := range files {
		fsys[name] = &fstest.MapFile{Data: b}
	}
	fsys["overlay.bin"] = &fstest.MapFile{Data: []byte("overlay 1\n" + strings.Join(removed, "\n") + suffix(removed))}
	pack, err := datapack.Build(fsys)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, pack, 0o644); err != nil {
		return err
	}
	fmt.Printf("profilegen: %d files differ, %d removed, %d bytes\n", len(files), len(removed), len(pack))
	return nil
}

func suffix(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return "\n"
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// overlay compares what the generators wrote, made, with base: the files
// that differ, a pool as its extension of base's, by their slash-separated
// names, and the base files of a directory made has that made did not
// write, sorted.
func overlay(base, made string) (map[string][]byte, []string, error) {
	files := map[string][]byte{}
	dirs := map[string]bool{}
	err := filepath.WalkDir(made, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(made, p)
		name := filepath.ToSlash(rel)
		if dir, _, ok := strings.Cut(name, "/"); ok {
			dirs[dir] = true
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		old, err := os.ReadFile(filepath.Join(base, rel))
		if err == nil && bytes.Equal(old, b) {
			return nil
		}
		if version, ok := pools[name]; ok && err == nil {
			if b, err = extension(old, b, version); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		files[name] = b
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	var removed []string
	for dir := range dirs {
		err := filepath.WalkDir(filepath.Join(base, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(base, p)
			if _, err := os.Stat(filepath.Join(made, rel)); errors.Is(err, fs.ErrNotExist) {
				removed = append(removed, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Strings(removed)
	return files, removed, nil
}

// extension is a seeded pool's extension of the base pool.
func extension(old, full []byte, version byte) ([]byte, error) {
	base, err := blob.ReadShared(old, version)
	if err != nil {
		return nil, err
	}
	pool, err := blob.ReadShared(full, version)
	if err != nil {
		return nil, err
	}
	return blob.ExtensionOf(base, pool, version)
}
