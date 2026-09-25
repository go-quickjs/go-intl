// Writes testdata/temporal_calendars_node.txt.gz: every Temporal calendar's
// years as Node's Temporal reckons them, which is ICU4X's icu_calendar
// 2.2.1, from the year ISO -3000 begins in to the one ISO 3000 ends in, and
// the years at the ends of Temporal's range.
//
//	node testdata/temporal_calendars_node.js
//
// Each line is a year:
//
//	[calendar, year, era, eraYear, start, monthsInYear, daysInYear, inLeapYear, codes, lengths]
//
// start is the day its first month begins, in days from 1970-01-01 in the
// ISO calendar; codes the month codes joined by ",", or "" where they are
// M01, M02 and so on; lengths each month's days. The era and era year are
// the first day's. A line
//
//	["japanese-day", start, [era, eraYear, ...]]
//
// has the Japanese era of each day from 1868 to 2030, which changes within
// months.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const calendars = ["buddhist", "chinese", "coptic", "dangi", "ethioaa", "ethiopic", "gregory", "hebrew",
  "indian", "islamic-civil", "islamic-tbla", "islamic-umalqura", "iso8601", "japanese", "persian", "roc"];
const epoch = Temporal.PlainDate.from("1970-01-01");
const dayOf = d => d.withCalendar("iso8601").since(epoch).days;

const lines = [`# Node ${process.version}, ICU ${process.versions.icu}, temporal_rs 0.2.3, icu_calendar 2.2.1`];
for (const cal of calendars) {
  const first = Temporal.PlainDate.from("-003000-01-01").withCalendar(cal).year;
  const last = Temporal.PlainDate.from("3000-12-31").withCalendar(cal).year;
  const min = Temporal.PlainDate.from("-271821-04-19").withCalendar(cal).year;
  const max = Temporal.PlainDate.from("+275760-09-13").withCalendar(cal).year;
  const years = [];
  for (let y = first; y <= last; y++) years.push(y);
  for (let y = min; y <= min + 2; y++) years.push(y);
  for (let y = max - 2; y <= max; y++) years.push(y);
  for (const y of years) {
    let d;
    try {
      d = Temporal.PlainDate.from({ calendar: cal, year: y, month: 1, day: 1 }, { overflow: "reject" });
    } catch (e) {
      // A year that starts before Temporal's range: its first whole month.
      continue;
    }
    const codes = [], lengths = [];
    let plain = true;
    for (let m = 1; m <= d.monthsInYear; m++) {
      let md;
      try {
        md = Temporal.PlainDate.from({ calendar: cal, year: y, month: m, day: 1 }, { overflow: "reject" });
      } catch (e) {
        codes.push("?"); lengths.push(0); plain = false;
        continue;
      }
      codes.push(md.monthCode);
      lengths.push(md.daysInMonth);
      if (md.monthCode !== "M" + String(m).padStart(2, "0")) plain = false;
    }
    lines.push(JSON.stringify([cal, y, d.era ?? null, d.eraYear ?? null, dayOf(d), d.monthsInYear,
      d.daysInYear, d.inLeapYear, plain ? "" : codes.join(","), lengths]));
  }
}

const eras = [];
let day = Temporal.PlainDate.from("1868-01-01");
const start = dayOf(day);
for (; Temporal.PlainDate.compare(day, "2031-01-01") < 0; day = day.add({ days: 1 })) {
  const j = day.withCalendar("japanese");
  eras.push(j.era, j.eraYear);
}
lines.push(JSON.stringify(["japanese-day", start, eras]));

fs.writeFileSync(path.join(__dirname, "temporal_calendars_node.txt.gz"), zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
