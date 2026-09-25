package temporal

// The lunisolar calendars, whose years have a thirteenth month: the Hebrew,
// and the Chinese and Korean, as icu_calendar 2.2.1 reckons them.

// The Hebrew calendar is arithmetic, and ICU4X's keviyah tables give the
// same years as Calendrical Calculations' rules, which these are.

// hebrewEpoch is the Hebrew calendar's epoch, 1 Tishri AM 1.
var hebrewEpoch = julianFixed(-3760, 10, 7)

func hebrewLeap(year int) bool { return floorMod(7*int64(year)+1, 19) < 7 }

// hebrewElapsedDays is hebrew_calendar_elapsed_days: the days from the
// epoch to the molad of Tishri, deferred for the day of the week.
func hebrewElapsedDays(year int) int64 {
	months := floorDiv(235*int64(year)-234, 19)
	parts := 12084 + 13753*months
	days := 29*months + floorDiv(parts, 25920)
	if floorMod(3*(days+1), 7) < 3 {
		days++
	}
	return days
}

// hebrewNewYear is the day Tishri 1 falls on, with the corrections for
// years of impossible length.
func hebrewNewYear(year int) int64 {
	ny0, ny1, ny2 := hebrewElapsedDays(year-1), hebrewElapsedDays(year), hebrewElapsedDays(year+1)
	correction := int64(0)
	switch {
	case ny2-ny1 == 356:
		correction = 2
	case ny1-ny0 == 382:
		correction = 1
	}
	return hebrewEpoch + ny1 + correction
}

type hebrewRules struct{}

func (hebrewRules) year(extended int) calYear {
	start := hebrewNewYear(extended)
	length := hebrewNewYear(extended+1) - start
	y := calYear{extended: extended, start: start, count: 12, leapYear: hebrewLeap(extended)}
	months := []uint8{30, 29, 30, 29, 30, 29, 30, 29, 30, 29, 30, 29}
	if length == 355 || length == 385 {
		// A long Heshvan.
		months[1] = 30
	}
	if length == 353 || length == 383 {
		// A short Kislev.
		months[2] = 29
	}
	if y.leapYear {
		// Adar I, of thirty days, before Adar.
		months = append(months[:5], append([]uint8{30}, months[5:]...)...)
		y.count = 13
		y.leap = 6
	}
	copy(y.lengths[:], months)
	return y
}

func (r hebrewRules) yearOfRD(rd int64) calYear {
	year := int(floorDiv((rd-hebrewEpoch)*98496, 35975351)) + 1
	for hebrewNewYear(year) > rd {
		year--
	}
	for hebrewNewYear(year+1) <= rd {
		year++
	}
	return r.year(year)
}

// ordinalFromMonth is Hebrew's: Adar I is M05L, which a common year
// constrains to Adar.
func (hebrewRules) ordinalFromMonth(y calYear, m month, constrain bool) (int, errMonth) {
	switch {
	case !m.leap && m.number >= 1 && m.number <= 12:
		if m.number >= 6 && y.leapYear {
			return m.number + 1, monthOK
		}
		return m.number, monthOK
	case m.leap && m.number == 5:
		if y.leapYear || constrain {
			return 6, monthOK
		}
		return 0, monthNotInYear
	}
	return 0, monthNotInCalendar
}

func (hebrewRules) monthFromOrdinal(y calYear, ordinal int) month {
	if y.leapYear && ordinal >= 6 {
		return month{number: ordinal - 1, leap: ordinal == 6}
	}
	return month{number: ordinal}
}

func (hebrewRules) eraYear(y calYear, month, day int) (string, int, bool) {
	return "am", y.extended, true
}

func (hebrewRules) extendedFromEra(era string, year int) (int, bool) { return year, era == "am" }

func (hebrewRules) referenceYear(m month, day int) (int, errReference) {
	switch {
	case !m.leap && m.number == 1:
		return 5733, referenceOK
	case !m.leap && (m.number == 2 || m.number == 3):
		if day <= 29 {
			return 5733, referenceOK
		}
		return 5732, referenceOK
	case !m.leap && m.number == 4:
		if day <= 26 {
			return 5733, referenceOK
		}
		return 5732, referenceOK
	case !m.leap && m.number >= 5 && m.number <= 12:
		return 5732, referenceOK
	case m.leap && m.number == 5:
		return 5730, referenceOK
	}
	return 0, referenceNotInCalendar
}

func (hebrewRules) minMonthsFrom(y calYear, years int) int { return 235 * years / 19 }

