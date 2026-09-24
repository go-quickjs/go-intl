// Writes testdata/datetime_range_node.txt.gz: what Node's formatRange and
// formatRangeToParts write, in every locale Node supports, under a spread of
// styles and field sets, for pairs of moments that differ in each field in
// turn: not at all, in milliseconds, minutes, hours, the day period, days,
// months and years.
//
//	node testdata/datetime_range_node.js
//
// Each line is a JSON array: [locale, options, start, end, formatRange's
// output, formatRangeToParts' output as [type, value, source] triples].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;
const locales = fs.readdirSync(path.join(here, "..", "data", "dates"))
  .map(f => f.replace(/\.bin$/, "")).sort();

const configs = [
  {}, { dateStyle: "full" }, { dateStyle: "long" }, { dateStyle: "medium" }, { dateStyle: "short" },
  { timeStyle: "short" }, { timeStyle: "medium" }, { dateStyle: "medium", timeStyle: "short" },
  { year: "numeric", month: "short", day: "numeric" }, { month: "long", day: "numeric" },
  { year: "numeric", month: "long" }, { hour: "numeric", minute: "2-digit" },
  { hour: "numeric", minute: "2-digit", hour12: false },
  { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit" },
  { hour: "numeric", minute: "numeric", second: "numeric" }, { era: "short", year: "numeric" },
  { hour: "numeric", dayPeriod: "short" },
  { timeZone: "America/New_York", hour: "numeric", minute: "numeric", timeZoneName: "short" },
];
const base = Date.UTC(2024, 0, 5, 15, 4, 5);
const pairs = [
  [base, base], [base, base + 500], [base, base + 30 * 60e3], [base, base + 3 * 3600e3],
  [base - 6 * 3600e3, base], [base, base + 2 * 86400e3], [base, Date.UTC(2024, 1, 7, 9, 0)],
  [base, Date.UTC(2025, 2, 9, 15, 4, 5)],
];

const lines = [];
for (const loc of locales) {
  for (const c of configs) {
    const opts = { calendar: "gregory", timeZone: "UTC", ...c };
    let f;
    try { f = new Intl.DateTimeFormat(loc, opts); } catch (e) { continue; }
    if (f.resolvedOptions().locale.toLowerCase() !== loc.toLowerCase()) continue;
    for (const [a, b] of pairs) {
      const parts = f.formatRangeToParts(a, b).map(p => [p.type, p.value, p.source]);
      lines.push(JSON.stringify([loc, opts, a, b, f.formatRange(a, b), parts]));
    }
  }
}
const out = path.join(here, "datetime_range_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
