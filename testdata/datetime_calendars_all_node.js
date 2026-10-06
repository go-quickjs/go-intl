// Writes testdata/datetime_calendars_all_node.txt.gz: what Node's
// DateTimeFormat writes in every calendar other than the Gregorian, in every
// locale go-intl has date data for, more thinly than
// datetime_calendars_node.js does its forty: two styles and two field sets,
// on an ordinary day and on the first of a Japanese era.
//
//	node testdata/datetime_calendars_all_node.js
//
// The lines have the shape of datetime_node.js's: [locale, options, epoch
// milliseconds, output, resolved hour cycle]. Locales Node does not support
// for DateTimeFormat are left out.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;
const locales = fs.readdirSync(path.join(here, "..", "data", "dates"))
  .filter(f => f.endsWith(".bin") && f !== "same.bin")
  .map(f => f.replace(/\.bin$/, "")).sort();
const calendars = Intl.supportedValuesOf("calendar").filter(c => c !== "gregory");
const configs = [
  { dateStyle: "full" }, { dateStyle: "short" },
  { era: "long", year: "numeric", month: "long", day: "numeric" },
  { year: "numeric", month: "numeric" },
];
const instants = [Date.UTC(2024, 2, 20), Date.UTC(2019, 4, 1)];

const lines = [];
for (const loc of locales) {
  for (const calendar of calendars) {
    for (const c of configs) {
      const opts = { calendar, timeZone: "UTC", ...c };
      let f;
      try { f = new Intl.DateTimeFormat(loc, opts); } catch (e) { continue; }
      if (f.resolvedOptions().locale.toLowerCase() !== loc.toLowerCase()) continue;
      for (const t of instants) {
        lines.push(JSON.stringify([loc, opts, t, f.format(t), f.resolvedOptions().hourCycle || ""]));
      }
    }
  }
}
const out = path.join(here, "datetime_calendars_all_node.txt.gz");
fs.writeFileSync(out, zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
