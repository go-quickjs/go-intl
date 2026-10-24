// Command listgen writes the list-pattern tables the intl package carries.
//
// It reads ICU's locale bundles, data/locales/*.txt in the data archive
// SOURCES.md pins, each locale's listPattern as ICU resolves it.
//
//	go run ./internal/listgen icu4c-78.3-data.zip
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icusrc"
	"github.com/go-quickjs/go-intl/internal/listdata"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

// names are ICU's keys for each set, in the order listdata stores them.
var names = [listdata.Count]string{
	"standard",
	"standard-short",
	"standard-narrow",
	"or",
	"or-short",
	"or-narrow",
	"unit",
	"unit-short",
	"unit-narrow",
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/listgen <icu4c-78.3-data.zip>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "listgen:", err)
		os.Exit(1)
	}
}

func run(zip string) error {
	locales, err := icusrc.OpenLocales(zip)
	if err != nil {
		return err
	}
	defer locales.Close()
	fb, err := icusrc.ICUFallback()
	if err != nil {
		return err
	}
	built, err := icusrc.EachLocale(locales, fb, func(v *icusrc.View) ([]byte, error) {
		l, err := read(v)
		if err != nil || l == nil {
			return nil, err
		}
		return listdata.Encode(l), nil
	})
	if err != nil {
		return err
	}

	set := writeset.New()
	if err := datawrite.AddLocales(set, filepath.Join("data", "lists"), built); err != nil {
		return err
	}
	if err := set.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "listgen: %d locales\n", len(built))
	return nil
}

func read(v *icusrc.View) (*listdata.Locale, error) {
	var out listdata.Locale
	any := false
	for i, key := range names {
		p, err := v.Get("listPattern", key)
		if err != nil {
			return nil, err
		}
		if p == nil {
			continue
		}
		s := func(k string) string {
			if n := p.Get(k); n != nil {
				return n.Value
			}
			return ""
		}
		out.Sets[i] = listdata.Patterns{Two: s("2"), Start: s("start"), Middle: s("middle"), End: s("end")}
		any = true
	}
	if !any {
		return nil, nil
	}
	return &out, nil
}
