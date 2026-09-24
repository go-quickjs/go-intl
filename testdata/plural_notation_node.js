// Writes testdata/plural_notation_node.txt.gz: what Node's PluralRules.select
// answers in every locale it supports, in each notation -- where compact and
// scientific numbers carry a power of ten the rules can count -- and under
// directional rounding of negative numbers.
//
//	node testdata/plural_notation_node.js
//
// Each line is a JSON array: [locale, options, number, category].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;
const locales = fs.readdirSync(path.join(here, "..", "data", "dates"))
  .map(f => f.replace(/\.bin$/, "")).sort();
const configs = [
  { notation: "compact" }, { notation: "compact", compactDisplay: "long" },
  { notation: "scientific" }, { notation: "engineering" },
  { notation: "compact", maximumFractionDigits: 2 }, { type: "ordinal", notation: "compact" },
  { maximumFractionDigits: 0, roundingMode: "floor" }, { maximumFractionDigits: 0, roundingMode: "ceil" },
];
const numbers = [0, 1, 1.5, 2, 1000, 1500, 12345, 1e6, 1.5e6, 2e6, 3e6, 1e7, 1e9, 0.001, 1e-7, -1.5, -1.5e6, -0.5];

const lines = [];
for (const loc of locales) {
  for (const opts of configs) {
    let p;
    try { p = new Intl.PluralRules(loc, opts); } catch (e) { continue; }
    if (p.resolvedOptions().locale.toLowerCase() !== loc.toLowerCase()) continue;
    for (const n of numbers) lines.push(JSON.stringify([loc, opts, n, p.select(n)]));
  }
}
const out = path.join(here, "plural_notation_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
