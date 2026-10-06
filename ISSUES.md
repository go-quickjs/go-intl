# Known issues

What the review of 2026-10-06 found, at v0.3.2: seven reviewers, one for each
area of the codebase, each finding confirmed against Node 26.10.0 or against
the ICU 78.3, V8 or temporal_rs 0.2.3 source it was ported from. Each entry
says what goes wrong, how to see it, what the right answer is and where that
answer comes from.

An entry is closed by the commit that fixes it, with a regression test, and
its status names the test, which `git log -S` finds the commit by. Where a
fix finds the entry was wrong, the status says that instead.

Status: **open**, **fixed** (the test), **documented** (the example), or **not a
bug** (why).

## JavaScript-visible: NumberFormat, PluralRules and their kin

**NF-1. Engineering notation is wrong when rounding carries the mantissa to
the next power.** `scientific.go`. `en {notation:"engineering"}` 999999.5 is
"100E4", Node "1E6"; 0.9999995 is "100E-2", Node "1E0". The plural follows:
`fr` engineering 0.99999 is "many", Node "one". On a carry the exponent moves
by one where engineering must move to the next multiple of three, as ICU's
ScientificHandler does (getMultiplier(magnitude+1), and re-rounding as
chooseMultiplierAndApply does). Status: fixed (TestScientificCarry). The French plural the review gave as "one" is "many" in Node too.

**NF-2. roundingPriority picks the wrong side when both counts round at the
same place and rounding carries.** `digits.go`. `en {maximumSignificantDigits:3,
minimumSignificantDigits:2, maximumFractionDigits:3, roundingPriority:
"morePrecision"}` 0.99999 is "1.0", Node "1"; `{maximumSignificantDigits:3,
maximumFractionDigits:3, minimumFractionDigits:2, roundingPriority:
"lessPrecision"}` 0.99999 is "1.00", Node "1". Scientific `{1, 1, 1, 1,
morePrecision}` 9.99 is "1E1", Node "1.0E1"; compact 999.99 "1K", Node "1.0K".
The significant place is taken from the unrounded magnitude; ECMA-402's
ToRawPrecision takes [[RoundingMagnitude]] from the rounded result, as ICU's
number_rounding.cpp does (roundingMag2 += 1). Status: fixed (TestRoundingPriorityCarry).

**NF-3. Compact notation picks its wording from the unrounded magnitude.**
`compact.go`. `ar {notation:"compact", compactDisplay:"long"}` 9999.9 is
"10 آلاف", Node "10 ألف". ICU's CompactHandler takes the pattern for the
rounded magnitude even where the divisor stays. Status: fixed (TestCompactCarry).

**NF-4. Compact currency uses the decimal patterns.** `compact.go`,
`number.go`. ICU formats a compact currency with CLDR's currencyFormats short
patterns (CompactType TYPE_CURRENCY, short even for compactDisplay long),
which replace the currency and accounting patterns. `en {style:"currency",
currency:"USD", notation:"compact", compactDisplay:"long"}` 1234 is
"$1.2 thousand", Node "$1.2K"; accounting −1234 "($1.2K)", Node "-$1.2K";
`nl` −1234567 "US$ -1,2 mln.", Node "-US$ 1,2 mln."; `fa`, `ar`, `he`, `lo`,
`de-CH` differ too. Status: fixed (TestCompactCurrency).

**NF-5. A compact percentage is not written as the unit percent.**
`number.go`. V8 makes percent style the unit percent, and ICU writes a
compact one as a CLDR unit (number_formatimpl.cpp isCldrUnit). `ar` compact
−12.5% is "‎-1.3 ألف‎%‎", Node "‎-1.3 ألف٪"; `tr` "-%1,3 B", Node "%-1,3 B";
and formatToParts types the sign "percentSign" where Node says "unit".
Status: fixed (TestCompactPercent).

**NF-6. A unit's or currency's plural is chosen as if the number were
positive.** `number.go` (`f.round(magnitude, false)`). `en {style:"unit",
unit:"day", unitDisplay:"long", maximumFractionDigits:0, roundingMode:
"floor"}` −1.5 is "-2 day", Node "-2 days"; with ceil "-1 days", Node
"-1 day". Status: fixed (TestUnitPluralSignAndNotation).

