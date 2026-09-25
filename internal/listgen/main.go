// Command listgen writes the list-pattern tables the intl package carries.
//
// It reads CLDR's cldr-misc-full package, which is not vendored: SOURCES.md
// pins the release and its checksum, and the path to an unpacked copy is given
// here.
//
//	curl -sLO https://registry.npmjs.org/cldr-misc-full/-/cldr-misc-full-48.0.0.tgz
//	tar xzf cldr-misc-full-48.0.0.tgz
//	go run ./internal/listgen package
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/listdata"
)

// names are the CLDR keys for each set, in the order listdata stores them.
var names = [listdata.Count]string{
	"listPattern-type-standard",
	"listPattern-type-standard-short",
	"listPattern-type-standard-narrow",
	"listPattern-type-or",
	"listPattern-type-or-short",
	"listPattern-type-or-narrow",
	"listPattern-type-unit",
	"listPattern-type-unit-short",
	"listPattern-type-unit-narrow",
}

type file struct {
	Main map[string]struct {
		ListPatterns map[string]struct {
			Two    string `json:"2"`
			Start  string `json:"start"`
			Middle string `json:"middle"`
			End    string `json:"end"`
		} `json:"listPatterns"`
	} `json:"main"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/listgen <cldr-misc-full/package>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "listgen:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	main := filepath.Join(root, "main")
	entries, err := os.ReadDir(main)
	if err != nil {
		return fmt.Errorf("reading %s: %w", main, err)
	}

	built := map[string][]byte{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		l, err := read(main, e.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if l == nil {
			continue
		}
		built[e.Name()] = listdata.Encode(l)
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}

	out := filepath.Join("data", "lists")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := datawrite.Locales(out, built); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "listgen: %d locales\n", len(built))
	return nil
}

func read(main, name string) (*listdata.Locale, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, "listPatterns.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("listPatterns.json: %w", err)
	}
	entry, ok := f.Main[name]
	if !ok {
		return nil, nil
	}
	var out listdata.Locale
	any := false
	for i, key := range names {
		p, ok := entry.ListPatterns[key]
		if !ok {
			continue
		}
		out.Sets[i] = listdata.Patterns{
			Two: p.Two, Start: p.Start, Middle: p.Middle, End: p.End,
		}
		any = true
	}
	if !any {
		return nil, nil
	}
	return &out, nil
}
