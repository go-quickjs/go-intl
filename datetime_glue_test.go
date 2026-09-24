package intl_test

import (
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// Traditional Chinese joins a short date to a time with a thin space and a
// longer one with a plain space, as CLDR says and ICU writes. The thin space
// was once flattened to a plain one in every glue.
func TestDateTimeGlueKeepsThinSpace(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		date intl.DateTimeLength
		want string
	}{
		{intl.LengthShort, "2024/1/5 下午3:04"},
		{intl.LengthMedium, "2024年1月5日 下午3:04"},
	} {
		f := newDateTime(t, "zh-Hant", intl.DateTimeFormatOptions{
			TimeZone: "UTC", DateStyle: c.date, TimeStyle: intl.LengthShort,
		})
		if got := f.Format(when); got != c.want {
			t.Errorf("zh-Hant date style %d = %+q, want %+q", c.date, got, c.want)
		}
	}
}
