// Command reltimegen writes the relative-time tables the intl package carries.
//
// It reads CLDR's cldr-dates-full package, which is not vendored: SOURCES.md
// pins the release and its checksum, and the path to an unpacked copy is given
// here.
//
//	curl -sLO https://registry.npmjs.org/cldr-dates-full/-/cldr-dates-full-48.0.0.tgz
//	tar xzf cldr-dates-full-48.0.0.tgz
//	go run ./internal/reltimegen package
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/reltimedata"
)

type file struct {
	Main map[string]struct {
		Dates struct {
			Fields map[string]json.RawMessage `json:"fields"`
		} `json:"dates"`
	} `json:"main"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/reltimegen <cldr-dates-full/package>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "reltimegen:", err)
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
		built[e.Name()] = reltimedata.Encode(l)
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}

	out := filepath.Join("data", "reltime")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := datawrite.Locales(out, built); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "reltimegen: %d locales\n", len(built))
	return nil
}

func read(main, name string) (*reltimedata.Locale, error) {
	raw, err := os.ReadFile(filepath.Join(main, name, "dateFields.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("dateFields.json: %w", err)
	}
	entry, ok := f.Main[name]
	if !ok {
		return nil, nil
	}

	var out reltimedata.Locale
	any := false
	for unit := 0; unit < reltimedata.Units; unit++ {
		for width := 0; width < reltimedata.Widths; width++ {
			key := reltimedata.Names[unit] + reltimedata.WidthNames[width]
			body, ok := entry.Dates.Fields[key]
			if !ok {
				continue
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			target := &out.Fields[unit*reltimedata.Widths+width]
			for name, raw := range fields {
				switch {
				case strings.HasPrefix(name, "relative-type-"):
					offset, err := strconv.Atoi(strings.TrimPrefix(name, "relative-type-"))
					if err != nil {
						continue
					}
					var text string
					if err := json.Unmarshal(raw, &text); err != nil || text == "" {
						continue
					}
					target.Named = append(target.Named, reltimedata.Named{
						Offset: offset, Text: text,
					})
				case name == "relativeTime-type-future", name == "relativeTime-type-past":
					var patterns map[string]string
					if err := json.Unmarshal(raw, &patterns); err != nil {
						continue
					}
					set := counted(patterns)
					if name == "relativeTime-type-future" {
						target.Future = set
					} else {
						target.Past = set
					}
				}
			}
			sort.Slice(target.Named, func(i, j int) bool {
				return target.Named[i].Offset < target.Named[j].Offset
			})
			if len(target.Future) > 0 || len(target.Named) > 0 {
				any = true
			}
		}
	}
	if !any {
		return nil, nil
	}
	return &out, nil
}

func counted(in map[string]string) []reltimedata.CountedText {
	out := make([]reltimedata.CountedText, 0, len(in))
	for key, text := range in {
		count, ok := strings.CutPrefix(key, "relativeTimePattern-count-")
		if !ok || text == "" {
			continue
		}
		out = append(out, reltimedata.CountedText{Count: count, Text: text})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count < out[j].Count })
	return out
}
