package icusrc

import (
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// localefallback_data.h is ICU 78.3's source/common/localefallback_data.h,
// vendored from icu4c-78.3-sources.tgz. It holds the two tables ICU's
// resource fallback consults for a bundle that does not exist: CLDR's parent
// locales and each language's default script. The default scripts are not
// the likely subtags: the table has "sr" written in Cyrillic and nothing for
// "sr_ME", so ICU takes Montenegro's Serbian to be Cyrillic when it falls
// back, though its likely script is Latin.
//
//go:embed localefallback_data.h
var localeFallbackData string

// ICUFallback returns the fallback ICU's resource bundles use, from its own
// tables.
func ICUFallback() (Fallback, error) {
	scripts, err := cStrings("scriptCodeChars")
	if err != nil {
		return Fallback{}, err
	}
	ids, err := cStrings("dsLocaleIDChars")
	if err != nil {
		return Fallback{}, err
	}
	defaults, err := pairs("defaultScriptTable", ids, scripts)
	if err != nil {
		return Fallback{}, err
	}
	parentChars, err := cStrings("parentLocaleChars")
	if err != nil {
		return Fallback{}, err
	}
	parents, err := pairs("parentLocaleTable", parentChars, parentChars)
	if err != nil {
		return Fallback{}, err
	}
	return Fallback{
		Parent: func(name string) (string, bool) {
			p, ok := parents[name]
			return p, ok
		},
		// getDefaultScript: the language and region, then the language,
		// then Latin.
		DefaultScript: func(language, region string) string {
			if region != "" {
				if s, ok := defaults[language+"_"+region]; ok {
					return s
				}
			}
			if s, ok := defaults[language]; ok {
				return s
			}
			return "Latn"
		},
	}, nil
}

// cStrings reads a char array made of adjacent string literals, each holding
// NUL-separated strings, as a map from offset to string.
func cStrings(name string) (map[int]string, error) {
	start := strings.Index(localeFallbackData, "const char "+name+"[] =")
	if start < 0 {
		return nil, fmt.Errorf("localefallback_data.h has no %s", name)
	}
	body := localeFallbackData[start:]
	body = body[:strings.Index(body, ";")]
	var all strings.Builder
	for _, m := range regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(body, -1) {
		all.WriteString(strings.ReplaceAll(m[1], `\0`, "\x00"))
	}
	out := map[int]string{}
	offset := 0
	for _, s := range strings.Split(all.String(), "\x00") {
		out[offset] = s
		offset += len(s) + 1
	}
	return out, nil
}

// pairs reads an int32_t table of offset pairs into a map from key to value.
func pairs(name string, keys, values map[int]string) (map[string]string, error) {
	start := strings.Index(localeFallbackData, "const int32_t "+name+"[] = {")
	if start < 0 {
		return nil, fmt.Errorf("localefallback_data.h has no %s", name)
	}
	body := localeFallbackData[start:]
	body = body[strings.Index(body, "{")+1 : strings.Index(body, "};")]
	out := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\r' })
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s: a line of %d numbers", name, len(fields))
		}
		k, err1 := strconv.Atoi(fields[0])
		v, err2 := strconv.Atoi(fields[1])
		key, ok1 := keys[k]
		value, ok2 := values[v]
		if err1 != nil || err2 != nil || !ok1 || !ok2 {
			return nil, fmt.Errorf("%s: a bad pair %q", name, line)
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	return out, nil
}
