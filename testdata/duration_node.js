// Writes testdata/duration_node.txt.gz: what Node's Intl.DurationFormat
// writes, as parts, for durations under a set of option bags, in forty
// locales chosen for their scripts, and for a few durations in every locale
// go-intl carries data for; with each formatter's resolved options.
//
//	node testdata/duration_node.js
//
// Each line is [locale, options, duration, parts or null, resolved options],
// parts being [type, value, unit] and null meaning Node threw.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const some = ["en", "fr", "de", "da", "fi", "ja", "zh", "zh-Hant", "ko", "ar", "fa", "he", "hi", "bn",
  "mr", "ne", "my", "th", "ru", "uk", "pl", "cs", "es", "pt", "it", "sv", "nb", "tr", "id", "vi",
  "ur", "ps", "sw", "am", "el", "ka", "hy", "ta", "te", "km"];
const configs = [
  {}, { style: "long" }, { style: "narrow" }, { style: "digital" },
  { style: "digital", fractionalDigits: 2 }, { style: "digital", fractionalDigits: 0 },
  { style: "long", hours: "numeric" }, { minutes: "numeric" }, { seconds: "numeric" },
  { milliseconds: "numeric" }, { style: "narrow", daysDisplay: "always", hoursDisplay: "always" },
  { hours: "2-digit" }, { style: "digital", hours: "long" },
  { microseconds: "numeric", fractionalDigits: 5 },
  { style: "long", yearsDisplay: "always", secondsDisplay: "always" },
  { style: "digital", numberingSystem: "arab" }, { style: "short", numberingSystem: "deva" },
];
const durations = [
  {}, { seconds: 0 }, { hours: 1, minutes: 46, seconds: 40 },
  { years: 1, months: 2, weeks: 3, days: 4, hours: 5, minutes: 6, seconds: 7, milliseconds: 8, microseconds: 9, nanoseconds: 10 },
  { hours: -1, minutes: -2 }, { hours: 0, seconds: -5 }, { milliseconds: -500 }, { days: -3, hours: 0 },
  { seconds: 1, milliseconds: 500 }, { milliseconds: 1234567 }, { nanoseconds: 1 },
  { seconds: 59, nanoseconds: 999999999 }, { hours: 1234567 }, { years: 4294967295 },
  { nanoseconds: 1e20 }, { minutes: 5 }, { hours: 2 }, { hours: 1, seconds: 5 },
  { days: 1 }, { days: 2 }, { days: 5 }, { days: 21 }, { weeks: 11, minutes: 1 },
  { seconds: -1, microseconds: -1 },
];
const everywhere = [{}, { style: "digital" }, { style: "long" }];
const everyDuration = [{ hours: 1, minutes: 46, seconds: 40, milliseconds: 5 }, { years: 2, days: 1 }, { weeks: -1, seconds: -30 }];

const lines = [];
function record(locale, options, list) {
  let f, resolved = null;
  try {
    f = new Intl.DurationFormat(locale, options);
    resolved = f.resolvedOptions();
  } catch (e) {
    lines.push(JSON.stringify([locale, options, null, null, null]));
    return;
  }
  for (const d of list) {
    let parts = null;
    try {
      parts = f.formatToParts(d).map(p => [p.type, p.value, p.unit || ""]);
    } catch (e) { /* recorded as null */ }
    lines.push(JSON.stringify([locale, options, d, parts, resolved]));
  }
}
for (const locale of some) {
  for (const c of configs) record(locale, c, durations);
}
const all = fs.readdirSync(path.join(__dirname, "..", "data", "dates")).map(f => f.replace(/\.bin$/, ""));
for (const locale of Intl.DurationFormat.supportedLocalesOf(all)) {
  if (some.includes(locale)) continue;
  for (const c of everywhere) record(locale, c, everyDuration);
}
lines.unshift(`# Node ${process.version}, ICU ${process.versions.icu}`);
const out = path.join(__dirname, "duration_node.txt.gz");
fs.writeFileSync(out, zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
console.log(`${lines.length - 1} cases`);
