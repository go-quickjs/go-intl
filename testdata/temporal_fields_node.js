// Writes testdata/temporal_fields_node.txt.gz: what Node's Temporal makes of
// calendar fields, and of adding to dates and differencing them, in every
// calendar Temporal takes.
//
//	node testdata/temporal_fields_node.js
//
// Each line is a JSON array, its result an ISO date string or the name of
// the error thrown:
//
//	["date", calendar, fields, overflow, result]     Temporal.PlainDate.from
//	["ym", calendar, fields, overflow, result]       Temporal.PlainYearMonth.from
//	["md", calendar, fields, overflow, result]       Temporal.PlainMonthDay.from
//	["add", calendar, iso, duration, overflow, result]
//	["until", calendar, iso, iso, largestUnit, [years, months, weeks, days]]
//
// The fields are only those V8 reads for the calendar, in values V8 hands
// on to temporal_rs unchanged.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const calendars = ["buddhist", "chinese", "coptic", "dangi", "ethioaa", "ethiopic", "gregory", "hebrew",
  "indian", "islamic-civil", "islamic-tbla", "islamic-umalqura", "iso8601", "japanese", "persian", "roc"];
const eras = {
  buddhist: ["be"], coptic: ["am"], ethioaa: ["aa"], ethiopic: ["am", "aa"], gregory: ["ce", "bce", "ad", "bc"],
  hebrew: ["am"], indian: ["shaka"], "islamic-civil": ["ah", "bh"], "islamic-tbla": ["ah", "bh"],
  "islamic-umalqura": ["ah", "bh"], japanese: ["reiwa", "heisei", "showa", "taisho", "meiji", "ce", "bce"],
  persian: ["ap"], roc: ["roc", "broc"],
};
const isoYears = ["2026-06-15", "1972-06-15", "1900-06-15", "2100-06-15", "-001000-06-15", "5000-06-15",
  "-271821-06-01", "+275760-06-01"];
const codes = [];
for (let m = 1; m <= 13; m++) codes.push("M" + String(m).padStart(2, "0"));
for (let m = 1; m <= 12; m++) codes.push("M" + String(m).padStart(2, "0") + "L");
const days = [1, 15, 29, 30, 31, 32];
const overflows = ["constrain", "reject"];

function iso(d) {
  return d.withCalendar("iso8601").toString();
}
function attempt(f) {
  try {
    return f();
  } catch (e) {
    return e.constructor.name;
  }
}

const lines = [`# Node ${process.version}, temporal_rs 0.2.3, icu_calendar 2.2.1`];
function emit(...a) {
  lines.push(JSON.stringify(a));
}

for (const cal of calendars) {
  const years = [...new Set(isoYears.map(s => Temporal.PlainDate.from(s).withCalendar(cal).year))];
  for (const year of years) {
    for (const overflow of overflows) {
      for (let month = 1; month <= 14; month++) for (const day of days) {
        const f = { year, month, day };
        emit("date", cal, f, overflow, attempt(() => iso(Temporal.PlainDate.from({ calendar: cal, ...f }, { overflow }))));
      }
      for (const monthCode of codes) for (const day of days) {
        const f = { year, monthCode, day };
        emit("date", cal, f, overflow, attempt(() => iso(Temporal.PlainDate.from({ calendar: cal, ...f }, { overflow }))));
        const y = { year, monthCode };
        emit("ym", cal, y, overflow, attempt(() => Temporal.PlainYearMonth.from({ calendar: cal, ...y }, { overflow }).toString({ calendarName: "never" })));
        const md = { year, monthCode, day };
        emit("md", cal, md, overflow, attempt(() => Temporal.PlainMonthDay.from({ calendar: cal, ...md }, { overflow }).toString({ calendarName: "always" })));
      }
      for (let month = 1; month <= 14; month++) {
        const y = { year, month };
        emit("ym", cal, y, overflow, attempt(() => Temporal.PlainYearMonth.from({ calendar: cal, ...y }, { overflow }).toString({ calendarName: "never" })));
        const md = { year, month, day: 30 };
        emit("md", cal, md, overflow, attempt(() => Temporal.PlainMonthDay.from({ calendar: cal, ...md }, { overflow }).toString({ calendarName: "always" })));
      }
      // Month and code together, agreeing or not.
      for (const [month, monthCode] of [[1, "M01"], [2, "M01"], [6, "M05L"], [7, "M06"], [13, "M12"]]) {
        const f = { year, month, monthCode, day: 1 };
        emit("date", cal, f, overflow, attempt(() => iso(Temporal.PlainDate.from({ calendar: cal, ...f }, { overflow }))));
      }
    }
  }
  // Month-days without a year, in the reference year.
  for (const overflow of overflows) for (const monthCode of codes) for (const day of [1, 10, 20, 26, 27, 29, 30, 31]) {
    const md = { monthCode, day };
    emit("md", cal, md, overflow, attempt(() => Temporal.PlainMonthDay.from({ calendar: cal, ...md }, { overflow }).toString({ calendarName: "always" })));
  }
  for (const era of eras[cal] || []) {
    for (const eraYear of [-5, 0, 1, 5, 30, 64, 100, 1000, 2019]) {
      for (const overflow of overflows) {
        const f = { era, eraYear, month: 1, day: 1 };
        emit("date", cal, f, overflow, attempt(() => iso(Temporal.PlainDate.from({ calendar: cal, ...f }, { overflow }))));
        const g = { era, eraYear, year: eraYear, month: 1, day: 1 };
        emit("date", cal, g, overflow, attempt(() => iso(Temporal.PlainDate.from({ calendar: cal, ...g }, { overflow }))));
      }
    }
  }
  for (const f of [{ era: "xx", eraYear: 1, month: 1, day: 1 }, { eraYear: 1, month: 1, day: 1 }]) {
    if (!eras[cal]) continue;
    emit("date", cal, f, "constrain", attempt(() => iso(Temporal.PlainDate.from({ calendar: cal, ...f }))));
  }

  // Arithmetic, from the first, middle and last days of months, leap
  // months among them.
  const starts = [];
  for (const s of ["2023-01-01", "2024-02-29", "2025-07-25", "2023-03-22", "1972-11-30", "2020-05-23", "2033-12-22",
    "1900-01-31", "2100-12-31", "1582-10-10", "-000500-03-01", "3000-08-31"]) {
    const d = Temporal.PlainDate.from(s).withCalendar(cal);
    starts.push(d, d.with({ day: 1 }), d.with({ day: d.daysInMonth }));
  }
  const durations = [];
  for (const u of ["years", "months", "weeks", "days"]) for (const n of [1, 2, 3, 12, 13, 19, 30, 400]) {
    durations.push({ [u]: n }, { [u]: -n });
  }
  durations.push({ years: 1, months: 1, days: 1 }, { years: -1, months: -1, days: -1 }, { months: 1, days: 30 },
    { years: 3, months: 25, weeks: 2, days: 40 });
  for (const d of starts) {
    for (const dur of durations) for (const overflow of overflows) {
      emit("add", cal, iso(d), dur, overflow, attempt(() => iso(d.add(dur, { overflow }))));
    }
    for (const other of starts) {
      if (d === other) continue;
      for (const largestUnit of ["year", "month", "week", "day"]) {
        const r = attempt(() => {
          const x = d.until(other, { largestUnit });
          return [x.years, x.months, x.weeks, x.days];
        });
        emit("until", cal, iso(d), iso(other), largestUnit, r);
      }
    }
  }
}

fs.writeFileSync(path.join(__dirname, "temporal_fields_node.txt.gz"), zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