**NF-7. A unit's or currency's plural ignores scientific notation.**
`number.go`. `pt {style:"unit", unit:"meter", unitDisplay:"long", notation:
"scientific"}` 0.001 is "1E-3 metro", Node "1E-3 metros"; `lv`, `zu`, `pl`
too. ICU picks the plural from the mantissa with the exponent as an operand.
Status: fixed (TestUnitPluralSignAndNotation).

**NF-8. PluralRules.Select of NaN or an infinity returns a real category.**
`plural.go`. `fr` Select(NaN) and Select(+Inf) are "one", `cy` NaN "few";
Node answers "other". Status: fixed (TestPluralNonFinite).

**NF-9. Currency spacing is missing after an exponent.** `number.go`. `bn
{style:"currency", currency:"USD", currencyDisplay:"code", notation:
"scientific"}` 1234 is "১.২৩৪E৩USD", Node "১.২৩৪E৩ USD"; accounting `ar`
likewise, ranges too. ICU tests only whether the adjoining character is a
digit. Status: fixed (TestCurrencySpacingAfterExponent).

**NF-10. Per-currency formats are missing.** Data (`numbergen`). CLDR gives
some currencies their own pattern, decimal or group, and ICU applies a
region's currency pattern to every currency: `en-150` EUR −1234.5 is
"-1,234.50 €", Node "-€1,234.50"; `kea` CVE "-1 234,50", Node "-1 234$50";
`en-DE` USD "-1.234,50 US$", Node "-US$1,234.50". About 45 locales differ.
Status: fixed (TestCurrencyFormats). The data now carries the formats ICU's curr tree gives, and where ICU goes beyond UTS #35, writing a region's currency's format for every other currency and the pattern for the name and accounting forms, it is the divergence CurrencyFormats. Over 25,740 currency amounts in every locale Node supports, NodeICU now differs from Node only where ICU misplaces a name after a spaced symbol (not reproduced) and in the locales of NF-12 and NF-20.

**NF-11. Compact long data is filled from short where a locale has only
some long patterns.** Data (`numbergen`). `ps` compact long 1234 is "۱٫۲K",
Node "۱۲۳۴"; `ast` long 12345678901 "12G", Node "12.346 millones"; `ps-PK`,
`wo` too. ICU uses only the locale's own long patterns; cldr-json hands
datagen inherited short ones. Status: fixed (TestCompactLongOwnPatterns). Over 22,464 compact numbers in every locale Node supports, short and long, with and without -u-nu-latn, ast, ps, ps-PK and wo now match, and only the locales of NF-20 differ.

**NF-12. Burmese writes a currency name on the wrong side.** Data. `my` GBP
currencyDisplay name −700.3 is "ဗြိတိသျှ ပေါင် -၇၀၀.၃၀", Node
"-၇၀၀.၃၀ ဗြိတိသျှ ပေါင်". cldr-json's unitPattern is "{1} {0}", ICU's
curr/my.txt "{0} {1}". Status: fixed (TestCurrencyUnitPatterns). The patterns now come from ICU's curr tree for every locale; over 25,740 currency amounts only Burmese changed.

**NF-13. Scientific signDisplay ignores a mantissa that rounds to zero.**
`number.go`. `de-CH {notation:"scientific", signDisplay:"exceptZero",
minimumFractionDigits:3, maximumFractionDigits:3, roundingIncrement:2500}` 1
is "+0.000E0", Node "0.000E0". Status: fixed (TestScientificSignOfZero).

**NF-14. formatToParts splits a currency name's pattern text.** `number.go`.
`ro` CHF name, maximumFractionDigits 0, 100: `[literal " de ", currency
"franci elvețieni"]`, Node `[literal " ", currency "de franci elvețieni"]`.
Status: fixed (TestCurrencyNameParts).

**NF-15. ListFormat lacks ICU's Spanish and Hebrew joins.** `list.go`. `es`
["a","b","Isabel"] is "a, b y Isabel", Node "a, b e Isabel"; disjunction
["siete","ocho"] "siete o ocho", Node "siete u ocho"; `he` ["a","Bob"]
lacks "ו-". ICU's listformatter.cpp chooses these in code, not data.
Status: fixed (TestListContextualJoins).

