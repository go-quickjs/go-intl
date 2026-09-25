// Writes testdata/temporal_node.txt.gz: what Node's Temporal answers for
// its operations, as JavaScript calls them.
//
//	node testdata/temporal_node.js
//
// Each line is a JSON array: [op, receiver, args, result]. op is
// "new:Type", "static:Type.name", "method:Type.name" or "get:Type.name".
// Values are JSON with these objects standing for what JSON lacks:
//
//	{"$u": 0}                 undefined
//	{"$n": "NaN"}             NaN, Infinity, -Infinity or -0
//	{"$big": "123"}           a BigInt
//	{"$PlainDate": "..."}     Temporal.PlainDate.from(...), and so for every
//	                          Temporal type
//
// any other object being a plain object. A result is a value, a Temporal
// object written as the string that identifies it (see enc), or
// {"$err": name, "msg": message} for what was thrown.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const types = ["Duration", "Instant", "PlainDate", "PlainDateTime", "PlainMonthDay", "PlainTime",
  "PlainYearMonth", "ZonedDateTime"];

function dec(s) {
  if (s === null || typeof s !== "object") return s;
  if (Array.isArray(s)) return s.map(dec);
  const keys = Object.keys(s);
  if (keys.length === 1 && keys[0][0] === "$") {
    const t = keys[0].slice(1), v = s[keys[0]];
    switch (t) {
      case "u": return undefined;
      case "n": return Number(v);
      case "big": return BigInt(v);
    }
    return Temporal[t].from(v);
  }
  const o = {};
  for (const k of keys) o[k] = dec(s[k]);
  return o;
}

function enc(v) {
  if (v === undefined) return { $u: 0 };
  if (typeof v === "number") {
    if (Number.isNaN(v)) return { $n: "NaN" };
    if (!Number.isFinite(v)) return { $n: v > 0 ? "Infinity" : "-Infinity" };
    if (Object.is(v, -0)) return { $n: "-0" };
    return v;
  }
  if (typeof v === "bigint") return { $big: v.toString() };
  if (v instanceof Temporal.Duration) return { $Duration: v.toString() };
  if (v instanceof Temporal.Instant) return { $Instant: v.toString() };
  if (v instanceof Temporal.PlainTime) return { $PlainTime: v.toString() };
  if (v instanceof Temporal.PlainDate || v instanceof Temporal.PlainDateTime ||
    v instanceof Temporal.PlainYearMonth || v instanceof Temporal.PlainMonthDay) {
    return { ["$" + v.constructor.name]: v.toString({ calendarName: "always" }) };
  }
  if (v instanceof Temporal.ZonedDateTime) {
    return { $ZonedDateTime: v.toString({ calendarName: "always" }) + " " + v.offset };
  }
  return v;
}

const lines = [];
function run(op, recv, args) {
  const [kind, rest] = op.split(":");
  const [type, name] = rest.split(".");
  let result;
  const t0 = Date.now();
  try {
    const self = dec(recv);
    const a = args.map(dec);
    switch (kind) {
      case "new": result = new Temporal[type](...a); break;
      case "static": result = Temporal[type][name](...a); break;
      case "method": result = self[name](...a); break;
      case "get": result = self[name]; break;
    }
    result = enc(result);
  } catch (e) {
    result = { $err: e.constructor.name, msg: e.message };
  }
  if (Date.now() - t0 > 200) console.error("slow:", Date.now() - t0, "ms", JSON.stringify([op, recv, args]));
  lines.push(JSON.stringify([op, recv, args, result]));
}

const U = { $u: 0 };

// ==== Strings, parsed as every type ====

const dates = ["2020-01-01", "2020-1-01", "20200101", "+002020-01-01", "-002020-01-01", "-000000-01-01",
  "+000000-01-01", "2020-02-29", "2021-02-29", "2020-13-01", "2020-00-01", "2020-01-00", "2020-01-32",
  "2020-04-31", "+275760-09-13", "+275760-09-14", "-271821-04-19", "-271821-04-20", "-271821-04-18",
  "+999999-12-31", "2020-01", "202001", "01-01", "--01-01", "0101", "--0101", "-01-01", "2020-01-01T",
  "2020-W01-1", "2020-001", "12-31", "02-29", "02-30", "--02-29"];
const times = ["T12", "T12:34", "T1234", "T12:34:56", "T123456", "T12:34:56.1", "T12:34:56.123456789",
  "T12:34:56.1234567891", "T12:34:56,5", "T12:34:60", "T23:59:59.999999999", "T24:00", "T12:3456",
  "T1234:56", "t12:00", " 12:00", "T12:00:00.", "T12.5", "T12:34.5"];
const offsets = ["", "Z", "z", "+00:00", "-00:00", "+05:30", "+0530", "+05", "-08:00", "+05:30:15",
  "+05:30:15.123456789", "+053015", "+05:30:15.1234567891", "+24:00", "+23:59", "-23:59:59.999999999",
  "+5:30", "+05:3", "−05:00"];
const annotations = ["", "[UTC]", "[Europe/Paris]", "[europe/paris]", "[+05:30]", "[-0800]", "[!UTC]",
  "[Asia/Calcutta]", "[u-ca=hebrew]", "[!u-ca=hebrew]", "[u-ca=iso8601]", "[UTC][u-ca=japanese]",
  "[u-ca=hebrew][u-ca=gregory]", "[u-ca=hebrew][!u-ca=gregory]", "[foo=bar]", "[!foo=bar]",
  "[UTC][foo=bar][u-ca=chinese]", "[Etc/GMT+5]", "[Nowhere/Land]", "[+05:30:15]", "[u-ca=islamic]",
  "[u-ca=islamic-rgsa]", "[u-ca=ISLAMICC]", "[u-ca=ethiopic-amete-alem]", "[u-ca=gregory-true]",
  "[u-ca=x]", "[a=b]", "[_x=yz]", "[x-y=abc-def]", "[UTC", "UTC]", "[]", "[u-ca=]"];

