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
