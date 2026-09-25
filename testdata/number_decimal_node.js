// Writes testdata/number_decimal_node.txt.gz: what Node's NumberFormat writes
// for numbers given exactly -- as strings, which ECMA-402 reads without
// rounding them to a float -- under a spread of options, in a handful of
// locales.
//
//	node testdata/number_decimal_node.js
//
// Each line is a JSON array: [locale, options, input string, output,
// formatToParts' output as [type, value] pairs].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const tab = String.fromCharCode(9);
const inputs = [
  "123456789012345678901234567890.123456789", "0.1", "1e21", "-0", "0", "1e-7",
  "12345678901234567890e-10", "0x1F", "0o17", "0b101", " 42 ", tab + "7" + tab, "", "abc",
  "Infinity", "-Infinity", "+Infinity", "1.005", "2.675", "9.999999999999999999",
  "0.000000000000000000001", "1e400", "-1e-400", "5e-1", "1.5", "2.5", "-2.5",
  "99999999999999999999", "+1", ".5", "5.", "1_000", "1e", "--1", "-0x10", "1.2.3",
  "999999.5", "0.00045", "1E+3", "123.456e2", "-987654321987654321.5",
];
const configs = [
  {}, { maximumFractionDigits: 20 }, { minimumFractionDigits: 5 },
  { maximumSignificantDigits: 21 }, { minimumSignificantDigits: 3, maximumSignificantDigits: 5 },
  { notation: "compact" }, { notation: "compact", compactDisplay: "long" },
  { notation: "scientific" }, { notation: "engineering", maximumFractionDigits: 5 },
  { style: "percent", maximumFractionDigits: 20 }, { style: "currency", currency: "USD" },
  { roundingMode: "halfEven", maximumFractionDigits: 0 },
  { roundingIncrement: 5, maximumFractionDigits: 2, minimumFractionDigits: 2 },
  { signDisplay: "exceptZero" }, { style: "unit", unit: "kilometer", unitDisplay: "long" },
  { useGrouping: false }, { roundingPriority: "lessPrecision", maximumFractionDigits: 3, maximumSignificantDigits: 3 },
  // The unit percent is written with the percent pattern, unscaled, except
  // spelled out or compact.
  { style: "unit", unit: "percent" }, { style: "unit", unit: "percent", unitDisplay: "narrow" },
  { style: "unit", unit: "percent", unitDisplay: "long" }, { style: "unit", unit: "percent", notation: "compact" },
  { useGrouping: "min2" }, { useGrouping: "always", notation: "compact" }, { useGrouping: true },
];
const locales = ["en", "de", "fr", "ar", "hi", "ja", "ru", "es", "bn", "fa", "tr"];

const lines = [];
for (const loc of locales) {
  for (const opts of configs) {
    const f = new Intl.NumberFormat(loc, opts);
    for (const s of inputs) {
      const parts = f.formatToParts(s).map(p => [p.type, p.value]);
      lines.push(JSON.stringify([loc, opts, s, f.format(s), parts]));
    }
  }
}
const out = path.join(__dirname, "number_decimal_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
