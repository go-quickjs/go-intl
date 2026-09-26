// Writes testdata/datetime_resolved_node.txt.gz: what Node's
// Intl.DateTimeFormat resolvedOptions reports, in every locale go-intl has
// date data for, under a spread of styles, field sets, clocks and calendars.
//
//	node testdata/datetime_resolved_node.js
//
// Each line is a JSON array: [locale, options, resolved options], the
// resolved options without the time zone, which is always UTC here. Locales
// Node does not support for DateTimeFormat are left out.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;
const locales = fs.readdirSync(path.join(here, "..", "data", "dates"))
  .map(f => f.replace(/\.bin$/, "")).sort();

const configs = [
  {}, { dateStyle: "full" }, { timeStyle: "short" }, { dateStyle: "medium", timeStyle: "long" },
  { weekday: "long" }, { weekday: "short", day: "numeric" }, { weekday: "narrow" },
  { era: "short" }, { era: "long", year: "numeric" }, { era: "narrow", year: "2-digit", month: "short" },
  { year: "numeric" }, { year: "2-digit", month: "2-digit" },
  { month: "long" }, { month: "short" }, { month: "narrow" }, { month: "numeric", day: "numeric" },
  { year: "numeric", month: "long" }, { year: "numeric", month: "short", day: "numeric" },
  { year: "numeric", month: "long", day: "numeric", weekday: "long" },
  { day: "2-digit" }, { month: "long", day: "numeric" },
  { hour: "numeric" }, { hour: "2-digit", minute: "2-digit" }, { minute: "numeric" }, { second: "numeric" },
  { hour: "numeric", minute: "numeric", second: "numeric" },
  { hour: "numeric", hour12: true }, { hour: "numeric", hour12: false },
  { hour: "numeric", hourCycle: "h11" }, { hour: "numeric", hourCycle: "h24" },
  { minute: "numeric", hourCycle: "h23" },
  { dayPeriod: "long" }, { dayPeriod: "narrow", hour: "numeric" }, { dayPeriod: "short", hour: "numeric", minute: "numeric" },
  { second: "numeric", fractionalSecondDigits: 2 }, { fractionalSecondDigits: 3 },
  { hour: "numeric", timeZoneName: "short" }, { timeZoneName: "long" },
  { hour: "numeric", minute: "numeric", timeZoneName: "shortOffset" }, { timeZoneName: "longGeneric" },
  { month: "long", day: "numeric", hour: "numeric", minute: "numeric" },
  { calendar: "japanese", era: "long", year: "numeric" }, { calendar: "buddhist", year: "numeric", month: "long" },
  { calendar: "chinese", year: "numeric", month: "long" }, { calendar: "hebrew", month: "long", day: "numeric" },
  { calendar: "islamic-civil", dateStyle: "long" },
];

// Tags whose Unicode extension the options may override, and calendars
// ECMA-402 deprecates, each under the options that bear on them.
const tagged = ["en-u-hc-h23", "en-u-hc-h11", "ja-u-hc-h11", "de-u-hc-h12", "en-u-hc-h25",
  "en-u-ca-islamic", "ar-SA-u-ca-islamic-rgsa", "en-u-ca-japanese-hc-h24"];
const taggedConfigs = [
  {}, { hour: "numeric" }, { hour12: false }, { hour12: true }, { hour: "numeric", hour12: false },
  { hour: "numeric", hour12: true }, { hourCycle: "h23" }, { hour: "numeric", hourCycle: "h23" },
  { hour: "numeric", hourCycle: "h11" }, { timeStyle: "short", hour12: false },
  { dateStyle: "short", hourCycle: "h23" }, { calendar: "islamic" }, { calendar: "islamic-rgsa", year: "numeric" },
];

const lines = [];
for (const loc of tagged) {
  for (const c of taggedConfigs) {
    const opts = { ...c, timeZone: "UTC" };
    const r = new Intl.DateTimeFormat(loc, opts).resolvedOptions();
    delete r.timeZone;
    lines.push(JSON.stringify([loc, opts, r]));
  }
}
for (const loc of locales) {
  for (const c of configs) {
    const opts = { ...c, timeZone: "UTC" };
    let f;
    try { f = new Intl.DateTimeFormat(loc, opts); } catch (e) { continue; }
    const r = f.resolvedOptions();
    // A locale Node does not support falls back to Node's default locale,
    // which is negotiation, not resolution; such locales are left out.
    if (r.locale.toLowerCase() !== loc.toLowerCase() &&
        !r.locale.toLowerCase().startsWith(loc.toLowerCase() + "-u-")) continue;
    delete r.timeZone;
    lines.push(JSON.stringify([loc, opts, r]));
  }
}
const out = path.join(here, "datetime_resolved_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases, ${locales.length} locales -> ${out}`);