const strings = new Set();
for (const d of dates) strings.add(d);
for (const t of times) { strings.add(t.slice(1)); strings.add(t); strings.add("2020-06-15" + t); }
for (const o of offsets) { strings.add("2020-06-15T12:00" + o); strings.add("12:00" + o); }
for (const a of annotations) {
  strings.add("2020-06-15T12:00" + a);
  strings.add("2020-06-15T12:00Z" + a);
  strings.add("2020-06-15T12:00+01:00" + a);
  strings.add("2020-06-15" + a);
  strings.add("2020-06" + a);
  strings.add("06-15" + a);
  strings.add("12:00" + a);
}
for (const s of ["", " ", "x", "2020-06-15T12:00:00Z[UTC]x", "2020-06-15T12:00:00Z junk",
  "é2020-06-15", "2020-06-15é", "2020-06-15T12:00[Europe/Paris]ā", "1976-11-18T15:23",
  "1976-11-18T15:23:30.1+01:00[Europe/Vienna][u-ca=gregory]", "2019-12-31T23:59:59.999999999Z",
  "-271821-04-20T00:00Z", "+275760-09-13T00:00Z", "-271821-04-19T23:59:59.999999999Z",
  "+275760-09-13T00:00:00.000000001Z", "-271821-04-20T00:00+01:00", "+275760-09-13T00:00-01:00",
  "2020-03-08T02:30[America/Los_Angeles]", "2020-11-01T01:30[America/Los_Angeles]",
  "2020-11-01T01:30-07:00[America/Los_Angeles]", "2020-11-01T01:30-08:00[America/Los_Angeles]",
  "2020-11-01T01:30-09:00[America/Los_Angeles]", "2020-11-01T01:30+00:00[America/Los_Angeles]",
  "1970-01-01T00:00+00:00:01[UTC]", "1883-11-18T12:00-07:52:58[America/Los_Angeles]",
  "1883-11-18T12:00-07:53[America/Los_Angeles]", "2020-06-15T12:00[America/Argentina/ComodRivadavia]",
  "2020-06-15T12:00[US/Pacific-New]", "2020-06-15T12:00[Etc/Unknown]", "2020-06-15T12:00[GMT]",
  "2020-06-15T12:00[etc/gmt]", "2020-06-15T12:00[UCT]", "2020-06-15T12:00[EST5EDT]"]) strings.add(s);

const durationStrings = ["P1Y", "P1Y2M3W4DT5H6M7.123456789S", "-P1Y2M3W4DT5H6M7.123456789S",
  "+P1D", "PT1.5H", "PT1.5M", "PT1.5S", "PT0.000000001S", "PT1.1234567891S", "P1DT", "PT", "P", "",
  "P1W1D", "P1D1W", "P1Y1Y", "PT1H1H", "p1y", "PT1,5H", "PT1.5H1M", "PT1H1.5M", "P4294967295Y",
  "P4294967296Y", "P9007199254740991D", "PT9007199254740991S", "PT9007199254740992S",
  "PT18446744073709551616S", "PT18446744073709551615S", "P1Y-1M", "-PT0S", "PT0S", "P0D",
  "PT10000000000000000000H", "PT1.999999999H", "PT100000000000000000.5H", "P1.5D", "PT.5S", "P1M1Y",
  "PT1S1M", "PT1H2M3S4", "−1D", "-P", "P-1D"];

for (const s of strings) {
  for (const t of types) run("static:" + t + ".from", null, [s]);
}
for (const s of durationStrings) run("static:Duration.from", null, [s]);

// ==== Durations ====

const durations = ["PT0S", "P1Y", "-P1Y", "P1M", "P1W", "P1D", "PT1H", "PT1M", "PT1S", "PT0.001S",
  "PT0.000001S", "PT0.000000001S", "P1Y2M3W4DT5H6M7.123456789S", "-P1Y2M3W4DT5H6M7.123456789S",
  "P2DT12H", "-P2DT12H", "PT36H", "PT1439M", "PT86399.999999999S", "P400Y", "P13M", "P5W", "P45D",
  "PT100000H", "-PT100000H", "PT1.5S", "PT2.5S", "-PT2.5S", "PT0.5S", "PT59M59.5S", "P1DT0.5S",
  "PT9007199254740991S", "-PT9007199254740991.999999999S", "P1Y1D", "P3M15D", "-P3M15D", "P10Y10M10D"];
const bigDurations = [{ microseconds: 9007199254740991e6 }, { nanoseconds: 1e25 },
  { milliseconds: 9007199254740991e3, microseconds: 999999 }, { hours: 2501999792983 },
  { days: 104249991374 }, { weeks: 14892855910 }, { years: 4294967295 }, { years: 4294967296 },
  { seconds: 1.5 }, { seconds: -1, minutes: 1 }, { hours: 1e20 }, { nanoseconds: 1.5e300 },
  { microseconds: -1e40 }, { days: 1, seconds: -0 }];