**NF-16. DisplayNames drops a language's variants and dialect names.**
`displaynames.go`. "en-GB-oxendict" is "British English", Node "British
English (Oxford English Dictionary spelling)"; "en-US-posix" "English (United
States)", Node "American English (Computer)"; "ca-ES-valencia" "Catalan
(Spain)", Node "Catalan (Spain, Valencian)". About half of 220 tags checked
differ. Needs variant names in the data and UTS #35's dialect lookup.
Status: fixed (TestLanguageDisplayNames). Compared with Node over 1,046 tags in 14 locales with five sets of options, all 73,220 alike. go-quickjs rejects "en-US-posix", which canonicalizes to "en-US-u-va-posix", where V8 checks the code as given; that is fixed there when it takes this release.

**NF-17. Three Node behaviours differ from ECMA-402 with no named
divergence.** Accounting with signDisplay "never" (`nb` EUR: "€ 1,00", Node
"1,00 €": V8 maps it to UNUM_SIGN_NEVER, dropping the accounting pattern);
RelativeTimeFormat numeric "auto" treating values within 0.005 of −2…2 as
exact (`en` 0.0001 day: "in 0 days", Node "today": ICU formatRelativeImpl);
and roundingIncrement on doubles past 16 digits (`{minimumFractionDigits:2,
maximumFractionDigits:2, roundingIncrement:2}` 3.9967620239602476e27: Node
"…248000…", go-intl "…247600…": ICU rounds an approximate value).
Status: fixed (TestAccountingNever, TestRelativeEpsilon, TestApproximateIncrement): named the divergences AccountingNever, RelativeEpsilon and ApproximateIncrement. The increment one is ICU reading a double by its fast path, whose digits roundToIncrement alone leaves uncorrected, for any increment but 1 and 5, 10 and 100 included. go-quickjs must pass RelativeTimeFormat its Compat, and a Number to NumberFormat as DecimalFromFloat, for its Node side to see them.

**NF-18. DisplayNames names the calendar "islamicc".** `internal/namegen`.
`ar {type:"calendar", fallback:"none"}` "islamicc" is "التقويم الهجري
المدني", Node undefined. namegen files CLDR's "islamic-civil" under
"islamicc" too, a deprecated alias ICU's Types table does not have; found
comparing every calendar in 29 locales while fixing NF-16. Status: fixed (TestCalendarDisplayNames). Every region, script and calendar name in 29 locales and three widths now matches Node, 45,849 of them.

**NF-19. ListFormat keeps an empty item as an element.** `list.go`. `es`
["a",""] is [element "a", literal " y ", element ""], Node [element "a",
literal " y "]: ICU's FormattedList has no field for an empty span. Found
while fixing NF-15. Status: fixed (TestEmptyListItems): ECMA-402 writes the element, so this is the divergence EmptyListItems. go-quickjs must pass ListFormat its Compat for its Node side to see it.

**NF-20. A locale with a script ICU's number data lacks resolves to it
anyway.** `number.go`, the available locales. Node resolves `bm-Nkoo`,
`ha-Arab`, `mn-Mong`, `mni-Mtei`, `ms-Arab`, `zh-Latn` and `az-Arab` for
NumberFormat to the language alone and formats with its data: `zh-Latn`
compact CVE is "-CVE 123万", go-intl "-CVE 1.2M"; `mni-Mtei` writes
Bengali digits. Found comparing currencies in every locale while fixing
NF-10. Status: documented (ExampleLocaleMatcher_Resolve). Not JS-visible: a constructor works in the locale it is given, and ECMA-402's ResolveLocale, LocaleMatcher.Resolve over the service's available locales, is what makes "az-Arab" "az", as V8 does; go-quickjs runs it and writes all seven locales as Node does. A Go program calling a constructor directly does not get it, so each constructor, LocaleMatcher.Resolve and the README now say so, and the example shows it.

## JavaScript-visible: DateTimeFormat and the calendars

**DT-1. Time styles skip ICU's pattern-generator path.** `skeleton.go`.
ICU's SimpleDateFormat::construct (smpdtfmt.cpp) builds a time style from
"jmmsszzzz" and the rest with a generator, instead of the locale's stored
pattern, where the locale has -u-hc or -u-rg, or the calendar's
DateTimePatterns came from a parent of another language or region.
`en-GB-u-hc-h12` timeStyle medium 07:00 is "07:00:00 am", Node "7:00:00 am";
`ja-u-hc-h23` full "7時05分09秒 協定世界時", Node "7:05:09 協定世界時"; `ar`
chinese medium "7:05:09 ص", Node "07:05:09 ص". About 960 of 1,520 locale ×
style cases with -u-hc-h11 or -h12, and 110–535 per non-Gregorian calendar
with no keyword. Status: fixed (TestTimeStyleAfresh). The first fix took the valid locale from the data locale, which for en-US is cldr-json's "en", and test262's timedatestyle-en.js caught it in go-quickjs; it is now the bundle ICU opens for the locale itself. Over 948 locales, ICU's default-content ones among them, all 56,880 time styles with each hour cycle keyword, 34,128 with hour12 or hourCycle, 114,708 style sets and 192,444 field sets match Node.