// The Chinese and Korean calendars: ICU4X's tables where they reach, and
// its mean-motion approximation, anchored in the Gregorian calendar,
// outside them.

// Durations in milliseconds, as simple.rs computes them in 128 bits and
// truncates.
const (
	msPerDay = 86400000

	utcPlus8 = msPerDay * 8 / 24
	utcPlus9 = msPerDay * 9 / 24
	// The reference time of the Qing calendar, UTC+(1397/180) hours.
	beijingOffset = msPerDay * 1397 / 180 / 24

	meanGregorianYear      = msPerDay * 146097 / 400
	meanGregorianSolarTerm = msPerDay * 146097 / 400 / 12
	// meanSynodicMonth is 86400000 × 29.5305888531 days, truncated.
	meanSynodicMonth = 2551442876
)

// A localMoment is a day and the milliseconds into it.
type localMoment struct {
	rd     int64
	millis int64
}

func (m localMoment) add(ms int64) localMoment {
	t := m.millis + ms
	return localMoment{rd: m.rd + floorDiv(t, msPerDay), millis: floorMod(t, msPerDay)}
}

// The solstice of 1999-12-22T07:44 and the new moon of 2000-01-06T18:14,
// in UTC, that the approximation counts from.
var (
	utcSolstice = localMoment{rd: gregorianFixed(1999, 12, 22), millis: (7*60 + 44) * 60 * 1000}
	utcNewMoon  = localMoment{rd: gregorianFixed(2000, 1, 6), millis: (18*60 + 14) * 60 * 1000}
)

// periodicOnOrBefore is the last moment base + n × period that falls on or
// before a day.
func periodicOnOrBefore(rd int64, base localMoment, period int64) localMoment {
	diff := (rd-base.rd)*msPerDay - base.millis
	n := floorDiv(diff+msPerDay-1, period)
	millis := base.rd*msPerDay + base.millis + n*period
	return localMoment{rd: floorDiv(millis, msPerDay), millis: floorMod(millis, msPerDay)}
}

// simpleEastAsianYear is EastAsianTraditionalYear::simple: the píngqì rule
// with mean solar terms and mean new moons, at an offset from UTC.
func simpleEastAsianYear(offset int64, related int) (lengths [13]bool, leap int, newYear int64) {
	majorTerm := periodicOnOrBefore(gregorianDayBeforeYear(related), utcSolstice.add(offset), meanGregorianYear)
	newMoon := periodicOnOrBefore(majorTerm.rd, utcNewMoon.add(offset), meanSynodicMonth)
	nextNewMoon := newMoon.add(meanSynodicMonth)
	term := -2
	hadLeap := false
	// The months before the year: the eleventh, the twelfth, and a leap
	// month after either.
	for term < 0 || nextNewMoon.rd <= majorTerm.rd && !hadLeap {
		if nextNewMoon.rd <= majorTerm.rd && !hadLeap {
			hadLeap = true
		} else {
			term++
			majorTerm = majorTerm.add(meanGregorianSolarTerm)
		}
		newMoon, nextNewMoon = nextNewMoon, nextNewMoon.add(meanSynodicMonth)
	}
	newYear = newMoon.rd
	for term < 12 || nextNewMoon.rd <= majorTerm.rd && !hadLeap {
		i := term
		if leap != 0 {
			i++
		}
		if i < len(lengths) {
			lengths[i] = nextNewMoon.rd-newMoon.rd == 30
		}
		if nextNewMoon.rd <= majorTerm.rd && !hadLeap {
			hadLeap = true
			leap = term + 1
		} else {
			term++
			majorTerm = majorTerm.add(meanGregorianSolarTerm)
		}
		newMoon, nextNewMoon = nextNewMoon, nextNewMoon.add(meanSynodicMonth)
	}
	return lengths, leap, newYear
}

// eastAsianRules are ICU4X's China and Korea rules: the country's table,
// the Qing table before it, and the approximation at the country's offset
// after the table and at Beijing's before the Qing.
type eastAsianRules struct {
	table, qing []tableYear
	offset      int64
	korean      bool
}

func (r eastAsianRules) year(related int) calYear {
	var lengths [13]bool
	var leap int
	var newYear int64
	if t, ok := lookup(r.table, related); ok {
		lengths, leap, newYear = t.lengthsLeap(), t.leap, t.start
	} else if related > r.table[0].year {
		lengths, leap, newYear = simpleEastAsianYear(r.offset, related)
	} else if t, ok := lookup(r.qing, related); ok {
		lengths, leap, newYear = t.lengthsLeap(), t.leap, t.start
	} else {
		lengths, leap, newYear = simpleEastAsianYear(beijingOffset, related)
	}
	return packEastAsianYear(related, lengths, leap, newYear)
}

