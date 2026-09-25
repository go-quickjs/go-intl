package icusrc

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

// uloc_tag.cpp.txt is ICU 78.3's source/common/uloc_tag.cpp, vendored from
// icu4c-78.3-sources.tgz and renamed as islamcal.cpp is. Its parser rewrites
// two lists of whole tags before anything else, and they are compiled into
// the parser rather than kept in the data archive: BCP 47's legacy
// ("grandfathered") tags, "art-lojban" to "jbo", and its redundant ones,
// "sgn-no" to "nsl". ICU matches each as a prefix of the tag.
//
//go:embed uloc_tag.cpp.txt
var ulocTagSource string

// LegacyTags reads the two lists, each a list of tag and replacement.
func LegacyTags() (legacy, redundant [][2]string, err error) {
	if legacy, err = stringPairs("LEGACY"); err != nil {
		return nil, nil, err
	}
	if redundant, err = stringPairs("REDUNDANT"); err != nil {
		return nil, nil, err
	}
	return legacy, redundant, nil
}

var cString = regexp.MustCompile(`"([^"]*)"`)

// stringPairs reads "constexpr const char* NAME[] = { "a", "b", ... };",
// leaving out the comments.
func stringPairs(name string) ([][2]string, error) {
	start := strings.Index(ulocTagSource, "constexpr const char* "+name+"[] = {")
	if start < 0 {
		return nil, fmt.Errorf("uloc_tag.cpp has no %s", name)
	}
	var body strings.Builder
	for _, line := range strings.Split(ulocTagSource[start:], "\n")[1:] {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		if strings.HasPrefix(strings.TrimSpace(line), "};") {
			break
		}
		body.WriteString(line)
		body.WriteString("\n")
	}
	var values []string
	for _, m := range cString.FindAllStringSubmatch(body.String(), -1) {
		values = append(values, m[1])
	}
	if len(values) == 0 || len(values)%2 != 0 {
		return nil, fmt.Errorf("uloc_tag.cpp: %s has %d strings", name, len(values))
	}
	out := make([][2]string, 0, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		out = append(out, [2]string{values[i], values[i+1]})
	}
	return out, nil
}
