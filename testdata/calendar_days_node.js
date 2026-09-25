// Writes testdata/calendar_days_node.txt.gz: the date Node's DateTimeFormat
// gives every day from 1600 to 2400 in each calendar whose months are not
// the Gregorian ones, so that each calendar's arithmetic is checked day by
// day rather than at a few chosen dates.
//
//	node testdata/calendar_days_node.js
//
// The file is kept small by recording runs: a line is written only where a
// day does not follow on from the one before, in the shape [calendar,
// milliseconds into the UTC day, epoch day, fields, day of the month], the
// fields being every part but the day and the literals, "era=AH year=1445
// month=9". Each calendar is written at midnight UTC, and the astronomical
// Islamic ones, which ICU reckons partly from the moment itself, a
// millisecond before the next midnight as well.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const calendars = ["buddhist", "chinese", "coptic", "dangi", "ethioaa", "ethiopic", "hebrew", "indian",
  "islamic", "islamic-civil", "islamic-rgsa", "islamic-tbla", "islamic-umalqura", "persian"];
const late = ["islamic", "islamic-rgsa", "islamic-umalqura"];
const first = Date.UTC(1600, 0, 1) / 864e5;
const last = Date.UTC(2400, 11, 31) / 864e5;

const lines = [];
function record(calendar, offset) {
  const f = new Intl.DateTimeFormat("en-u-nu-latn", {
    calendar, timeZone: "UTC", era: "short", year: "numeric", month: "numeric", day: "numeric",
  });
  let prevFields = null;
  let prevDay = 0;
  for (let d = first; d <= last; d++) {
    const parts = f.formatToParts(d * 864e5 + offset);
    let day = 0;
    const fields = [];
    for (const p of parts) {
      if (p.type === "literal") continue;
      if (p.type === "day") day = Number(p.value);
      else fields.push(p.type + "=" + p.value);
    }
    fields.sort();
    const key = fields.join(" ");
    if (key !== prevFields || day !== prevDay + 1) {
      lines.push(JSON.stringify([calendar, offset, d, key, day]));
    }
    prevFields = key;
    prevDay = day;
  }
}
for (const calendar of calendars) {
  record(calendar, 0);
  if (late.includes(calendar)) record(calendar, 864e5 - 1);
}
lines.push(JSON.stringify(["", 0, last + 1, "", 0]));
lines.unshift(`# Node ${process.version}, ICU ${process.versions.icu}`);

const out = path.join(__dirname, "calendar_days_node.txt.gz");
fs.writeFileSync(out, zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
console.log(`${lines.length} runs, ${process.versions.icu} ICU, Node ${process.version}`);
