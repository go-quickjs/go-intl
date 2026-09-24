// Writes testdata/datetime_features_node.txt.gz: what Node writes for the
// DateTimeFormat options beyond the golden corpus -- dayPeriod,
// fractionalSecondDigits and the six timeZoneName styles -- in every locale
// Node supports, across zones and seasons.
//
//	node testdata/datetime_features_node.js
//
// The lines have the shape of datetime_node.js's: [locale, options, epoch
// milliseconds, output, resolved hour cycle].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;
const locales = fs.readdirSync(path.join(here, "..", "data", "dates"))
  .map(f => f.replace(/\.bin$/, "")).sort();

// Midnight, early morning, noon, the afternoon and the evening, in winter and
// summer, and a moment with milliseconds for the fraction.
const instants = [1704412800000, 1704434400123, 1704456000000, 1721487545678, 1721505600000];
const periodConfigs = [
  { dayPeriod: "narrow" }, { dayPeriod: "short" }, { dayPeriod: "long" },
  { hour: "numeric", dayPeriod: "short" }, { hour: "numeric", dayPeriod: "long" },
  { hour: "numeric", minute: "2-digit", dayPeriod: "narrow", hourCycle: "h12" },
  { hour: "numeric", dayPeriod: "long", hourCycle: "h23" },
];
const fractionConfigs = [
  { second: "numeric", fractionalSecondDigits: 1 },
  { minute: "numeric", second: "numeric", fractionalSecondDigits: 2 },
  { hour: "numeric", minute: "numeric", second: "numeric", fractionalSecondDigits: 3 },
  { fractionalSecondDigits: 3 },
];
const zoneInstants = [1704467045000, 1721467800000];
const zones = ["America/New_York", "Europe/London", "Asia/Kolkata", "Australia/Sydney", "Asia/Kathmandu"];
const styles = ["short", "long", "shortOffset", "longOffset", "shortGeneric", "longGeneric"];

const lines = [];
const add = (loc, opts, times) => {
  let f;
  try { f = new Intl.DateTimeFormat(loc, opts); } catch (e) { return; }
  if (f.resolvedOptions().locale.toLowerCase() !== loc.toLowerCase()) return;
  for (const t of times) {
    lines.push(JSON.stringify([loc, opts, t, f.format(t), f.resolvedOptions().hourCycle || ""]));
  }
};
for (const loc of locales) {
  for (const c of periodConfigs) add(loc, { calendar: "gregory", timeZone: "UTC", ...c }, instants);
  for (const c of fractionConfigs) add(loc, { calendar: "gregory", timeZone: "UTC", ...c }, [instants[1], instants[3]]);
  for (const zone of zones) {
    for (const style of styles) {
      add(loc, { calendar: "gregory", timeZone: zone, hour: "numeric", timeZoneName: style }, zoneInstants);
    }
  }
}
const out = path.join(here, "datetime_features_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