**DT-2. The Japanese first year "元年" is missing outside date styles.**
`skeleton.go`. ICU writes year 1 of a Japanese era as 元年 in any pattern with
an unquoted 年 when the language is ja (smpdtfmt.cpp, jpanyear). `ja`
japanese `{year:"numeric", month:"long", day:"numeric"}` 2019-07-23 is
"令和1年7月23日", Node "令和元年7月23日"; ranges likewise. Status: fixed (TestGannen), for fields, styles, ranges and Temporal values alike, each checked against Node.

**DT-3. Non-Gregorian patterns come from cldr-json, not ICU.** Data
(`dategen`). cldr-json resolves root's generic calendar alias to the locale's
Gregorian patterns; ICU resolves it to root's own. `en-GB` indian
`{hour:"numeric", minute:"2-digit"}` 07:00 is "7:00", Node "07:00"; `da`
chinese "07.05.09", Node "07:05:09"; `zh-Hans-HK` coptic short
"4/3/40科普特历", Node "科普特历1740/3/4". 56–181 of 1,900 cases per calendar.
Status: fixed (TestCalendarStylePatterns). In every locale Node supports, 95,410 field option sets over seven calendars and 56,870 style sets over eleven now all match Node; 1,866 and 3,173 had differed.

**DT-4. The pattern generator takes availableFormats in the wrong order.**
Data (`dategen`) and `dtpg.go`. ICU adds them bundle by bundle, the child
first, and keeps the first of two equally near; datagen sorts them by ID.
`en-ZA {month:"numeric", day:"2-digit"}` is "13/7", Node "07/13"; `es-PA`
likewise; `es-419 {month:"long", day:"2-digit"}` "13 de julio", Node
"13-julio". Status: fixed (TestAvailableFormatsOrder). Over 13,630 Gregorian option sets in every locale Node supports, all now match Node, from 25 differing. The entry's claim that ICU keeps the first of two equally near is right; ICU's AvailableFormatsSink also lets every entry, the root's included, override a pattern the styles made.

**DT-5. A two-digit year below zero loses its sign.** `datepattern.go`.
`en` persian `{year:"2-digit"}` 500 CE is "78 AP", Node "-22 AP"; Japanese,
Hebrew, Buddhist, Indian likewise. ICU's zeroPaddingNumber(value, 2, 2)
keeps the sign. Status: fixed (TestTwoDigitYearSign).

**DT-6. The NodeICU Chinese and Dangi reckoning writes month 0 in the far
future.** `calchinese.go`. ICU refuses a date outside two winter solstices
(chnsecal.cpp) and Node throws past about year 69,096; go-intl writes dangi
month 0 with an empty name: " 29, 100000(geng-zi)". Status: fixed (TestChineseAstronomyRange): DateTimeFormat.Check reports ErrCalendarRange where ICU fails, its own solstice check and the year lengths Calendar::computeWeekFields asks for, which match Node's failing days exactly in both windows, for both calendars, by each locale's week rules. go-quickjs must call Check before formatting to throw where Node does.

## JavaScript-visible: locales

**LO-1. A keyword given twice keeps the wrong value past twelve keywords.**
`locale.go`. Keywords are sorted with sort.Slice, which is not stable past
twelve elements, before the first of two duplicates is kept.
`en-u-ca-gregory-co-phonebk-cu-usd-fw-mon-hc-h23-ka-shifted-kb-kc-kf-upper-kn-kr-space-ks-level1-ca-buddhist`
canonicalizes with ca-buddhist, Node ca-gregory. Status: fixed (TestDuplicateKeywordFirstWins).

**LO-2. Canonicalizing can give the same variant twice.** `canonical.go`.
`en-heploc-alalc97` is "en-alalc97-alalc97", Node "en-alalc97";
`en-fonipa-x-lvariant-fonipa` "en-fonipa-fonipa", Node a RangeError. ICU
drops repeated variants (uloc_tag.cpp). Status: fixed (TestVariantGivenTwice).

