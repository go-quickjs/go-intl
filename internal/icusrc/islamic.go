package icusrc

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

// islamcal.cpp.txt is ICU 78.3's source/i18n/islamcal.cpp, vendored from
// icu4c-78.3-sources.tgz (renamed, since Go refuses C++ files in a package
// without cgo). The Umm al-Qura calendar is defined by the tables
// in it, which are not in the data archive: which months of each year from
// 1300 to 1600 AH have thirty days, and the corrections to the linear fit
// ICU finds each year's first day by.
//
//go:embed islamcal.cpp.txt
var islamcalSource string

// UmmAlQura is the Umm al-Qura calendar's definition, as islamcal.cpp holds
// it.
type UmmAlQura struct {
	// First and Last are the years the tables cover.
	First, Last int
	// Months has one mask per year, the bit 1<<(11-m) set when month m,
	// from zero, has thirty days.
	Months []int
	// Fixes corrects each year's start as the fit estimates it.
	Fixes []int
}

// ReadUmmAlQura reads the Umm al-Qura tables.
func ReadUmmAlQura() (UmmAlQura, error) {
	var u UmmAlQura
	var err error
	if u.First, err = cConstant("UMALQURA_YEAR_START"); err != nil {
		return u, err
	}
	if u.Last, err = cConstant("UMALQURA_YEAR_END"); err != nil {
		return u, err
	}
	if u.Months, err = cInts("UMALQURA_MONTHLENGTH[] = {"); err != nil {
		return u, err
	}
	if u.Fixes, err = cInts("umAlQuraYrStartEstimateFix[] = {"); err != nil {
		return u, err
	}
	if n := u.Last - u.First + 1; len(u.Months) != n || len(u.Fixes) != n {
		return u, fmt.Errorf("islamcal.cpp: %d years, %d masks, %d fixes", n, len(u.Months), len(u.Fixes))
	}
	return u, nil
}

// cConstant reads "static const int32_t NAME = 1300;".
func cConstant(name string) (int, error) {
	start := strings.Index(islamcalSource, name+" = ")
	if start < 0 {
		return 0, fmt.Errorf("islamcal.cpp has no %s", name)
	}
	body := islamcalSource[start+len(name)+3:]
	return strconv.Atoi(strings.TrimSpace(body[:strings.Index(body, ";")]))
}

// cInts reads an array of numbers, decimal or hex, leaving out comments.
func cInts(opening string) ([]int, error) {
	start := strings.Index(islamcalSource, opening)
	if start < 0 {
		return nil, fmt.Errorf("islamcal.cpp has no %q", opening)
	}
	// The comments are left out first: one of them holds a "};" of its own.
	var body strings.Builder
	for _, line := range strings.Split(islamcalSource[start+len(opening):], "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		body.WriteString(line)
		body.WriteString("\n")
	}
	end := strings.Index(body.String(), "}")
	if end < 0 {
		return nil, fmt.Errorf("islamcal.cpp: %q has no end", opening)
	}
	var out []int
	for _, line := range strings.Split(body.String()[:end], "\n") {
		for _, field := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\r' }) {
			n, err := strconv.ParseInt(field, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("islamcal.cpp: %q in %q", field, opening)
			}
			out = append(out, int(n))
		}
	}
	return out, nil
}
