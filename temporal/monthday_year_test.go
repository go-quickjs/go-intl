package temporal

import (
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// What each side of MonthDayYearClamp makes of a year past an int32 in the
// ISO calendar, as Node and Chrome 158 answer, and test262's
// iso-year-used-only-for-overflow.
func TestMonthDayYearClamp(t *testing.T) {
	const leap, common = 1000000000000004.0, 1000000000000005.0
	fields := func(year float64) *jsObject {
		return newObject("year", year, "monthCode", "M02", "day", 29.0)
	}
	reject := func() *jsObject { return newObject("overflow", "reject") }
	const outOfRange = "Temporal error: Integer out of range."
	for _, c := range []struct {
		name           string
		op             string
		recv           any
		args           []any
		standard, node string
	}{
		{"a leap year", "static:PlainMonthDay.from", nil, []any{fields(leap)}, "02-29", "02-28"},
		{"a common year", "static:PlainMonthDay.from", nil, []any{fields(common)}, "02-28", "02-28"},
		{"a leap year, reject", "static:PlainMonthDay.from", nil, []any{fields(leap), reject()}, "02-29", outOfRange},
		{"a common year, reject", "static:PlainMonthDay.from", nil, []any{fields(common), reject()},
			"Temporal error: day value is not in a valid range.", outOfRange},
		{"a year before the first", "static:PlainMonthDay.from", nil, []any{fields(-leap)}, "02-29", "02-29"},
		{"with", "method:PlainMonthDay.with", "02-28", []any{newObject("year", leap, "day", 29.0)}, "02-29", "02-28"},
		// A year within an int32 is read as it was.
		{"an int32 year", "static:PlainMonthDay.from", nil, []any{fields(2021)}, "02-28", "02-28"},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.NodeICU, c.node}} {
			h, err := newHarness()
			if err != nil {
				t.Fatal(err)
			}
			h.compat = side.compat
			var recv any = jsNull{}
			if s, ok := c.recv.(string); ok {
				if recv, err = h.toPlainMonthDay(s, jsUndefined{}, "test"); err != nil {
					t.Fatal(err)
				}
			}
			got := ""
			switch x := h.run(c.op, recv, c.args).(type) {
			case map[string]any:
				if msg, ok := x["msg"].(string); ok {
					got = msg
				} else if s, ok := x["$PlainMonthDay"].(string); ok {
					got = s[len("1972-") : len("1972-")+len("02-29")]
				}
			}
			if got != side.want {
				t.Errorf("%s, %v: %q, want %q", c.name, side.compat, got, side.want)
			}
		}
	}
}