for (const d of durationStrings) run("static:Duration.from", null, [d]);
for (const f of bigDurations) run("static:Duration.from", null, [f]);
for (const args of [[], [1], [1, 2, 3, 4, 5, 6, 7, 8, 9, 10], [-1, -2], [1, -1], [1.5], [{ $n: "NaN" }],
  [{ $n: "Infinity" }], [0, 0, 0, 0, 0, 0, 0, 0, 1e30], [0, 0, 0, 0, 0, 0, 0, 0, 0, 1e40],
  [0, 0, 0, 0, 0, 0, 0, 9.3e18], [4294967296], [0, 0, 0, 0, 0, 0, 9007199254740991, 999, 999, 999],
  [0, 0, 0, 0, 0, 0, 9007199254740991, 999, 999, 1000], ["1", "2"], [U, 1], [{ $n: "-0" }, 1]]) {
  run("new:Duration", null, args);
}

const units = ["year", "month", "week", "day", "hour", "minute", "second", "millisecond", "microsecond",
  "nanosecond"];
const modes = ["ceil", "floor", "expand", "trunc", "halfCeil", "halfFloor", "halfExpand", "halfTrunc",
  "halfEven"];
const relatives = [U, "2020-01-31", "2020-02-29", "2019-12-31[u-ca=hebrew]", "2020-01-01[u-ca=chinese]",
  "2020-03-08T01:30[America/Los_Angeles]", "2020-11-01T00:30-07:00[America/Los_Angeles]",
  "2020-01-31T12:00+05:30[Asia/Kolkata]", "2020-01-01T00:00[UTC]", "1970-01-01T00:00+01:00[+01:00]",
  { year: 2020, month: 2, day: 29 }, { year: 2020, month: 3, day: 8, hour: 2, timeZone: "America/New_York" },
  { year: 2020, monthCode: "M05L", day: 1, calendar: "hebrew" }];

for (const a of durations) {
  for (const g of ["years", "months", "weeks", "days", "hours", "minutes", "seconds", "milliseconds",
    "microseconds", "nanoseconds", "sign", "blank"]) {
    run("get:Duration." + g, { $Duration: a }, []);
  }
  run("method:Duration.negated", { $Duration: a }, []);
  run("method:Duration.abs", { $Duration: a }, []);
  run("method:Duration.toJSON", { $Duration: a }, []);
  for (const b of durations) {
    run("method:Duration.add", { $Duration: a }, [b]);
    run("method:Duration.subtract", { $Duration: a }, [b]);
  }
  for (const w of [{ years: 2 }, { hours: -1 }, { nanoseconds: 999 }, {}, { foo: 1 }, { days: 1.5 }]) {
    run("method:Duration.with", { $Duration: a }, [w]);
  }
  for (const rel of relatives) {
    for (const u of units) run("method:Duration.total", { $Duration: a }, [{ unit: u, relativeTo: rel }]);
    for (const s of units) {
      run("method:Duration.round", { $Duration: a }, [{ smallestUnit: s, relativeTo: rel }]);
      for (const l of ["auto", "year", "month", "week", "day", "hour", "second"]) {
        run("method:Duration.round", { $Duration: a }, [{ smallestUnit: s, largestUnit: l, relativeTo: rel }]);
      }
    }
    for (const m of modes) {
      for (const [s, inc] of [["day", 1], ["hour", 6], ["minute", 15], ["second", 30], ["millisecond", 250],
        ["nanosecond", 10], ["year", 2], ["month", 3], ["week", 2]]) {
        run("method:Duration.round", { $Duration: a },
          [{ smallestUnit: s, roundingIncrement: inc, roundingMode: m, relativeTo: rel }]);
      }
    }
    for (const b of durations.slice(0, 16)) {
      run("static:Duration.compare", null, [{ $Duration: a }, { $Duration: b }, { relativeTo: rel }]);
    }
  }
  for (const opts of [U, {}, { fractionalSecondDigits: 0 }, { fractionalSecondDigits: 3 },
    { fractionalSecondDigits: 9 }, { fractionalSecondDigits: "auto" }, { fractionalSecondDigits: 10 },
    { fractionalSecondDigits: 2.7 }, { smallestUnit: "second" }, { smallestUnit: "millisecond" },
    { smallestUnit: "microseconds" }, { smallestUnit: "nanosecond" }, { smallestUnit: "minute" },
    { smallestUnit: "day" }, { smallestUnit: "second", roundingMode: "ceil" },
    { fractionalSecondDigits: 1, roundingMode: "halfExpand" }, { fractionalSecondDigits: 4, roundingMode: "floor" }]) {
    run("method:Duration.toString", { $Duration: a }, [opts]);
  }
}
for (const opts of [U, "hour", "days", "bogus", {}, { smallestUnit: "hour", largestUnit: "minute" },
  { roundingIncrement: 0 }, { smallestUnit: "hour", roundingIncrement: 5 }, { smallestUnit: "hour", roundingIncrement: 24 },
  { smallestUnit: "minute", roundingIncrement: 1e9 + 1 }, { smallestUnit: "day", roundingIncrement: 2, largestUnit: "week" },
  { largestUnit: "auto" }, { largestUnit: "auto", smallestUnit: "auto" }, { smallestUnit: "hour", roundingMode: "bogus" }]) {
  run("method:Duration.round", { $Duration: "P1DT12H30M" }, [opts]);
  run("method:Duration.total", { $Duration: "P1DT12H30M" }, [opts]);
}

// ==== Plain dates, in every calendar ====

const calendars = ["buddhist", "chinese", "coptic", "dangi", "ethioaa", "ethiopic", "gregory", "hebrew",
  "indian", "islamic-civil", "islamic-tbla", "islamic-umalqura", "iso8601", "japanese", "persian", "roc"];
