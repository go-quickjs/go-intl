package icusrc

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

// ucurr.cpp.txt is ICU 78.3's source/common/ucurr.cpp, vendored from
// icu4c-78.3-sources.tgz and renamed as islamcal.cpp is. The list of ISO
// currencies ucurr_openISOCurrencies enumerates, each with whether it is
// common and whether it is deprecated, is compiled into it (gCurrencyList)
// rather than kept in the data archive.
//
//go:embed ucurr.cpp.txt
var ucurrSource string

// A Currency is one entry of ICU's list: its code and its flags,
// "UCURR_COMMON" and the rest.
type Currency struct {
	Code  string
	Flags []string
}

var currencyEntry = regexp.MustCompile(`\{"([A-Z]{3})", ([A-Z_|]+)\}`)

// CurrencyList reads gCurrencyList.
func CurrencyList() ([]Currency, error) {
	start := strings.Index(ucurrSource, "} gCurrencyList[] = {")
	if start < 0 {
		return nil, fmt.Errorf("ucurr.cpp has no gCurrencyList")
	}
	body := ucurrSource[start:]
	end := strings.Index(body, "{ nullptr, 0 }")
	if end < 0 {
		return nil, fmt.Errorf("ucurr.cpp: gCurrencyList has no end")
	}
	var out []Currency
	for _, m := range currencyEntry.FindAllStringSubmatch(body[:end], -1) {
		out = append(out, Currency{Code: m[1], Flags: strings.Split(m[2], "|")})
	}
	if len(out) < 100 {
		return nil, fmt.Errorf("ucurr.cpp: gCurrencyList has only %d entries", len(out))
	}
	return out, nil
}

// uscript_props.cpp.txt is ICU 78.3's source/common/uscript_props.cpp,
// vendored likewise: each script's properties, which say whether it is
// written right to left, are compiled into it (SCRIPT_PROPS).
//
//go:embed uscript_props.cpp.txt
var uscriptPropsSource string

var scriptProps = regexp.MustCompile(`(?m)^\s*0x[0-9A-Fa-f]+ \|([^/\n]*)// ([A-Z][a-z]{3})\s*$`)

// RightToLeftScripts lists the scripts SCRIPT_PROPS marks RTL, by their
// four-letter codes.
func RightToLeftScripts() ([]string, error) {
	var out []string
	for _, m := range scriptProps.FindAllStringSubmatch(uscriptPropsSource, -1) {
		if strings.Contains(m[1], "RTL") {
			out = append(out, m[2])
		}
	}
	if len(out) < 20 {
		return nil, fmt.Errorf("uscript_props.cpp: only %d right-to-left scripts", len(out))
	}
	return out, nil
}
