package icusrc

import "testing"

func TestLegacyTags(t *testing.T) {
	legacy, redundant, err := LegacyTags()
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 25 || legacy[0] != [2]string{"art-lojban", "jbo"} ||
		legacy[24] != [2]string{"zh-min", "nan-x-zh-min"} {
		t.Errorf("legacy: %d, first %v, last %v", len(legacy), legacy[0], legacy[len(legacy)-1])
	}
	if len(redundant) != 26 || redundant[14] != [2]string{"sgn-no", "nsl"} ||
		redundant[25] != [2]string{"ja-latn-hepburn-heploc", "ja-latn-alalc97"} {
		t.Errorf("redundant: %d, %v, last %v", len(redundant), redundant[14], redundant[len(redundant)-1])
	}
}

func TestCurrencyList(t *testing.T) {
	list, err := CurrencyList()
	if err != nil {
		t.Fatal(err)
	}
	common := 0
	for _, c := range list {
		if len(c.Flags) == 2 && c.Flags[0] == "UCURR_COMMON" && c.Flags[1] == "UCURR_NON_DEPRECATED" {
			common++
		}
	}
	if list[0].Code != "ADP" || common != 159 {
		t.Errorf("first %s, %d common and current", list[0].Code, common)
	}
}