const isoDates = ["2020-01-31", "2020-02-29", "2021-02-28", "2019-12-31", "1970-01-01", "2000-06-15",
  "1582-10-15", "0001-01-01", "-000001-12-31", "2023-03-22", "2024-09-10", "+275760-09-13", "-271821-04-19"];
const dateGetters = ["era", "eraYear", "year", "month", "monthCode", "day", "dayOfWeek", "dayOfYear",
  "weekOfYear", "yearOfWeek", "daysInWeek", "daysInMonth", "daysInYear", "monthsInYear", "inLeapYear",
  "calendarId"];
const addDurations = ["P1D", "-P1D", "P1M", "-P1M", "P1Y", "-P1Y", "P1Y1M1D", "P13M", "P5W", "P400D",
  "PT36H", "-PT36H", "PT23H59M59.999999999S", "P1000Y", "P100000Y", "-P1000000Y", "P1M29D"];
const overflows = [U, { overflow: "reject" }, { overflow: "constrain" }, { overflow: "bogus" }, "x", null];
const diffOptions = [U, {}, { largestUnit: "year" }, { largestUnit: "month" }, { largestUnit: "week" },
  { largestUnit: "day" }, { largestUnit: "hour" }, { smallestUnit: "month" }, { smallestUnit: "year" },
  { smallestUnit: "week", roundingMode: "halfExpand" }, { largestUnit: "month", smallestUnit: "month", roundingMode: "ceil" },
  { largestUnit: "year", smallestUnit: "day", roundingIncrement: 5, roundingMode: "halfEven" },
  { largestUnit: "month", smallestUnit: "year" }, { smallestUnit: "day", roundingIncrement: 2 },
  { largestUnit: "auto", smallestUnit: "week", roundingMode: "floor" }, { roundingMode: "expand" },
  { smallestUnit: "hour" }, { largestUnit: "bogus" }];

for (const cal of calendars) {
  const ds = isoDates.map(d => ({ $PlainDate: d + "[u-ca=" + cal + "]" }));
  for (const d of ds) {
    for (const g of dateGetters) run("get:PlainDate." + g, d, []);
    for (const opts of [U, { calendarName: "auto" }, { calendarName: "never" }, { calendarName: "critical" },
      { calendarName: "x" }, "x"]) {
      run("method:PlainDate.toString", d, [opts]);
    }
    run("method:PlainDate.toPlainYearMonth", d, []);
    run("method:PlainDate.toPlainMonthDay", d, []);
    for (const t of [U, "12:30", { hour: 25 }]) run("method:PlainDate.toPlainDateTime", d, [t]);
    for (const dur of addDurations) {
      for (const o of overflows.slice(0, 2)) {
        run("method:PlainDate.add", d, [dur, o]);
        run("method:PlainDate.subtract", d, [dur, o]);
      }
    }
    for (const w of [{ day: 31 }, { day: 30 }, { month: 2 }, { month: 13 }, { monthCode: "M02" },
      { monthCode: "M05L" }, { monthCode: "M13" }, { year: 2021 }, { year: 5784 }, { era: "ce", eraYear: 2 },
      { era: "reiwa", eraYear: 1 }, { eraYear: 5 }, {}, { calendar: "iso8601" }, { day: 1.9 }, { month: 0 }]) {
      run("method:PlainDate.with", d, [w]);
      run("method:PlainDate.with", d, [w, { overflow: "reject" }]);
    }
    for (const c2 of ["iso8601", "hebrew", "bogus"]) run("method:PlainDate.withCalendar", d, [c2]);
  }
  for (const a of ds.slice(0, 10)) {
    for (const b of ds.slice(0, 10)) {
      for (const o of diffOptions) {
        run("method:PlainDate.until", a, [b, o]);
        run("method:PlainDate.since", a, [b, o]);
      }
      run("static:PlainDate.compare", null, [a, b]);
      run("method:PlainDate.equals", a, [b]);
    }
  }
  for (const f of [{ year: 2020, month: 2, day: 30 }, { year: 5784, monthCode: "M05L", day: 1 },
    { era: "ce", eraYear: 2020, month: 1, day: 1 }, { era: "bce", eraYear: 1, month: 1, day: 1 },
    { year: 2020, month: 13, day: 1 }, { year: 2020, day: 1 }, { month: 1, day: 1 },
    { year: 1e10, month: 1, day: 1 }, { year: 2020, month: -1, day: 1 }, { year: 2020, month: 1, day: 300 },
    { year: 4660, monthCode: "M06L", day: 1 }, { year: 2020, monthCode: "M00L", day: 1 }]) {
    const o = Object.assign({ calendar: cal }, f);
    for (const ov of overflows) run("static:PlainDate.from", null, [o, ov]);
    run("static:PlainYearMonth.from", null, [o]);
    run("static:PlainMonthDay.from", null, [o]);
    run("static:PlainMonthDay.from", null, [o, { overflow: "reject" }]);
  }
  for (const args of [[2020, 1, 1], [2020, 2, 30], [1e6, 1, 1], [275760, 9, 13], [275760, 9, 14], ["2020", 1.9, 1]]) {
    run("new:PlainDate", null, [...args, cal]);
    run("new:PlainDateTime", null, [...args, 12, 0, 0, 0, 0, 0, cal]);
    run("new:PlainYearMonth", null, [args[0], args[1], cal]);
    run("new:PlainMonthDay", null, [args[1], args[2], cal]);
  }
}
for (const args of [[], [2020], [2020, 1, 1, "bogus"], [2020, 1, 1, 42], [2020, 1, 1, "ISLAMICC"],
  [2020, 1, 1, "islamic"], [2020, 1, 1, "islamic-rgsa"], [{ $n: "Infinity" }, 1, 1], [-271821, 4, 19],
  [-271821, 4, 18], [2020, 0, 1], [2020, 1, 0]]) {
  run("new:PlainDate", null, args);
}

