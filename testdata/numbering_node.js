// Writes testdata/numbering_node.txt.gz: what Node writes in every numeric
// numbering system, asked for by keyword and by option, across locales whose
// own data covers the system and locales whose data does not.
//
//	node testdata/numbering_node.js
//
// Each line is a JSON array:
//
//	[service, locale, options, value, output, resolved locale, resolved system]
//
// The value is a number for NumberFormat and RelativeTimeFormat (in days) and
// an epoch millisecond for DateTimeFormat.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const systems = Intl.supportedValuesOf("numberingSystem");
// Locales with a native system of their own, locales without, and ones whose
// default is not Latin.
const locales = ["en", "de", "fr", "hi", "ar", "ar-EG", "fa", "bn", "th", "zh", "ja", "my", "ps", "ur"];
const numberOptions = [
  {},
  { style: "percent" },
  { style: "currency", currency: "EUR" },
  { notation: "scientific" },
  { notation: "compact" },
];
const values = [-1234567.891, 0.25, 42];

const lines = [];
const add = (service, tag, opts, value, f, output) => {
  const r = f.resolvedOptions();
  lines.push(JSON.stringify([service, tag, opts, value, output, r.locale, r.numberingSystem]));
};

for (const loc of locales) {
  const asks = [[loc, {}]];
  for (const nu of systems) {
    asks.push([loc + "-u-nu-" + nu, {}]);
    asks.push([loc, { numberingSystem: nu }]);
  }
  // A keyword the option overrules, one the option agrees with, and a
  // keyword that is not a numeric system.
  asks.push([loc + "-u-nu-arab", { numberingSystem: "deva" }]);
  asks.push([loc + "-u-nu-arab", { numberingSystem: "arab" }]);
  asks.push([loc + "-u-nu-roman", {}]);
  asks.push([loc + "-u-ca-buddhist-nu-thai", {}]);
  for (const [tag, base] of asks) {
    for (const o of numberOptions) {
      const opts = { ...base, ...o };
      const f = new Intl.NumberFormat(tag, opts);
      for (const v of values) add("NumberFormat", tag, opts, v, f, f.format(v));
    }
    const d = new Intl.DateTimeFormat(tag, { ...base, timeZone: "UTC", dateStyle: "short", timeStyle: "medium" });
    add("DateTimeFormat", tag, { ...base, timeZone: "UTC", dateStyle: "short", timeStyle: "medium" },
      1704467045000, d, d.format(1704467045000));
    const r = new Intl.RelativeTimeFormat(tag, base);
    add("RelativeTimeFormat", tag, base, -1234, r, r.format(-1234, "day"));
  }
}

const out = path.join(__dirname, "numbering_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
