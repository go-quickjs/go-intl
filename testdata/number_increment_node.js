// Writes testdata/number_increment_node.json: what Node writes for rounding
// to an increment, with every rounding mode, at none, one and two decimals,
// for numbers either side of each step and on the midpoints between them.
//
//	node testdata/number_increment_node.js
//
// Each case is [increment, decimals, roundingMode, value, output].
"use strict";
const fs = require("fs");
const path = require("path");

const increments = [2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000];
const modes = ["ceil", "floor", "expand", "trunc", "halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"];
const values = [0, 1.25, 1.35, 1.45, 0.125, -2.5, 1234.5678, 7.49, 0.0249, 3.75, -1.25, 99.99, 1e-7, 123456.789];

const cases = [];
for (const inc of increments) {
  for (const frac of [0, 1, 2]) {
    for (const mode of modes) {
      const f = new Intl.NumberFormat("en-US", {
        roundingIncrement: inc, minimumFractionDigits: frac, maximumFractionDigits: frac,
        roundingMode: mode, useGrouping: false,
      });
      for (const x of values) cases.push([inc, frac, mode, x, f.format(x)]);
    }
  }
}
const out = path.join(__dirname, "number_increment_node.json");
fs.writeFileSync(out, JSON.stringify({ node: process.version, icu: process.versions.icu, cases }) + "\n");
console.log(`${cases.length} cases -> ${out}`);
