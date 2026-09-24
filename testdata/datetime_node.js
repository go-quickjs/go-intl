// Writes testdata/datetime_node.txt.gz: what Node's Intl.DateTimeFormat
// writes, in every locale go-intl has date data for, under a spread of styles
// and field sets, in two zones and two seasons, in the Gregorian calendar.
//
//	node testdata/datetime_node.js
//
// Each line is a JSON array: [locale, options, epoch milliseconds, output,
// resolved hour cycle]. Locales Node does not support for DateTimeFormat are
// left out. The golden corpus covers a few locales in depth; this
// covers every locale more thinly, which is where the glue bugs were hiding.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;
const locales = fs.readdirSync(path.join(here, "..", "data", "dates"))
  .map(f => f.replace(/\.bin$/, "")).sort();

const g = { calendar: "gregory" };
const configs = [
  { dateStyle: "full" }, { dateStyle: "long" }, { dateStyle: "medium" }, { dateStyle: "short" },
  { timeStyle: "medium" }, { timeStyle: "short" },
  { dateStyle: "full", timeStyle: "short" }, { dateStyle: "long", timeStyle: "medium" },
  { dateStyle: "short", timeStyle: "short" },
  { year: "numeric", month: "long", day: "numeric" },
  { year: "numeric", month: "short", day: "numeric", weekday: "short" },
  { month: "numeric", day: "numeric" },
  { year: "numeric", month: "2-digit" },
  { month: "long" },
  { weekday: "long" },
  { era: "short", year: "numeric" },
  { hour: "numeric", minute: "2-digit" },
  { hour: "numeric", minute: "2-digit", second: "2-digit", hour12: false },
  { hour: "numeric", minute: "2-digit", hour12: true },
  { hour: "numeric", hourCycle: "h11" },
  { day: "numeric", hour: "numeric" },
  { year: "numeric", month: "numeric", day: "numeric", hour: "numeric", minute: "numeric", second: "numeric" },
  { month: "short", day: "numeric", hour: "numeric", minute: "numeric" },
];
const zones = [
  // New York with the zone written, to reach the zone names.
  { timeZone: "America/New_York", timeStyle: "long" },
  { timeZone: "America/New_York", hour: "numeric", timeZoneName: "short" },
  { timeZone: "America/New_York", hour: "numeric", timeZoneName: "long" },
];
const instants = [1704467045000, 1721467800000];

const lines = [];
for (const loc of locales) {
  for (const c of [...configs.map(c => ({ ...c, timeZone: "UTC" })), ...zones]) {
    const opts = { ...g, ...c };
    let f;
    try { f = new Intl.DateTimeFormat(loc, opts); } catch (e) { continue; }
    // A locale Node does not support falls back to Node's default locale,
    // which is negotiation, not formatting; such locales are left out.
    if (f.resolvedOptions().locale.toLowerCase() !== loc.toLowerCase()) continue;
    for (const t of instants) {
      lines.push(JSON.stringify([loc, opts, t, f.format(t), f.resolvedOptions().hourCycle || ""]));
    }
  }
}
const out = path.join(here, "datetime_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases, ${locales.length} locales -> ${out}`);
