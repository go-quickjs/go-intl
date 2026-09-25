// Writes testdata/number_range_node.txt.gz: what Node's NumberFormat
// formatRange and formatRangeToParts write, under a spread of styles,
// notations and signs, for pairs of numbers: equal, equal once rounded,
// apart, of opposite signs, and the wrong way round.
//
//	node testdata/number_range_node.js
//
// Each line is a JSON array: [locale, options, start, end, formatRange's
// output, formatRangeToParts' output as [type, value, source] triples].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const configs = [
  {}, { maximumFractionDigits: 0 }, { style: "percent" },
  { style: "currency", currency: "USD" }, { style: "currency", currency: "EUR" },
  { style: "currency", currency: "USD", currencyDisplay: "code" },
  { style: "currency", currency: "USD", currencyDisplay: "name" },
  { style: "currency", currency: "USD", currencySign: "accounting" },
  { style: "unit", unit: "kilometer" }, { style: "unit", unit: "kilometer", unitDisplay: "long" },
  { style: "unit", unit: "liter", unitDisplay: "narrow" }, { notation: "compact" },
  { notation: "compact", compactDisplay: "long" }, { notation: "scientific" },
  { notation: "engineering" }, { signDisplay: "always" }, { signDisplay: "exceptZero" },
  { style: "unit", unit: "kilometer-per-hour", unitDisplay: "long" },
];
const pairs = [
  [5, 10], [5, 5], [5, 5.0001], [1, 2], [0, 1], [-5, 5], [-10, -5], [10, 5], [1000, 5000],
  [1500, 1500.4], [0.05, 0.1], [2.5, 3.5], [-1, 1], [1e6, 2e6], [123.456, 789.012],
];
const locales = ["en", "de", "fr", "ar", "hi", "ja", "ru", "es", "zh", "he", "pl", "sv", "pt", "bn", "fa", "sw"];

const lines = [];
for (const loc of locales) {
  for (const opts of configs) {
    const f = new Intl.NumberFormat(loc, opts);
    for (const [a, b] of pairs) {
      const parts = f.formatRangeToParts(a, b).map(p => [p.type, p.value, p.source]);
      lines.push(JSON.stringify([loc, opts, a, b, f.formatRange(a, b), parts]));
    }
  }
}
const out = path.join(__dirname, "number_range_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