**LO-3. und with a script and a region is maximized by the region first.**
`fallbacker.go`. ICU and ICU4X look up und_S_R, then und_S, then und_R.
`und-Cyrl-CN` maximizes to "zh-Cyrl-CN", Node "ru-Cyrl-CN"; minimize "zh-Cyrl",
Node "ru-CN". 237 of 33,216 combinations. Status: fixed (TestLikelySubtagsUndScriptFirst). Of 7,700 language, script and region combinations, maximize and minimize all now match Node; the 149 that differed were all und with a script and a region.

**LO-4. A -u-rg- or -u-sd- region is not checked.** `localeinfo.go`,
`skeleton.go`. ICU ignores a region not in RegionValidateMap
(loclikely.cpp). `en-u-rg-xxzzzz` getWeekInfo().firstDay is 1, Node 7;
getHourCycles ["h23"], Node ["h12"]; `en-u-rg-abcdefgh` h23, Node h12.
Status: fixed (TestKeywordRegionValid). A keyword with no value, "en-u-rg-gb" being the keywords gb and rg, is ICU's "yes" and names Yemen on Node's side, part of the YesValues divergence.

**LO-5. A lone POSIX variant becomes -u-va-posix before aliases are
replaced.** `canonical.go`. ICU makes the change when it writes the tag, after
AliasReplacer. `en-arevela-posix` is "en-posix", Node "en-u-va-posix".
Status: fixed (TestLonePosixAfterAliases).

**LO-6. Private use before lvariant is kept.** `canonical.go`. ICU's
ultag_parse drops it. `en-x-foo-lvariant-abcde` is "en-abcde-x-foo", Node
"en-abcde". Status: fixed (TestLvariantDropsPrivateUse).

**LO-7. HostLocale gives up on a POSIX locale with a modifier.**
`hostlocale.go`. `LANG=de_DE.UTF-8@euro` becomes en-US; ICU's toLanguageTag
keeps German, "de-DE-x-lvariant-euro". `de@euro` reads as the script Euro.
Status: fixed (TestLocaleFromICUID), following ICU's _appendVariantsToLanguageTag; Node on Windows reads the Windows locale, not LANG, so this could not be checked against it here.

**LO-8. A -t- extension's fields are ordered by key alone.** `canonical.go`.
ICU sorts them by key and value. `art-CS-t-m0-names-names-m0-hwidth-names`:
Node "art-RS-t-m0-hwidth-names-m0-names-names", go-intl keeps the input
order. Status: fixed (TestTransformedFieldOrder).

## JavaScript-visible: time zones

**TZ-1. A custom UTC zone from TZ is named by its offset.** `zone.go`,
`zonedisplay.go`. ICU names a custom zone of offset zero "GMT", whose CLDR
canonical zone is Etc/GMT. Windows, `TZ=GMT+00`: `new Date(0).toString()`
ends "(GMT+00:00)", Node "(Greenwich Mean Time)". Status: fixed (TestHostTimeZoneGMT).

**TZ-2. Canonical and DefaultTimeZone mix V8's two namings.** `zone.go`. With
`TZ=GMT` Node's DateTimeFormat reports "+00:00" (JSDateTimeFormat::TimeZoneId
special-cases "GMT") and Temporal.Now.timeZoneId "UTC"
(Intl::DefaultTimeZone); go-intl's Canonical gives "UTC" for TZ=GMT and
DefaultTimeZone "+00:00" for TZ=GMT+00. Status: fixed (TestHostTimeZoneGMT): the host zone's Canonical is what a DateTimeFormat reports and DefaultTimeZone what Temporal.Now reports. go-quickjs takes one name for both (localZoneName) and must take each where V8 does.

## Go API: panics, hangs and silent acceptance

**API-1. NumberFormat and PluralRules accept options ECMA-402 rejects, and
some panic.** `number.go`, `digits.go`. MaximumFractionDigits −1 builds and
then panics in Format; 101, MaximumSignificantDigits 22,
MinimumSignificantDigits 0, MinimumIntegerDigits 22, a roundingIncrement not
in ECMA-402's list, unnamed enum values and an ill-formed currency code are
accepted. Status: fixed (TestNumberOptionsRefused): the new ErrOption wraps every option NumberFormat or PluralRules refuses.

