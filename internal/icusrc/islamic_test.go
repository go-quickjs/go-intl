package icusrc

import "testing"

func TestReadUmmAlQura(t *testing.T) {
	u, err := ReadUmmAlQura()
	if err != nil {
		t.Fatal(err)
	}
	if u.First != 1300 || u.Last != 1600 {
		t.Errorf("years %d to %d, want 1300 to 1600", u.First, u.Last)
	}
	// The first and last masks and fixes, as islamcal.cpp writes them.
	if u.Months[0] != 0x0AAA || u.Months[len(u.Months)-1] != 0x029D {
		t.Errorf("masks %#x ... %#x", u.Months[0], u.Months[len(u.Months)-1])
	}
	if u.Fixes[2] != -1 || u.Fixes[20] != 1 || u.Fixes[len(u.Fixes)-1] != 1 {
		t.Errorf("fixes %d %d ... %d", u.Fixes[2], u.Fixes[20], u.Fixes[len(u.Fixes)-1])
	}
}