// ==== Plain times and date-times ====

const timeStrings = ["00:00", "12:34:56.789123456", "23:59:59.999999999", "01:00", "13:30:15.5"];
const timeDurations = ["PT1H", "-PT1H", "PT25H", "PT0.000000001S", "-PT0.000000001S", "P1D", "PT1439M",
  "PT86400.5S", "P1Y", "PT9007199254740991S"];
const timeUnits = ["hour", "minute", "second", "millisecond", "microsecond", "nanosecond"];
for (const t of timeStrings) {
  const pt = { $PlainTime: t };
  for (const g of timeUnits) run("get:PlainTime." + g, pt, []);
  for (const d of timeDurations) {
    run("method:PlainTime.add", pt, [d]);
    run("method:PlainTime.subtract", pt, [d]);
  }
  for (const u of timeUnits.concat(["day", "auto"])) {
    for (const m of ["halfExpand", "floor", "ceil", "halfEven", "trunc"]) {
      for (const inc of [1, 2, 5, 30, 1000, 24]) {
        run("method:PlainTime.round", pt, [{ smallestUnit: u, roundingMode: m, roundingIncrement: inc }]);
      }
    }
  }
  for (const u of [U, "hour", "bogus", 5]) run("method:PlainTime.round", pt, [u]);
  for (const b of timeStrings) {
    for (const o of [U, { largestUnit: "hour" }, { largestUnit: "minute", smallestUnit: "second" },
      { smallestUnit: "minute", roundingMode: "halfExpand", roundingIncrement: 15 }, { largestUnit: "day" },
      { smallestUnit: "nanosecond", roundingIncrement: 7 }, { largestUnit: "second", roundingMode: "ceil" }]) {
      run("method:PlainTime.until", pt, [b, o]);
      run("method:PlainTime.since", pt, [b, o]);
    }
    run("static:PlainTime.compare", null, [pt, b]);
    run("method:PlainTime.equals", pt, [b]);
  }
  for (const w of [{ hour: 5 }, { minute: 70 }, { second: -1 }, { nanosecond: 1000 }, {}, { foo: 1 }, { hour: 1.9 }]) {
    run("method:PlainTime.with", pt, [w]);
    run("method:PlainTime.with", pt, [w, { overflow: "reject" }]);
  }
  for (const o of [U, { fractionalSecondDigits: 2 }, { smallestUnit: "minute" }, { smallestUnit: "hour" },
    { fractionalSecondDigits: 0, roundingMode: "halfExpand" }, { smallestUnit: "microsecond", roundingMode: "ceil" }]) {
    run("method:PlainTime.toString", pt, [o]);
  }
}
for (const args of [[], [24], [23, 59, 59, 999, 999, 999], [1.5, 2.5], [-1], [0, 60], [{ $n: "NaN" }], ["12"]]) {
  run("new:PlainTime", null, args);
}
for (const f of [{ hour: 12 }, { hour: 25, minute: 61 }, {}, { hour: -1 }, { second: 60 }, { millisecond: 1e3 }]) {
  run("static:PlainTime.from", null, [f]);
  run("static:PlainTime.from", null, [f, { overflow: "reject" }]);
}

const dateTimes = ["2020-01-31T12:30:15.123456789", "2020-02-29T00:00", "2019-12-31T23:59:59.999999999",
  "1970-01-01T00:00", "2020-03-08T02:30", "+275760-09-13T00:00", "-271821-04-19T00:00:00.000000001"];
for (const cal of ["iso8601", "gregory", "hebrew", "chinese", "japanese", "islamic-umalqura"]) {
  const dts = dateTimes.map(d => ({ $PlainDateTime: d + "[u-ca=" + cal + "]" }));
  for (const dt of dts) {
    for (const g of dateGetters.concat(timeUnits)) run("get:PlainDateTime." + g, dt, []);
    for (const d of addDurations.concat(timeDurations)) {
      run("method:PlainDateTime.add", dt, [d]);
      run("method:PlainDateTime.subtract", dt, [d, { overflow: "reject" }]);
    }
    for (const u of timeUnits.concat(["day"])) {
      for (const m of ["halfExpand", "floor", "ceil", "halfEven"]) {
        for (const inc of [1, 3, 10, 12]) {
          run("method:PlainDateTime.round", dt, [{ smallestUnit: u, roundingMode: m, roundingIncrement: inc }]);
        }
      }
    }
    for (const o of [U, { fractionalSecondDigits: 3, calendarName: "never" }, { smallestUnit: "minute" },
      { smallestUnit: "second", roundingMode: "halfExpand" }, { fractionalSecondDigits: 7, roundingMode: "ceil" },
      { smallestUnit: "hour" }]) {
      run("method:PlainDateTime.toString", dt, [o]);
    }
    for (const w of [{ hour: 25 }, { day: 31, hour: 1 }, { monthCode: "M02", day: 29 }, { year: 2021, nanosecond: 5 }, {}]) {
      run("method:PlainDateTime.with", dt, [w]);
      run("method:PlainDateTime.with", dt, [w, { overflow: "reject" }]);
    }
    for (const t of [U, "12:00", { hour: 3 }]) run("method:PlainDateTime.withPlainTime", dt, [t]);
    run("method:PlainDateTime.withCalendar", dt, ["gregory"]);
    run("method:PlainDateTime.toPlainDate", dt, []);
    run("method:PlainDateTime.toPlainTime", dt, []);
    for (const tz of ["UTC", "America/Los_Angeles", "+05:30", "Europe/London"]) {
      for (const dis of [U, { disambiguation: "earlier" }, { disambiguation: "later" }, { disambiguation: "reject" }]) {
        run("method:PlainDateTime.toZonedDateTime", dt, [tz, dis]);
      }
    }
    // ICU4X differences in months a month at a time: across the whole range
    // that takes Node minutes, so only ISO's is recorded there.
    for (const b of cal === "iso8601" ? dts : dts.slice(0, 5)) {
      if (cal !== "iso8601" && dts.indexOf(dt) >= 5) break;
      for (const o of diffOptions.concat([{ largestUnit: "hour", smallestUnit: "minute", roundingIncrement: 30 },
        { smallestUnit: "second", roundingMode: "halfExpand" }, { largestUnit: "nanosecond" }])) {
        run("method:PlainDateTime.until", dt, [b, o]);
        run("method:PlainDateTime.since", dt, [b, o]);
      }
      run("static:PlainDateTime.compare", null, [dt, b]);
      run("method:PlainDateTime.equals", dt, [b]);
    }
  }
}