**API-2. DateTimeFormat accepts options ECMA-402 rejects, and one panics.**
`datetime.go`, `skeleton.go`. DateStyle 9 panics; FractionalSecondDigits 4
is clamped; Hour WidthLong writes no hour; Era Width2Digit is dropped;
unnamed hour cycles and zone styles are ignored; a calendar alias such as
"islamicc" falls back to Gregorian where ECMA-402 canonicalizes it.
Status: fixed (TestDateTimeOptionsRefused).

**API-3. Temporal's DateAdd hangs on math.MinInt64.** `temporal/calendarops.go`.
`abs64(MinInt64)` stays negative, so the range check passes and the month
loop runs for ever in every non-ISO calendar. temporal_rs refuses it.
Status: fixed (TestDateAddMinInt64). A full validity check in front of DateAdd was tried first and is wrong: temporal_rs lets the days balanced out of time units through to the ISO arithmetic, which reports "epoch days exceed maximum range.", and go-intl's Node replay caught the changed message.

**API-4. Temporal wraps years outside int32.** `temporal/plaindate.go`.
`NewPlainDate(4294969316, 1, 1, ISOCalendar, Reject)` succeeds and prints
"2020-01-01"; Node "Invalid ISO date.". Status: fixed (TestYearOutsideInt32).

**API-5. A Hebrew date far out of range hangs.** `temporal/lunisolar.go`.
`heb.Date(ISODate{1<<40, 6, 15})` overflows and loops; the solar calendars
return garbage years. Status: fixed (TestCalendarDateOutOfRange): Calendar.Date now returns an error, for a date outside ISO's limits, which no Temporal value holds.

**API-6. Duration.Round and Total with an unnamed unit panic or answer
infinity.** `temporal/durationround.go`, `temporal/duration.go`.
`SmallestUnit: Unit(11)` divides by zero; Total(UnitAuto) is +Inf, where
temporal_rs refuses it. Status: fixed (TestDurationUnitOptions).

**API-7. Temporal's zero option structs are errors.** `temporal/options.go`.
`DifferenceSettings{}` gives "Unit was not part of the date unit group."
where DESIGN.md has the zero value be the default; NoUnit is −1, so a zero
Unit means "auto". Status: fixed. NoUnit is now the zero Unit, the rest
renumbered in the same order, so `DifferenceSettings{}` and
`RoundingOptions{}` are the defaults. Precision keeps its numbering, which
go-quickjs converts from fractionalSecondDigits: its zero is 0 digits, as
`fractionalSecondDigits: 0` is, and the field's doc says to start from
DefaultToStringOptions (temporal/zero_options_test.go).

**API-8. ParseTables panics on a malformed month.**
`internal/eastasian/eastasian.go`. Month 13 or 00 indexes out of range; a
leap ordinal of 99 is accepted. Reachable through a custom Source.
Status: fixed. A year must have twelve or thirteen months, a leap ordinal
of 0 or, in a year of thirteen, 2 to 13, and a start that is a real
Gregorian date (internal/eastasian/eastasian_test.go).

**API-9. LoadZoneRecord hands out read-only memory as a writable slice.**
`zonerecord.go`. TransitionTypes points into the embedded pack; writing to
it is a fault recover cannot catch. Status: fixed. The record copies the
types; the transitions were already decoded into a fresh slice
(zonerecord_test.go).

**API-10. Exported temporal functions take the unexported int128.**
`temporal`. NewZonedDateTime, TimeZone.OffsetNanosecondsFor and the
EpochNanosecondsForUTC methods cannot be called from outside; OffsetTimeZone
accepts an offset of a day or more and writes garbage. Status: fixed.
NewZonedDateTime is unexported, NewZonedDateTimeFromInstant being the
constructor; OffsetNanosecondsFor takes an Instant; EpochNanosecondsForUTC
returns a high and low word, as Instant.EpochNanoseconds does; NewDuration
takes int64 microseconds and nanoseconds, larger ones coming from
DurationFromNumbers or ParseDuration. OffsetTimeZone returns an error for
an offset that is not whole minutes less than a day, as temporal_rs's
offset zones are. A change to exported signatures, none of which go-quickjs
calls (temporal/exported_api_test.go).

**API-11. Exact decimals with huge exponents allocate without bound.**
`mag.go`. maxDecimalExponent is 2³⁰, so "1e1000000000" can build a
gigabyte string. Status: fixed. ParseExactDecimal keeps only digits
written out exactly; a string with an exponent is read as ParseDecimal
reads it, rounding past a float's range to an infinity, as Node does with
every such string. A BigInt's decimal string has no exponent. go-quickjs
had handed BigInts over as "0.<digits>e<exp>", a by-product of its own
decimal type, and now writes them out in full; that change goes with the
release that takes this (decimal_exact_test.go).

