package intl_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/internal/corpus"
)

func TestDateTimeFormatMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "DateTimeFormat" || c.Method != "format" {
			continue
		}
		opts, err := dateTimeOptions(c.Options)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		millis, ok := c.Number()
		if !ok {
			t.Errorf("%s: no instant", c.Source)
			continue
		}

		ran++
		dtf, err := intl.NewDateTimeFormat(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		got := dtf.Format(time.UnixMilli(int64(millis)).UTC())
		if got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
				c.Source, got, c.Want))
		}
	}

	if ran == 0 {
		t.Fatal("no DateTimeFormat cases ran")
	}
	t.Logf("%d of %d DateTimeFormat cases match ICU exactly (%.2f%%)",
		matched, ran, 100*float64(matched)/float64(ran))
	// What is not done yet is named rather than hidden, and the count is
	// checked too, so that finishing one of these or breaking something else
	// is noticed. Both are stated in PLAN.md.
	//
	//	zh-Hant  uses the flexible day period -- the small hours are 凌晨
	//	         rather than 上午 -- and joins a date to a time with a space
	//	         that CLDR's own glue, "{1}{0}", does not contain.
	const outstanding = 11
	var unexpected []string
	for _, d := range differences {
		if strings.Contains(d, `DateTimeFormat("zh-Hant"`) {
			continue
		}
		unexpected = append(unexpected, d)
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		shown := unexpected
		if len(shown) > 20 {
			shown = shown[:20]
		}
		t.Errorf("%d cases differ for reasons that are not known:\n%s",
			len(unexpected), strings.Join(shown, "\n"))
	}
	switch got := ran - matched; {
	case got > outstanding:
		t.Errorf("%d cases differ, which is more than the %d outstanding",
			got, outstanding)
	case got < outstanding:
		t.Errorf("only %d cases differ where %d were outstanding; something "+
			"was fixed and the count needs lowering", got, outstanding)
	}
}

func dateTimeOptions(in map[string]any) (intl.DateTimeFormatOptions, error) {
	var out intl.DateTimeFormatOptions
	length := func(v any) (intl.DateTimeLength, error) {
		switch v {
		case "full":
			return intl.LengthFull, nil
		case "long":
			return intl.LengthLong, nil
		case "medium":
			return intl.LengthMedium, nil
		case "short":
			return intl.LengthShort, nil
		}
		return 0, fmt.Errorf("length %v", v)
	}
	width := func(v any) (intl.FieldWidth, error) {
		switch v {
		case "numeric":
			return intl.WidthNumeric, nil
		case "2-digit":
			return intl.Width2Digit, nil
		case "long":
			return intl.WidthLong, nil
		case "short":
			return intl.WidthShort, nil
		case "narrow":
			return intl.WidthNarrow, nil
		}
		return 0, fmt.Errorf("width %v", v)
	}

	var err error
	for key, value := range in {
		switch key {
		case "timeZone":
			out.TimeZone, _ = value.(string)
		case "dateStyle":
			out.DateStyle, err = length(value)
		case "timeStyle":
			out.TimeStyle, err = length(value)
		case "weekday":
			out.Weekday, err = width(value)
		case "era":
			out.Era, err = width(value)
		case "year":
			out.Year, err = width(value)
		case "month":
			out.Month, err = width(value)
		case "day":
			out.Day, err = width(value)
		case "hour":
			out.Hour, err = width(value)
		case "minute":
			out.Minute, err = width(value)
		case "second":
			out.Second, err = width(value)
		case "timeZoneName":
			out.TimeZoneName, err = width(value)
		case "hour12":
			if on, ok := value.(bool); ok {
				out.Hour12 = intl.Bool(on)
			}
		default:
			return out, fmt.Errorf("unknown option %q", key)
		}
		if err != nil {
			return out, fmt.Errorf("%s: %w", key, err)
		}
	}
	return out, nil
}