// ==== Year-months and month-days ====

for (const cal of calendars) {
  const yms = ["2020-01", "2020-02", "2019-12", "1970-06", "2023-03"].map(d =>
    ({ $PlainYearMonth: d + "-15[u-ca=" + cal + "]" }));
  for (const ym of yms) {
    for (const g of ["era", "eraYear", "year", "month", "monthCode", "daysInMonth", "daysInYear", "monthsInYear",
      "inLeapYear", "calendarId"]) run("get:PlainYearMonth." + g, ym, []);
    for (const d of ["P1M", "-P1M", "P1Y", "P13M", "-P1Y1M", "P1D", "PT1H", "P1W"]) {
      run("method:PlainYearMonth.add", ym, [d]);
      run("method:PlainYearMonth.subtract", ym, [d, { overflow: "reject" }]);
    }
    for (const w of [{ month: 13 }, { monthCode: "M05L" }, { year: 2021 }, {}, { day: 5 }]) {
      run("method:PlainYearMonth.with", ym, [w]);
      run("method:PlainYearMonth.with", ym, [w, { overflow: "reject" }]);
    }
    for (const f of [{ day: 1 }, { day: 31 }, {}, 5]) run("method:PlainYearMonth.toPlainDate", ym, [f]);
    for (const o of [U, { calendarName: "always" }]) run("method:PlainYearMonth.toString", ym, [o]);
    for (const b of yms) {
      for (const o of [U, { largestUnit: "month" }, { smallestUnit: "year", roundingMode: "halfExpand" },
        { largestUnit: "day" }, { smallestUnit: "week" }, { roundingIncrement: 2, smallestUnit: "month" }]) {
        run("method:PlainYearMonth.until", ym, [b, o]);
        run("method:PlainYearMonth.since", ym, [b, o]);
      }
      run("static:PlainYearMonth.compare", null, [ym, b]);
    }
  }
  const mds = ["01-01", "02-29", "12-31", "06-15"].map(d => ({ $PlainMonthDay: "2020-" + d + "[u-ca=" + cal + "]" }));
  for (const md of mds) {
    for (const g of ["monthCode", "day", "calendarId"]) run("get:PlainMonthDay." + g, md, []);
    for (const w of [{ day: 31 }, { monthCode: "M02", day: 30 }, { month: 2 }, { year: 2021 }, {}]) {
      run("method:PlainMonthDay.with", md, [w]);
      run("method:PlainMonthDay.with", md, [w, { overflow: "reject" }]);
    }
    for (const f of [{ year: 2021 }, { year: 2020 }, {}, { era: "ce", eraYear: 1 }]) run("method:PlainMonthDay.toPlainDate", md, [f]);
    for (const o of [U, { calendarName: "always" }]) run("method:PlainMonthDay.toString", md, [o]);
    run("method:PlainMonthDay.equals", md, [mds[0]]);
  }
}

// ==== Instants and zoned date-times ====

const zones = ["UTC", "America/Los_Angeles", "America/New_York", "Europe/London", "Europe/Dublin",
  "Australia/Lord_Howe", "Asia/Kolkata", "Asia/Calcutta", "Pacific/Apia", "America/Sao_Paulo",
  "Africa/Casablanca", "Asia/Tehran", "America/St_Johns", "Antarctica/Troll", "+05:30", "-00:01",
  "Europe/Paris", "America/Argentina/Buenos_Aires", "Asia/Gaza", "Pacific/Kiritimati"];
const instants = ["1970-01-01T00:00Z", "2020-03-08T10:00Z", "2020-11-01T08:30Z", "2011-12-30T10:00Z",
  "1900-01-01T00:00Z", "2038-01-19T03:14:08Z", "2100-06-01T00:00Z", "-271821-04-20T00:00Z",
  "+275760-09-13T00:00Z", "2020-06-15T12:34:56.789123456Z"];