## Corrupt data

**DA-1. A huge pool part number panics on 64-bit.** `internal/blob/blob.go`.
`4*i+4 > len(s.ends)` overflows for i ≥ 2⁶¹. Status: fixed. The part
number is compared with the number of parts, not multiplied out
(internal/blob/blob_test.go).

**DA-2. Corrupt collation tables panic.** `internal/colldata`,
`collelements.go`. The trie and the element arrays are read without bounds
checks. Status: fixed. The trie's index reads, and every read of the
expansions, elements and contexts an element points to, are checked; a
value past them weighs as U+FFFD. A digit's first value is the element in
hand, as ICU's appendNumericCEs takes it, where a second lookup a corrupt
table answered otherwise had looped for ever. 3,000 corrupted tables are
built and compared with (collator_corrupt_test.go).

**DA-3. Counts overflow on 32-bit.** `internal/blob` ReadShared and
ReadIndex, `internal/datapack`. A count of 0x40000000 passes the length
check under GOARCH=386 and panics. Status: fixed. Counts are compared with
what the bytes can hold rather than multiplied out, and an offset that
reads as negative is refused; the tests run under GOARCH=386 too
(internal/blob/blob_test.go, internal/datapack/datapack_test.go).

**DA-4. The segmenter's data readers panic on corrupt data.** `brkdata.go`,
`dictbe.go`. Status: fixed. The decoder checks every state-table cell (next
states, look-ahead slots), the trie's categories, the sections' bounds
without adding them, and a dictionary's offset against its end; the trie,
dictionary and status readers bound every read and refuse a negative
number; and the breaker moves on by a code point where corrupt rules or a
dictionary engine would not advance or go back. 1,800 corrupted files are
segmented with, and 60,000 were in a probe on amd64 and 386
(segmenter_corrupt_test.go).

## Rules

**RU-1. Collation differs from Node where ICU skips the shared prefix, with
no named divergence.** `collator.go`. ICU's doCompare skips the prefix two
strings share unless the next character is unsafe; its digit test reads
only the tailoring. go-intl compares the whole strings, which is UCA's
answer. `ar {numeric:true}` "١٥" vs "١٠٠": Node 1, go-intl −1; fr-CA
backward secondaries and one ignorePunctuation case likewise. Status: fixed,
as the IdenticalPrefix divergence. NodeICU ports doCompare's prefix test
over UTF-16 units, with ICU's unsafe-backward sets, which collgen now reads
from collunsafe.h and each compiled table, and its one-unit,
tailoring-only digit test, which also misses a supplementary digit in the
root ("1é" against "1𝟏"). It matches Node on 7,065,792 pairs of strings
sharing a prefix over 29 locales and 8 option sets; Standard differs from
Node on 30,560 of them, all with numeric. go-quickjs's standards mode
takes Standard (collator_prefix_test.go).

