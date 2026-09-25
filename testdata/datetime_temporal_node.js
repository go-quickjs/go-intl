// Writes testdata/datetime_temporal_node.txt.gz: what Node writes for
// Temporal values -- Intl.DateTimeFormat's format, formatToParts and
// formatRange of each kind, and each type's toLocaleString -- across
// locales, option sets, calendars and zones.
//
//	node testdata/datetime_temporal_node.js
//
// Each line is [locale, options, method, kind, calendar, values, result]:
// method is "format", "parts", "range" or "toLocaleString"; kind is
// "date", "datetime", "time", "yearmonth", "monthday", "instant" or "zoned";
// values are one value, or two for a range, each its ISO fields
// [year, month, day, hour, minute, second, millisecond] -- a year-month's
// reference day and a month-day's reference year among them -- or, for an
// instant, [epoch milliseconds], and for a zoned date-time
// [epoch milliseconds, zone]. result is the string, the parts as
// [type, value] pairs, or {"error": name}.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const locales = [
  "en", "en-GB", "en-US-u-hc-h23", "de", "fr", "es", "pt-BR", "fi", "ru",
  "ja", "ja-JP-u-ca-japanese", "zh", "zh-Hant", "ko", "ar", "he", "hi", "th",
  "fa", "am",
];
const configs = [
  {}, { dateStyle: "full" }, { dateStyle: "short" }, { timeStyle: "short" },
  { timeStyle: "full" }, { dateStyle: "medium", timeStyle: "medium" },
  { dateStyle: "long", timeStyle: "short" },
  { year: "numeric" }, { month: "long" }, { day: "numeric" }, { weekday: "long" },
  { era: "short" }, { year: "numeric", month: "short" }, { month: "numeric", day: "numeric" },
  { weekday: "short", year: "numeric", month: "long", day: "numeric" },
  { hour: "numeric" }, { minute: "2-digit" }, { hour: "2-digit", minute: "2-digit" },
  { hour: "numeric", hour12: false }, { hour: "numeric", hourCycle: "h11" },
  { hour: "numeric", minute: "numeric", second: "numeric", fractionalSecondDigits: 3 },
  { dayPeriod: "long" }, { timeZoneName: "short" }, { hour: "numeric", timeZoneName: "longOffset" },
  { year: "numeric", hour: "numeric" }, { month: "long", day: "numeric", hour: "numeric", minute: "numeric" },
  { hourCycle: "h23" }, { hour12: true },
  { calendar: "japanese", era: "long", year: "numeric" }, { calendar: "hebrew" },
  { calendar: "islamic" }, { calendar: "islamic-civil", month: "long", day: "numeric" },
  { numberingSystem: "arab" },
];
const zones = ["UTC", "America/New_York"];

// Values: [kind, calendar, fields, make].
const T = Temporal;
const plainDate = (y, m, d, cal) => ["date", cal, [y, m, d, 0, 0, 0, 0], () => new T.PlainDate(y, m, d, cal)];
const plainDateTime = (y, m, d, h, mi, s, ms) =>
  ["datetime", "iso8601", [y, m, d, h, mi, s, ms], () => new T.PlainDateTime(y, m, d, h, mi, s, ms)];
const plainTime = (h, mi, s, ms) => ["time", "", [1970, 1, 1, h, mi, s, ms], () => new T.PlainTime(h, mi, s, ms)];
const yearMonth = (y, m, cal, day) => ["yearmonth", cal, [y, m, day, 0, 0, 0, 0], () => new T.PlainYearMonth(y, m, cal, day)];
const monthDay = (m, d, cal, year) => ["monthday", cal, [year, m, d, 0, 0, 0, 0], () => new T.PlainMonthDay(m, d, cal, year)];
const instant = ms => ["instant", "", [ms], () => T.Instant.fromEpochMilliseconds(ms)];

const values = [
  plainDate(2024, 1, 5, "iso8601"), plainDate(1999, 12, 31, "iso8601"),
  plainDate(2024, 1, 5, "gregory"), plainDate(2024, 1, 5, "japanese"),
  plainDate(2024, 1, 5, "hebrew"), plainDate(2024, 1, 5, "islamic-civil"),
  plainDateTime(2024, 7, 20, 15, 45, 6, 123), plainDateTime(2024, 3, 10, 2, 30, 0, 0),
  plainTime(3, 4, 5, 678), plainTime(15, 0, 0, 0),
  yearMonth(2024, 5, "iso8601", 1), yearMonth(2024, 5, "gregory", 1),
  monthDay(12, 25, "iso8601", 1972), monthDay(12, 25, "gregory", 1972),
  instant(1704412800123), instant(1721487545678),
];
const ranges = [
  [plainDate(2024, 1, 5, "iso8601"), plainDate(2024, 1, 20, "iso8601")],
  [plainDate(2024, 1, 5, "iso8601"), plainDate(2025, 3, 5, "iso8601")],
  [plainDateTime(2024, 7, 20, 15, 45, 0, 0), plainDateTime(2024, 7, 20, 18, 0, 0, 0)],
  [plainTime(3, 4, 0, 0), plainTime(15, 0, 0, 0)],
  [yearMonth(2024, 5, "gregory", 1), yearMonth(2024, 8, "gregory", 1)],
  [monthDay(12, 25, "gregory", 1972), monthDay(12, 31, "gregory", 1972)],
  [instant(1704412800123), instant(1704499200000)],
];
const zoned = [
  [1704412800123, "America/New_York"], [1721487545678, "Asia/Tokyo"], [1721487545678, "UTC"],
];

const outcome = fn => {
  try {
    return fn();
  } catch (e) {
    return { error: e.name };
  }
};
const lines = [];
const push = (loc, opts, method, kind, cal, vals, result) =>
  lines.push(JSON.stringify([loc, opts, method, kind, cal, vals, result]));

for (const loc of locales) {
  for (const config of configs) {
    for (const zone of zones) {
      const opts = { ...config, timeZone: zone };
      let f;
      try {
        f = new Intl.DateTimeFormat(loc, opts);
      } catch (e) {
        continue;
      }
      for (const [kind, cal, fields, make] of values) {
        push(loc, opts, "format", kind, cal, [fields], outcome(() => f.format(make())));
        push(loc, opts, "parts", kind, cal, [fields],
          outcome(() => f.formatToParts(make()).map(p => [p.type, p.value])));
        push(loc, opts, "toLocaleString", kind, cal, [fields], outcome(() => make().toLocaleString(loc, opts)));
      }
      for (const [a, b] of ranges) {
        push(loc, opts, "range", a[0], a[1], [a[2], b[2]], outcome(() => f.formatRange(a[3](), b[3]())));
      }
    }
    for (const [ms, zone] of zoned) {
      push(loc, config, "toLocaleString", "zoned", "iso8601", [[ms, zone]],
        outcome(() => T.Instant.fromEpochMilliseconds(ms).toZonedDateTimeISO(zone).toLocaleString(loc, config)));
    }
  }
}
const out = path.join(__dirname, "datetime_temporal_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
