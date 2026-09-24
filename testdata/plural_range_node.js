// Writes testdata/plural_range_node.txt.gz: what Node's
// PluralRules.selectRange answers, in every locale Node supports, for
// cardinal and ordinal rules, with and without fraction digits, over pairs of
// small numbers that reach each category at each end.
//
//	node testdata/plural_range_node.js
//
// Each line is a JSON array: [locale, options, start, end, category].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;
const locales = fs.readdirSync(path.join(here, "..", "data", "dates"))
  .map(f => f.replace(/\.bin$/, "")).sort();
const configs = [{}, { type: "ordinal" }, { minimumFractionDigits: 1 }, { maximumFractionDigits: 0 }];
const numbers = [0, 1, 2, 3, 5, 6, 11, 21, 100, 1.5, 0.5, -1, 1e6];

const lines = [];
for (const loc of locales) {
  for (const opts of configs) {
    let p;
    try { p = new Intl.PluralRules(loc, opts); } catch (e) { continue; }
    if (p.resolvedOptions().locale.toLowerCase() !== loc.toLowerCase()) continue;
    for (const a of numbers) for (const b of numbers) {
      lines.push(JSON.stringify([loc, opts, a, b, p.selectRange(a, b)]));
    }
  }
}
const out = path.join(here, "plural_range_node.txt.gz");
const text = `# node ${process.version} ICU ${process.versions.icu}\n` + lines.join("\n") + "\n";
fs.writeFileSync(out, zlib.gzipSync(text, { level: 9 }));
console.log(`${lines.length} cases -> ${out}`);