**RU-2. A failed generator run does not leave the old data.**
`internal/datawrite`, the generators. datawrite deletes the old files before
it writes the new; the shared pools are written after the locale files and
without a temporary file; dategen and availgen write some tables before all
are built. Seen three times on Windows while fixing NF-10 and DT-1: tzgen
moved data/tz aside, could not rename data/tz.tmp into its place ("Access
is denied", something holding the directory), and left the repository
without data/tz until it was restored from git. Status: fixed. Every
generator builds everything, then commits it as one internal/writeset Set:
each file is staged beside its target, the old one is kept while all are
swapped in, and a failed swap puts the old ones back. A directory a
generator owns, data/tz or a locale directory, is replaced file by file
rather than renamed whole, and renames are retried for five seconds, which
outlasts the scanner that held them. packgen refuses a file a failed run
left behind (internal/writeset/writeset_test.go).

**RU-3. Currency spacing depends on the Go toolchain's Unicode.**
`unicodeset.go`, `compact.go`. The sets come from Go's unicode tables, which
are Unicode 15 up to Go 1.26: built with it, `en-u-nu-gara` CHF loses its
space. Status: fixed. A new generator, propgen, writes the general
categories, the default ignorables and the Hebrew script from UCD 17.0.0,
ICU 78.3's, to data/properties.bin, and currency spacing, the parts'
trimming and Hebrew lists read that; nothing at run time reads Go's
unicode tables. The test fails on the old code under Go 1.24 and passes
under 1.24 and 1.27 (uprops_test.go).

**RU-4. Exported package variables can be reassigned.** `subtag.go` (Und),
`compat.go` (Divergences), `source.go` (Embedded), `temporal` (Calendars,
ISOCalendar). Status: fixed. Und is gone, the zero Language being und, with
Language.IsUnd to ask; Divergences and temporal.Calendars are functions
returning a copy; temporal.ISOCalendar and temporal.DefaultToStringOptions
are functions, and DefaultDifferenceSettings is gone, the zero
DifferenceSettings being the default since API-7; Embedded is of a type,
EmbeddedSource, with one value. Only error sentinels are left, as io.EOF
is. A test parses the package for any other exported variable
(exported_vars_test.go). go-quickjs's two ISOCalendar and five
DefaultToStringOptions uses change with it.

**RU-5. A generator's map order could make its output vary.**
`internal/namegen` fill: two alternates of one width and no plain name take
whichever the map gives first. No CLDR 48.2 locale has that today.
Status: fixed. The alternates are an ordered list: a plain name is the long
one, and an alternate fills its width where nothing has, the first in the
list winning; the meaningless empty suffix is gone. The data is unchanged
(internal/namegen/main_test.go).

**RU-6. Index and pair tables have no version byte.** `internal/blob`.
Status: fixed. blob.BuildIndex and ReadIndex take a version, as NewWriter
and NewReader do, and the pair tables (likelysubtags, parentlocales, each
same.bin) begin with one; each file's version is in internal/layout, and
normdata and zonedata's metazones, whose nested indexes gained the byte,
went up a version. A file written before, or under another version, is
refused (internal/blob/blob_test.go, pairtable_test.go).

**RU-7. regen trusts an unpacked tree and has no timeout.**
`internal/regen`. Status: fixed. The marker an extraction leaves names the
archive's sha256 and a digest of every file's path, size and time of
change; a tree that no longer matches, or another archive, is extracted
again, which costs a stat a file. Downloads give up after ten minutes, or
a minute without a response, and a generator after ten minutes
(internal/regen/main_test.go).

**RU-8. The host zone's last fallback on Linux is not a named divergence.**
`hostzone_other.go`. ICU falls back to the C library's abbreviations where
/etc/localtime is not under zoneinfo; go-intl gives Etc/Unknown.
Status: fixed, as the HostAbbreviations divergence; to be measured against
Node on a Linux host. Standard reports the host's offset, "+01:00", as
ECMA-262's SystemTimeZoneIdentifier has it. NodeICU ports uprv_tzname's
end: the abbreviations at 2007's solstices, as glibc's localtime_r leaves
them, of the zone Go loads from the same /etc/localtime, looked up with the
standard offset in putil.cpp's OFFSET_ZONE_MAPPINGS, which tzgen now reads
from the sources archive, else the standard abbreviation, which
detectHostZone makes a zone of. HostTimeZone and DefaultTimeZone take a
Compat, and date.Options carries one (hostabbrev_test.go).

## Performance

**PE-1. Collation is quadratic in a contraction followed by a run of
combining marks.** `collelements.go`. 10,000 marks take 3 s, 100,000 about
22 s; Node is quadratic too. Status: fixed. The case is a run of marks that
each begin a contraction, Tibetan's U+0F71: each looked through every mark
after it, though those of its own class are blocked. A run's marks are in
canonical order, sorted by class, so the search now goes past a class at
once; 40,000 take 0.1 s where they took 54 s under NodeICU and 108 s under
Standard, and the 7,065,792 pairs of RU-1 still match Node
(collator_marks_test.go).

**PE-2. Temporal's until in months is quadratic in the Chinese and Dangi
calendars.** `temporal/arith.go`. 48,000 years take 17 s, Node 6 s.
Status: open.

## Tests and dead code

**TE-1. Gaps.** No recording reaches a first Japanese year, the non-Gregorian
calendars in every locale, Segmenter.containing, duplicate keywords and
variants, corrupt data or the 32-bit paths; internal/datapack has no tests.
Status: open.

**TE-2. Dead code.** The quaternary collation level and Collator.elements;
digitsOf; DateDuration.negated, durationFromDate and Calendar.fieldsOf in
temporal; Environment.Parse repeats Environment.UTC. Status: open.
