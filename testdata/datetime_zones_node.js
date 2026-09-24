// Writes testdata/datetime_zones_node.txt.gz: what Node calls every time zone
// it knows, in each of the six timeZoneName styles, in winter and summer, in a
// handful of locales chosen for their differences -- names of their own, a
// "UTC" rather than a "GMT", other scripts, a locale ICU keeps no zone bundle
// for.
//
//	node testdata/datetime_zones_node.js
//
// The lines have the shape of datetime_node.js's: [locale, options, epoch
// milliseconds, output, resolved hour cycle].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const locales = ["en", "en-GB", "de", "fr", "ja", "ar", "zh-Hant", "sr-Cyrl-ME", "es-419"];
const styles = ["short", "long", "shortGeneric", "longGeneric", "shortOffset", "longOffset"];
const instants = [1704467045000, 1721467800000];

const lines = [];
for (const loc of locales) {
  for (const style of styles) {
    // Every zone Node knows, and offsets from UTC in each of the forms
    // ECMA-402 accepts.
    const offsets = ["+05:30", "-08", "+0530", "-00:00", "+00", "+14:00", "-23:59"];
    for (const zone of [...Intl.supportedValuesOf("timeZone"), ...offsets]) {
      const opts = { calendar: "gregory", timeZone: zone, hour: "numeric", timeZoneName: style };
      const f = new Intl.DateTimeFormat(loc, opts);
      for (const t of instants) {
        lines.push(JSON.stringify([loc, opts, t, f.format(t), f.resolvedOptions().hourCycle || ""]));
      }
    }
  }
}
const out = path.join(__dirname, "datetime_zones_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
