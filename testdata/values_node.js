// Writes testdata/values_node.json: Node's Intl.supportedValuesOf for each
// key.
//
//	node testdata/values_node.js
"use strict";
const fs = require("fs");
const path = require("path");

const out = { icu: process.versions.icu, node: process.version };
for (const key of ["calendar", "collation", "currency", "numberingSystem", "timeZone", "unit"]) {
  out[key] = Intl.supportedValuesOf(key);
}
fs.writeFileSync(path.join(__dirname, "values_node.json"), JSON.stringify(out, null, 1) + "\n");
