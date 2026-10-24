// Command reltimegen writes the relative-time tables the intl package carries.
//
// It reads ICU's locale bundles, data/locales/*.txt in the data archive
// SOURCES.md pins: each locale's fields as ICU resolves them.
//
//	go run ./internal/reltimegen [-seed pool.bin] icu4c-78.3-data.zip
//
// -seed numbers the pool's parts after an earlier pool's (datawrite.NewPool).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/icutxt"
	"github.com/go-quickjs/go-intl/internal/reltimedata"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

func main() {
	seed := flag.String("seed", "", "a pool to number the parts after")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/reltimegen [-seed pool.bin] <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *seed); err != nil {
		fmt.Fprintln(os.Stderr, "reltimegen:", err)
		os.Exit(1)
	}
}

func run(zip, seed string) error {
	locales, err := icusrc.OpenLocales(zip)
	if err != nil {
		return err
	}
	defer locales.Close()
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}
	// The locales share one pool, written beside them. EachLocale visits
	// the bundles in order, so the pool is the same every run.
	pool, err := datawrite.NewPool(reltimedata.Version, seed)
	if err != nil {
		return err
	}
	built, err := icusrc.EachLocale(locales, fb, func(v *icusrc.View) ([]byte, error) {
		l, err := read(v)
		if err != nil || l == nil {
			return nil, err
		}
		return reltimedata.Encode(l, pool), nil
	})
	if err != nil {
		return err
	}

	set := writeset.New()
	if err := datawrite.AddLocales(set, filepath.Join("data", "reltime"), built); err != nil {
		return err
	}
	set.File(filepath.Join("data", "reltimeshared.bin"), pool.Bytes())
	if err := set.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "reltimegen: %d locales\n", len(built))
	return nil
}

func read(v *icusrc.View) (*reltimedata.Built, error) {
	var out reltimedata.Built
	any := false
	for unit := 0; unit < reltimedata.Units; unit++ {
		for width := 0; width < reltimedata.Widths; width++ {
			field, err := v.Get("fields", reltimedata.Names[unit]+reltimedata.WidthNames[width])
			if err != nil {
				return nil, err
			}
			if field == nil {
				continue
			}
			target := &out.Fields[unit*reltimedata.Widths+width]
			if rel := field.Get("relative"); rel != nil {
				for _, n := range rel.Children {
					offset, err := strconv.Atoi(n.Key)
					if err != nil || n.Value == "" {
						continue
					}
					target.Named = append(target.Named, reltimedata.Named{Offset: offset, Text: n.Value})
				}
			}
			target.Future = counted(field.Get("relativeTime", "future"))
			target.Past = counted(field.Get("relativeTime", "past"))
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

// counted is a table of patterns by plural category.
func counted(t *icutxt.Node) []reltimedata.CountedText {
	if t == nil {
		return nil
	}
	out := make([]reltimedata.CountedText, 0, len(t.Children))
	for _, n := range t.Children {
		if n.Value == "" {
			continue
		}
		out = append(out, reltimedata.CountedText{Count: n.Key, Text: n.Value})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count < out[j].Count })
	return out
}
