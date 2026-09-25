package intl_test

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// The arithmetic calendars across their era boundaries, where the sweep's
// modern dates do not reach: before Diocletian, the Incarnation and the
// Republic of China, and far before the Hijra. The expectations are Node's;
// English has no name for the Coptic era before Diocletian, so none is
// written.
func TestArithmeticCalendars(t *testing.T) {
	dates := []time.Time{
		time.Date(200, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1905, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1911, 12, 31, 0, 0, 0, 0, time.UTC),
		time.Date(1912, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(-600, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for calendar, want := range map[string][]string{
		"coptic":        {"10/7/85 ", "4/23/1621 AM", "4/21/1628 AM", "4/22/1628 AM", "5/12/885 "},
		"ethiopic":      {"10/7/192 AM", "4/23/1897 AM", "4/21/1904 AM", "4/22/1904 AM", "5/12/4892 AA"},
		"ethioaa":       {"10/7/5692 AA", "4/23/7397 AA", "4/21/7404 AA", "4/22/7404 AA", "5/12/4892 AA"},
		"indian":        {"3/11/122 Śaka", "10/11/1826 Śaka", "10/10/1833 Śaka", "10/11/1833 Śaka", "10/11/-679 Śaka"},
		"islamic-civil": {"11/30/-435 AH", "10/24/1322 AH", "1/10/1330 AH", "1/11/1330 AH", "12/7/-1260 AH"},
		"islamic-tbla":  {"12/1/-435 AH", "10/25/1322 AH", "1/11/1330 AH", "1/12/1330 AH", "12/8/-1260 AH"},
		"roc":           {"6/1/1712 B.R.O.C.", "1/1/7 B.R.O.C.", "12/31/1 B.R.O.C.", "1/1/1 Minguo", "1/8/2512 B.R.O.C."},
	} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{
			Calendar: calendar, TimeZone: "UTC", Era: intl.WidthShort,
			Year: intl.WidthNumeric, Month: intl.WidthNumeric, Day: intl.WidthNumeric,
		})
		for i, d := range dates {
			if got := f.Format(d); got != want[i] {
				t.Errorf("%s %s = %q, want %q", calendar, d.Format("2006-01-02"), got, want[i])
			}
		}
	}
}