func (t tableYear) lengthsLeap() (lengths [13]bool) {
	for i := range lengths {
		lengths[i] = t.long&(1<<i) != 0
	}
	return lengths
}

// packEastAsianYear is PackedEastAsianTraditionalYearData: the new year
// kept as six bits of days from 19 January, and a month length for the
// thirteenth month only in a leap year.
func packEastAsianYear(related int, lengths [13]bool, leap int, newYear int64) calYear {
	earliest := gregorianFixed(related, 1, 19)
	y := calYear{extended: related, start: earliest + (newYear-earliest)&0x3F, count: 12, leap: leap, leapYear: leap != 0}
	if leap != 0 {
		y.count = 13
	}
	for i := 0; i < 13; i++ {
		y.lengths[i] = 29
		if lengths[i] {
			y.lengths[i] = 30
		}
	}
	if leap == 0 {
		y.lengths[12] = 0
	}
	return y
}

// yearOfRD is Rules::year_containing_rd: the year of the day's ISO year, or
// the one before where the day comes before its new year.
func (r eastAsianRules) yearOfRD(rd int64) calYear {
	related := gregorianYearFromFixed(rd)
	y := r.year(related)
	if rd < y.start {
		y = r.year(related - 1)
	}
	return y
}

func (eastAsianRules) ordinalFromMonth(y calYear, m month, constrain bool) (int, errMonth) {
	if m.number < 1 || m.number > 12 {
		return 0, monthNotInCalendar
	}
	sentinel := 14
	if y.leap != 0 {
		sentinel = y.leap
	}
	if m.leap && m.number == sentinel-1 {
		return sentinel, monthOK
	}
	if m.leap && !constrain {
		return 0, monthNotInYear
	}
	if m.number >= sentinel {
		return m.number + 1, monthOK
	}
	return m.number, monthOK
}

func (eastAsianRules) monthFromOrdinal(y calYear, ordinal int) month {
	leap := 14
	if y.leap != 0 {
		leap = y.leap
	}
	if ordinal >= leap {
		return month{number: ordinal - 1, leap: ordinal == leap}
	}
	return month{number: ordinal}
}

// The Chinese and Korean calendars have no eras in Temporal.
func (eastAsianRules) eraYear(y calYear, month, day int) (string, int, bool) { return "", 0, false }
func (eastAsianRules) extendedFromEra(era string, year int) (int, bool)      { return 0, false }

func (eastAsianRules) minMonthsFrom(y calYear, years int) int { return 12*years + years/3 }

// referenceYear is ecma_reference_year_common.
func (r eastAsianRules) referenceYear(m month, day int) (int, errReference) {
	long := day > 29
	regular := func() (int, errReference) { return 0, referenceUseRegularIfConstrain }
	switch {
	case m.number < 1 || m.number > 12:
		return 0, referenceNotInCalendar
	case !m.leap:
		switch m.number {
		case 1:
			if long {
				return 1970, referenceOK
			}
			return 1972, referenceOK
		case 2, 5, 7, 9, 10:
			return 1972, referenceOK
		case 3:
			if !long {
				return 1972, referenceOK
			}
			if r.korean {
				return 1968, referenceOK
			}
			return 1966, referenceOK
		case 4:
			if long {
				return 1970, referenceOK
			}
			return 1972, referenceOK
		case 6, 8:
			if long {
				return 1971, referenceOK
			}
			return 1972, referenceOK
		case 11:
			switch {
			case long:
				return 1969, referenceOK
			case day > 26:
				return 1971, referenceOK
			}
			return 1972, referenceOK
		}
		return 1971, referenceOK
	}
	pick := func(short, longYear int) (int, errReference) {
		if long {
			if longYear == 0 {
				return regular()
			}
			return longYear, referenceOK
		}
		if short == 0 {
			return regular()
		}
		return short, referenceOK
	}
	switch m.number {
	case 1:
		return regular()
	case 2:
		return pick(1947, 0)
	case 3:
		return pick(1966, 1955)
	case 4:
		return pick(1963, 1944)
	case 5:
		return pick(1971, 1952)
	case 6:
		return pick(1960, 1941)
	case 7:
		return pick(1968, 1938)
	case 8:
		return pick(1957, 0)
	case 9:
		return pick(2014, 0)
	case 10:
		return pick(1984, 0)
	case 11:
		return pick(2033, 0)
	}
	return regular()
}