for (const i of instants) {
  const ins = { $Instant: i };
  for (const g of ["epochMilliseconds", "epochNanoseconds"]) run("get:Instant." + g, ins, []);
  for (const d of ["PT1H", "-PT1H", "P1D", "PT24H", "PT0.000000001S", "PT9007199254740991S"]) {
    run("method:Instant.add", ins, [d]);
    run("method:Instant.subtract", ins, [d]);
  }
  for (const u of timeUnits.concat(["day"])) {
    for (const m of ["halfExpand", "floor", "ceil", "trunc", "halfEven"]) {
      for (const inc of [1, 5, 15, 86400, 1440]) {
        run("method:Instant.round", ins, [{ smallestUnit: u, roundingMode: m, roundingIncrement: inc }]);
      }
    }
  }
  for (const o of [U, { timeZone: "America/Los_Angeles" }, { timeZone: "+05:30", fractionalSecondDigits: 3 },
    { smallestUnit: "minute" }, { smallestUnit: "hour" }, { fractionalSecondDigits: 0, roundingMode: "ceil" },
    { timeZone: "Asia/Kolkata", smallestUnit: "second", roundingMode: "halfExpand" }, { timeZone: "Nowhere" }]) {
    run("method:Instant.toString", ins, [o]);
  }
  for (const tz of zones) run("method:Instant.toZonedDateTimeISO", ins, [tz]);
  for (const b of instants) {
    for (const o of [U, { largestUnit: "hour" }, { largestUnit: "day" }, { smallestUnit: "minute", roundingMode: "halfExpand" },
      { largestUnit: "second", smallestUnit: "millisecond", roundingIncrement: 250 }]) {
      run("method:Instant.until", ins, [b, o]);
      run("method:Instant.since", ins, [b, o]);
    }
    run("static:Instant.compare", null, [ins, b]);
    run("method:Instant.equals", ins, [b]);
  }
}
for (const n of ["0", "-1", "8640000000000000000000", "8640000000000000000001", "-8640000000000000000000",
  "-8640000000000000000001", "1700000000123456789"]) {
  run("new:Instant", null, [{ $big: n }]);
  run("static:Instant.fromEpochNanoseconds", null, [{ $big: n }]);
}
run("new:Instant", null, [5]);
for (const ms of [0, -1, 1.5, 8.64e15, 8.64e15 + 1, -8.64e15, { $n: "NaN" }, "5", 9.3e18]) {
  run("static:Instant.fromEpochMilliseconds", null, [ms]);
}

const zdtGetters = dateGetters.concat(timeUnits, ["epochMilliseconds", "epochNanoseconds", "offset",
  "offsetNanoseconds", "timeZoneId", "hoursInDay"]);
for (const tz of zones) {
  const seeds = [];
  let z = Temporal.ZonedDateTime.from({ year: 2019, month: 6, day: 1, timeZone: tz });
  seeds.push(z);
  for (let i = 0; i < 6; i++) {
    const n = z.getTimeZoneTransition("next");
    if (!n) break;
    seeds.push(n, n.subtract({ nanoseconds: 1 }), n.add({ minutes: 30 }));
    z = n;
  }
  for (const y of [1850, 1900, 1950, 2011, 2040, 2100]) {
    seeds.push(Temporal.ZonedDateTime.from({ year: y, month: 12, day: 30, timeZone: tz }));
  }
  const specs = seeds.map(s => ({ $ZonedDateTime: s.toString() }));
  for (const zs of specs) {
    for (const g of zdtGetters) run("get:ZonedDateTime." + g, zs, []);
    run("method:ZonedDateTime.startOfDay", zs, []);
    for (const dir of ["next", "previous", { direction: "next" }, U, "bogus"]) {
      run("method:ZonedDateTime.getTimeZoneTransition", zs, [dir]);
    }
    for (const d of ["P1D", "-P1D", "PT24H", "P1M", "-P1Y", "PT30M", "P1DT1H", "-PT1H"]) {
      run("method:ZonedDateTime.add", zs, [d]);
      run("method:ZonedDateTime.subtract", zs, [d, { overflow: "reject" }]);
    }
    for (const u of ["day", "hour", "minute", "second", "nanosecond"]) {
      for (const m of ["halfExpand", "floor", "ceil"]) {
        run("method:ZonedDateTime.round", zs, [{ smallestUnit: u, roundingMode: m }]);
      }
    }
    run("method:ZonedDateTime.round", zs, [{ smallestUnit: "minute", roundingIncrement: 15 }]);
    for (const o of [U, { offset: "never" }, { timeZoneName: "never", calendarName: "always" },
      { timeZoneName: "critical" }, { smallestUnit: "minute" }, { fractionalSecondDigits: 2, roundingMode: "halfExpand" },
      { smallestUnit: "hour" }]) {
      run("method:ZonedDateTime.toString", zs, [o]);
    }
    for (const w of [{ hour: 2, minute: 30 }, { day: 1 }, { offset: "+00:00" }, { hour: 1, minute: 30, offset: "-07:00" },
      { hour: 1, minute: 30, offset: "-08:00" }, {}]) {
      for (const o of [U, { disambiguation: "later" }, { offset: "use" }, { offset: "ignore" }, { offset: "reject" },
        { disambiguation: "reject", offset: "prefer" }]) {
        run("method:ZonedDateTime.with", zs, [w, o]);
      }
    }
    for (const t of [U, "02:30", "00:00"]) run("method:ZonedDateTime.withPlainTime", zs, [t]);
    for (const tz2 of ["UTC", "Asia/Kolkata"]) run("method:ZonedDateTime.withTimeZone", zs, [tz2]);
    run("method:ZonedDateTime.withCalendar", zs, ["hebrew"]);
    run("method:ZonedDateTime.toInstant", zs, []);
    run("method:ZonedDateTime.toPlainDateTime", zs, []);
    for (const b of specs.slice(0, 8)) {
      for (const o of [U, { largestUnit: "day" }, { largestUnit: "year", smallestUnit: "hour" },
        { largestUnit: "month", smallestUnit: "day", roundingMode: "halfExpand" }, { smallestUnit: "minute" },
        { largestUnit: "week" }]) {
        run("method:ZonedDateTime.until", zs, [b, o]);
        run("method:ZonedDateTime.since", zs, [b, o]);
      }
      run("static:ZonedDateTime.compare", null, [zs, b]);
      run("method:ZonedDateTime.equals", zs, [b]);
    }
  }
  for (const s of ["2020-03-08T02:30", "2020-11-01T01:30", "2011-12-30T12:00", "2020-06-15T00:00", "1880-01-01T00:00"]) {
    for (const off of ["", "Z", "-07:00", "-08:00", "+00:00", "+05:30"]) {
      for (const o of [U, { disambiguation: "earlier" }, { disambiguation: "later" }, { disambiguation: "reject" },
        { offset: "use" }, { offset: "ignore" }, { offset: "prefer" }]) {
        run("static:ZonedDateTime.from", null, [s + off + "[" + tz + "]", o]);
      }
    }
    const [date, time] = s.split("T");
    const [y, m, d] = date.split("-").map(Number);
    const [hh, mm] = time.split(":").map(Number);
    for (const o of [U, { disambiguation: "later" }, { offset: "ignore" }]) {
      run("static:ZonedDateTime.from", null, [{ year: y, month: m, day: d, hour: hh, minute: mm, timeZone: tz }, o]);
      run("static:ZonedDateTime.from", null, [{ year: y, month: m, day: d, hour: hh, minute: mm, offset: "-08:00", timeZone: tz }, o]);
    }
  }
  for (const n of ["0", "1583020800000000000", "-8640000000000000000000"]) {
    run("new:ZonedDateTime", null, [{ $big: n }, tz]);
    run("new:ZonedDateTime", null, [{ $big: n }, tz, "japanese"]);
  }
  for (const d of ["2020-03-08", "2020-11-01", "2011-12-30", "2011-12-31"]) {
    run("method:PlainDate.toZonedDateTime", { $PlainDate: d }, [tz]);
    run("method:PlainDate.toZonedDateTime", { $PlainDate: d }, [{ timeZone: tz, plainTime: "02:30" }]);
  }
}
// ==== Every zone Temporal names ====

