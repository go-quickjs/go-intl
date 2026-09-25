// Command temporalgen writes the tables Temporal's calendars read, as Node's
// Temporal has them: ICU4X's icu_calendar 2.2.1, the version Node 26.10.0
// builds Temporal with (see SOURCES.md).
//
//	go run ./internal/temporalgen <icu_calendar-2.2.1.crate>
//
// The crate is checked against crates.io's checksum before it is read. Four
// of its source files hold data rather than code, and they are copied here
// as they stand:
//
//   - east_asian_traditional/china_data.rs, the Chinese calendar's years
//     from 1912 to 2102;
//   - east_asian_traditional/korea_data.rs, the Korean calendar's;
//   - east_asian_traditional/qing_data.rs, both calendars' years from 1900
//     to 1911;
//   - hijri/ummalqura_data.rs, the Umm al-Qura calendar's years from AH
//     1300 to 1600.
//
// data/temporalcalendars.bin has a line per year:
//
//	china 1912 lslsslssllsls 0 1912-02-18
//	ummalqura 1300 lslslslslsls 0 1882-11-12
//
// the table, the year (the related ISO year for the East Asian calendars,
// the year of the Hijra for Umm al-Qura), each month long ("l", 30 days) or
// short ("s", 29), the ordinal of the leap month (0 for none), and the ISO
// date of the year's first day. Outside the tables ICU4X reckons the years
// itself, and so does the temporal package.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// crateSHA256 is crates.io's checksum of icu_calendar 2.2.1.
const crateSHA256 = "a2b2acc6263f494f1df50685b53ff8e57869e47d5c6fe39c23d518ae9a4f3e45"

// tables are the crate's data files, by the name each table is written
// under, in the order they are written.
var tables = []struct{ name, path string }{
	{"china", "icu_calendar-2.2.1/src/cal/east_asian_traditional/china_data.rs"},
	{"korea", "icu_calendar-2.2.1/src/cal/east_asian_traditional/korea_data.rs"},
	{"qing", "icu_calendar-2.2.1/src/cal/east_asian_traditional/qing_data.rs"},
	{"ummalqura", "icu_calendar-2.2.1/src/cal/hijri/ummalqura_data.rs"},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/temporalgen <icu_calendar-2.2.1.crate>")
		os.Exit(2)
	}
	out, err := build(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "temporalgen:", err)
		os.Exit(1)
	}
	target := filepath.Join("data", "temporalcalendars.bin")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "temporalgen:", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, target); err != nil {
		fmt.Fprintln(os.Stderr, "temporalgen:", err)
		os.Exit(1)
	}
}

func build(crate string) ([]byte, error) {
	b, err := os.ReadFile(crate)
	if err != nil {
		return nil, err
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != crateSHA256 {
		return nil, fmt.Errorf("%s has checksum %s, want %s", crate, got, crateSHA256)
	}
	files, err := untar(b)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("# icu_calendar 2.2.1\n")
	for _, t := range tables {
		src, ok := files[t.path]
		if !ok {
			return nil, fmt.Errorf("the crate has no %s", t.path)
		}
		n, err := writeTable(&out, t.name, src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.path, err)
		}
		if n == 0 {
			return nil, fmt.Errorf("%s: no years", t.path)
		}
	}
	return out.Bytes(), nil
}

func untar(crate []byte) (map[string]string, error) {
	z, err := gzip.NewReader(bytes.NewReader(crate))
	if err != nil {
		return nil, err
	}
	r := tar.NewReader(z)
	files := map[string]string{}
	for {
		h, err := r.Next()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		for _, t := range tables {
			if h.Name == t.path {
				b, err := io.ReadAll(r)
				if err != nil {
					return nil, err
				}
				files[h.Name] = string(b)
			}
		}
	}
}

var (
	// PackedEastAsianTraditionalYearData::new(1912, [l, s, ...], None, gregorian(1912, 2, 18)),
	eastAsianYear = regexp.MustCompile(`PackedEastAsianTraditionalYearData::new\((-?\d+), \[([ls, ]+)\], (None|Some\((\d+)\)), gregorian\((-?\d+), (\d+), (\d+)\)\)`)
	// PackedHijriYearData::try_new(1300, [l, s, ...], gregorian(1882, 11, 12)).unwrap(),
	hijriYear = regexp.MustCompile(`PackedHijriYearData::try_new\((-?\d+), \[([ls, ]+)\], gregorian\((-?\d+), (\d+), (\d+)\)\)`)
	// pub const STARTING_YEAR: i32 = 1912;
	startingYear = regexp.MustCompile(`pub const STARTING_YEAR: i32 = (-?\d+);`)
)

// writeTable writes a data file's years, checking that they run on from
// its starting year without a gap.
func writeTable(out *bytes.Buffer, name, src string) (int, error) {
	m := startingYear.FindStringSubmatch(src)
	if m == nil {
		return 0, fmt.Errorf("no STARTING_YEAR")
	}
	next, _ := strconv.Atoi(m[1])
	n := 0
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") {
			continue
		}
		var year, leap, isoY, isoM, isoD string
		var lengths string
		if e := eastAsianYear.FindStringSubmatch(line); e != nil {
			year, lengths, isoY, isoM, isoD = e[1], e[2], e[5], e[6], e[7]
			leap = "0"
			if e[4] != "" {
				leap = e[4]
			}
		} else if h := hijriYear.FindStringSubmatch(line); h != nil {
			year, lengths, isoY, isoM, isoD = h[1], h[2], h[3], h[4], h[5]
			leap = "0"
		} else {
			continue
		}
		y, _ := strconv.Atoi(year)
		if y != next {
			return n, fmt.Errorf("year %d where %d was due", y, next)
		}
		next++
		months := strings.ReplaceAll(strings.ReplaceAll(lengths, ",", ""), " ", "")
		mm, _ := strconv.Atoi(isoM)
		dd, _ := strconv.Atoi(isoD)
		fmt.Fprintf(out, "%s %s %s %s %s-%02d-%02d\n", name, year, months, leap, isoY, mm, dd)
		n++
	}
	return n, nil
}
