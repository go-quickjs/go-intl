// Command namegen writes the display-name tables the intl package carries.
//
// It reads two CLDR packages, neither vendored: SOURCES.md pins both releases
// and their checksums, and the paths to unpacked copies are given here.
//
//	go run ./internal/namegen <cldr-localenames-full/package> <cldr-dates-full/package>
//
// CLDR writes the shorter forms as alternates of the long one -- "GB" is
// "United Kingdom" and "GB-alt-short" is "UK" -- so the suffix is what sorts a
// name into its width. A width holds only what differs from the one above it,
// which is why a lookup walks back rather than each width being complete.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-quickjs/go-intl/internal/namedata"
)

type namesFile struct {
	Main map[string]struct {
		LocaleDisplayNames struct {
			Languages   map[string]string `json:"languages"`
			Territories map[string]string `json:"territories"`
			Scripts     map[string]string `json:"scripts"`
			Pattern     struct {
				Locale    string `json:"localePattern"`
				Separator string `json:"localeSeparator"`
			} `json:"localeDisplayPattern"`
			Types map[string]map[string]string `json:"types"`
		} `json:"localeDisplayNames"`
	} `json:"main"`
}

type fieldsFile struct {
	Main map[string]struct {
		Dates struct {
			Fields map[string]struct {
				DisplayName string `json:"displayName"`
			} `json:"fields"`
		} `json:"dates"`
	} `json:"main"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr,
			"usage: go run ./internal/namegen <cldr-localenames-full/package> <cldr-dates-full/package>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "namegen:", err)
		os.Exit(1)
	}
}

func run(namesRoot, datesRoot string) error {
	main := filepath.Join(namesRoot, "main")
	entries, err := os.ReadDir(main)
	if err != nil {
		return fmt.Errorf("reading %s: %w", main, err)
	}

	built := map[string][]byte{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		l, err := read(main, filepath.Join(datesRoot, "main"), e.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if l == nil {
			continue
		}
		built[e.Name()] = namedata.Encode(l)
	}
	if len(built) == 0 {
		return fmt.Errorf("no locales found under %s", main)
	}

	out := filepath.Join("data", "names")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(out, "*.bin"))
	for _, name := range old {
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(built))
	for name := range built {
		names = append(names, name)
	}
	sort.Strings(names)
	var total int
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(out, name+".bin"), built[name], 0o644); err != nil {
			return err
		}
		total += len(built[name])
	}
	fmt.Fprintf(os.Stderr, "namegen: %d locales, %.1f MB\n",
		len(names), float64(total)/(1<<20))
	return nil
}

// alternates maps CLDR's alternate suffix to the width it belongs to.
var alternates = map[string]int{
	"":        namedata.Long,
	"short":   namedata.Short,
	"narrow":  namedata.Narrow,
	"variant": namedata.Long,
	"menu":    namedata.Long,
}

func read(main, datesMain, name string) (*namedata.Locale, error) {
	var out namedata.Locale
	any := false

	raw, err := os.ReadFile(filepath.Join(main, name, "languages.json"))
	if err == nil {
		var f namesFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("languages.json: %w", err)
		}
		if e, ok := f.Main[name]; ok {
			fill(&out, namedata.Language, e.LocaleDisplayNames.Languages)
			any = true
		}
	}
	for file, kind := range map[string]int{
		"territories.json": namedata.Region,
		"scripts.json":     namedata.Script,
	} {
		raw, err := os.ReadFile(filepath.Join(main, name, file))
		if err != nil {
			continue
		}
		var f namesFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		e, ok := f.Main[name]
		if !ok {
			continue
		}
		source := e.LocaleDisplayNames.Territories
		if kind == namedata.Script {
			source = e.LocaleDisplayNames.Scripts
		}
		fill(&out, kind, source)
		any = true
	}

	raw, err = os.ReadFile(filepath.Join(main, name, "localeDisplayNames.json"))
	if err == nil {
		var f namesFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("localeDisplayNames.json: %w", err)
		}
		if e, ok := f.Main[name]; ok {
			out.Pattern = e.LocaleDisplayNames.Pattern.Locale
			out.Separator = e.LocaleDisplayNames.Pattern.Separator
			if calendars, ok := e.LocaleDisplayNames.Types["calendar"]; ok {
				fill(&out, namedata.Calendar, withBCP47(calendars))
			}
			any = true
		}
	}

	raw, err = os.ReadFile(filepath.Join(datesMain, name, "dateFields.json"))
	if err == nil {
		var f fieldsFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("dateFields.json: %w", err)
		}
		if e, ok := f.Main[name]; ok {
			fields := map[string]string{}
			for key, field := range e.Dates.Fields {
				if field.DisplayName == "" {
					continue
				}
				// The widths are their own keys here rather than alternates.
				base, suffix := key, ""
				if at := strings.LastIndexByte(key, '-'); at >= 0 {
					if s := key[at+1:]; s == "short" || s == "narrow" {
						base, suffix = key[:at], s
					}
				}
				if suffix == "" {
					fields[base] = field.DisplayName
				} else {
					fields[base+"-alt-"+suffix] = field.DisplayName
				}
			}
			fill(&out, namedata.DateTimeField, fields)
			any = true
		}
	}

	if !any {
		return nil, nil
	}
	return &out, nil
}

// fill sorts CLDR's names into the widths, by the alternate suffix each key
// carries.
func fill(out *namedata.Locale, kind int, source map[string]string) {
	byWidth := make([]map[string]string, namedata.Widths)
	for i := range byWidth {
		byWidth[i] = map[string]string{}
	}
	for key, name := range source {
		if name == "" {
			continue
		}
		code, alt, found := strings.Cut(key, "-alt-")
		width := namedata.Long
		if found {
			w, known := alternates[alt]
			if !known {
				continue
			}
			width = w
		}
		// A long alternate only stands in where there is nothing already.
		if _, taken := byWidth[width][code]; taken && found {
			continue
		}
		byWidth[width][code] = name
	}
	for w := range byWidth {
		entries := make([]namedata.Entry, 0, len(byWidth[w]))
		for code, name := range byWidth[w] {
			// A width holds only what differs from the one above it.
			if w != namedata.Long && byWidth[namedata.Long][code] == name {
				continue
			}
			entries = append(entries, namedata.Entry{Code: code, Name: name})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Code < entries[j].Code })
		out.Sets[kind*namedata.Widths+w].Entries = entries
	}
}

// calendarAliases are the calendars BCP-47 spells differently from CLDR.
// ECMA-402 asks for "gregory" and CLDR files it under "gregorian".
var calendarAliases = map[string]string{
	"gregorian":           "gregory",
	"ethiopic-amete-alem": "ethioaa",
	"islamic-civil":       "islamicc",
}

// withBCP47 adds the spelling ECMA-402 uses beside CLDR's own, so that a
// lookup finds a calendar under either name.
func withBCP47(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+len(calendarAliases))
	for key, name := range in {
		out[key] = name
		base, alt, found := strings.Cut(key, "-alt-")
		alias, ok := calendarAliases[base]
		if !ok {
			continue
		}
		if found {
			out[alias+"-alt-"+alt] = name
		} else {
			out[alias] = name
		}
	}
	return out
}
