// Writes testdata/datetime_calendars_node.txt.gz: what Node's DateTimeFormat
// writes in every calendar it supports, in forty locales chosen for their
// scripts and calendars, under styles and field sets, for dates from 1900 to
// 2077 -- leap days, year ends and the turn of eras among them.
//
//	node testdata/datetime_calendars_node.js
//
// The lines have the shape of datetime_node.js's: [locale, options, epoch
// milliseconds, output, resolved hour cycle].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const locales = ["en", "fa", "ar", "he", "th", "ja", "zh", "zh-Hant", "ko", "hi", "am", "fr", "de",
  "ru", "es", "pt", "tr", "id", "ms", "ur", "ps", "bn", "ta", "te", "mr", "my", "km", "lo", "vi",
  "uk", "pl", "sv", "sw", "ha", "yo", "el", "hy", "ka", "si", "ne"];
const calendars = Intl.supportedValuesOf("calendar").filter(c => c !== "gregory");
const configs = [
  { dateStyle: "full" }, { dateStyle: "long" }, { dateStyle: "medium" }, { dateStyle: "short" },
  { year: "numeric", month: "numeric", day: "numeric" }, { year: "numeric", month: "long" },
  { month: "long", day: "numeric" }, { era: "long", year: "numeric" },
  { weekday: "long", day: "numeric" },
];
const instants = [
  Date.UTC(1900, 2, 1), Date.UTC(1970, 0, 1), Date.UTC(2000, 1, 29), Date.UTC(2024, 0, 5, 15, 4),
  Date.UTC(2024, 2, 20), Date.UTC(2024, 8, 15), Date.UTC(2033, 5, 1), Date.UTC(2077, 10, 11),
];

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
const out = path.join(__dirname, "datetime_calendars_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