const allZones = fs.readFileSync(path.join(__dirname, "..", "data", "temporalzones.bin"), "utf8")
  .split("\n").slice(1).filter(l => l).map(l => l.split(" ")[0]);
for (const tz of allZones) {
  for (const id of [tz, tz.toUpperCase(), tz.toLowerCase()]) {
    run("get:ZonedDateTime.timeZoneId", { $ZonedDateTime: "2020-01-01T00:00Z[" + id + "]" }, []);
  }
  let fwd, back;
  try {
    fwd = Temporal.ZonedDateTime.from({ year: 1850, month: 1, day: 1, timeZone: tz });
    back = Temporal.ZonedDateTime.from({ year: 2045, month: 1, day: 1, timeZone: tz });
  } catch (e) {
    continue;
  }
  for (const [start, dir] of [[fwd, "next"], [back, "previous"]]) {
    let z = start;
    for (let i = 0; i < 12 && z; i++) {
      const s = { $ZonedDateTime: z.toString() };
      run("method:ZonedDateTime.getTimeZoneTransition", s, [dir]);
      run("get:ZonedDateTime.hoursInDay", s, []);
      run("method:ZonedDateTime.startOfDay", s, []);
      z = z.getTimeZoneTransition(dir);
      if (z) {
        run("get:ZonedDateTime.offset", { $ZonedDateTime: z.subtract({ nanoseconds: 1 }).toString() }, []);
        run("static:ZonedDateTime.from", null, [z.toPlainDateTime().toString() + "[" + tz + "]"]);
      }
    }
  }
  for (const y of [1800, 1900, 1937, 1970, 1990, 2010, 2030, 2100, 2400]) {
    for (const s of [y + "-01-15T12:00", y + "-07-15T12:00"]) {
      run("static:ZonedDateTime.from", null, [s + "[" + tz + "]"]);
    }
  }
}

for (const [t, v] of [["PlainDate", "2020-01-31[u-ca=hebrew]"], ["PlainDate", "-000001-01-01"],
  ["PlainTime", "12:34:56.5"], ["PlainDateTime", "2020-01-31T00:00[u-ca=japanese]"],
  ["PlainYearMonth", "2020-01"], ["PlainYearMonth", "2020-01-15[u-ca=chinese]"], ["PlainMonthDay", "01-31"],
  ["PlainMonthDay", "2020-01-31[u-ca=hebrew]"], ["Instant", "2020-01-01T00:00:00.1Z"],
  ["ZonedDateTime", "2020-01-01T00:00[Asia/Calcutta][u-ca=roc]"]]) {
  run("method:" + t + ".toJSON", { ["$" + t]: v }, []);
}

run("new:ZonedDateTime", null, [{ $big: "0" }, 5]);
run("new:ZonedDateTime", null, [{ $big: "0" }, "2020-01-01T00:00Z"]);
run("new:ZonedDateTime", null, [{ $big: "0" }, "Asia/Calcutta"]);

fs.writeFileSync(path.join(__dirname, "temporal_node.txt.gz"),
  zlib.gzipSync(`# Node ${process.version}, ICU ${process.versions.icu}, tz ${process.versions.tz}\n` +
    lines.join("\n") + "\n", { level: 9 }));
console.log(lines.length, "cases");
