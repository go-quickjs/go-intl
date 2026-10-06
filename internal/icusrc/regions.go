package icusrc

import (
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// loclikely.cpp.txt is ICU 78.3's source/common/loclikely.cpp, vendored
// from icu4c-78.3-sources.tgz and renamed as islamcal.cpp is. The regions
// a "-u-rg-" or "-u-sd-" keyword may name are compiled into it
// (gValidRegionMap) rather than kept in the data archive.
//
//go:embed loclikely.cpp.txt
var loclikelySource string

var hexWord = regexp.MustCompile(`0x[0-9a-fA-F]{8}`)

// ValidRegions reads RegionValidateMap's bitmap: a bit for each pair of
// letters, "AA" first and "ZZ" last, set for a region ICU takes from a
// keyword. They are returned sorted.
func ValidRegions() ([]string, error) {
	start := strings.Index(loclikelySource, "gValidRegionMap[] = {")
	if start < 0 {
		return nil, fmt.Errorf("loclikely.cpp has no gValidRegionMap")
	}
	body := loclikelySource[start:]
	end := strings.Index(body, "};")
	if end < 0 {
		return nil, fmt.Errorf("loclikely.cpp: gValidRegionMap has no end")
	}
	words := hexWord.FindAllString(body[:end], -1)
	if len(words) != 22 {
		return nil, fmt.Errorf("loclikely.cpp: gValidRegionMap has %d words, not 22", len(words))
	}
	var out []string
	for i := 0; i < 26*26; i++ {
		w, err := strconv.ParseUint(words[i/32][2:], 16, 32)
		if err != nil {
			return nil, err
		}
		if w>>(i%32)&1 != 0 {
			out = append(out, string([]byte{byte('A' + i/26), byte('A' + i%26)}))
		}
	}
	if len(out) < 200 {
		return nil, fmt.Errorf("loclikely.cpp: gValidRegionMap has only %d regions", len(out))
	}
	return out, nil
}
