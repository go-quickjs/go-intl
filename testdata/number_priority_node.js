// Writes testdata/number_priority_node.json: what Node writes when both
// significant and fraction digits are given with a rounding priority,
// morePrecision or lessPrecision, over combinations of the four counts and
// a spread of values.
//
//	node testdata/number_priority_node.js
//
// Each case is [options, value, output]; the options leave out what was not
// given.
"use strict";
const fs = require("fs");
const path = require("path");

const counts = [undefined, 1, 2, 3, 5];
const values = [0, 1, 1.5, 1.625, 1.75, 1.875, 2, 0.001234, 12.5, 123.456, 98765.4321, -3.14159, 0.5, 1e-10];

const cases = [];
for (const roundingPriority of ["morePrecision", "lessPrecision"]) {
  for (const minimumSignificantDigits of counts) {
    for (const maximumSignificantDigits of counts) {
      for (const minimumFractionDigits of counts) {
        for (const maximumFractionDigits of counts) {
          const opts = { roundingPriority, useGrouping: false };
          const put = (k, v) => { if (v !== undefined) opts[k] = v; };
          put("minimumSignificantDigits", minimumSignificantDigits);
          put("maximumSignificantDigits", maximumSignificantDigits);
          put("minimumFractionDigits", minimumFractionDigits);
          put("maximumFractionDigits", maximumFractionDigits);
          let f;
          try {
            f = new Intl.NumberFormat("en-US", opts);
          } catch (e) {
            continue;
          }
          for (const x of values) cases.push([opts, x, f.format(x)]);
        }
      }
    }
  }
}
const out = path.join(__dirname, "number_priority_node.json");
fs.writeFileSync(out, JSON.stringify({ node: process.version, icu: process.versions.icu, cases }) + "\n");
console.log(`${cases.length} cases -> ${out}`);
