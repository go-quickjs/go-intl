// The locales a data directory has data for, as internal/datawrite writes
// one: <tag>.bin for each locale whose data is its own, and same.bin, a
// table of each other locale and the one whose file it shares. A recording
// that lists a directory's files alone misses the locales in same.bin --
// en-AG is en-001's -- and same.bin itself is no locale.
//
//	const { dataLocales } = require("./datalocales.js");
//	const locales = dataLocales(path.join(__dirname, "..", "data", "dates"));
"use strict";
const fs = require("fs");
const path = require("path");

// The version same.bin begins with (internal/layout.Pairs), and a data
// locale's bytes: the language, script and region, zero-padded.
const pairsVersion = 1;
const fields = [8, 4, 3];
const localeSize = fields.reduce((a, b) => a + b);

function tagOf(b) {
  const parts = [];
  let at = 0;
  for (const n of fields) {
    const s = b.subarray(at, at + n).toString("latin1").replace(/\0+$/, "");
    if (s) parts.push(s);
    at += n;
  }
  if (parts.length === 0 || fields[0] && b[0] === 0) parts.unshift("und");
  return parts.join("-");
}

// dataLocales is every locale dir has data for, its own or another's,
// sorted as the file names sort.
function dataLocales(dir) {
  const tags = new Set();
  for (const f of fs.readdirSync(dir)) {
    if (f.endsWith(".bin") && f !== "same.bin") tags.add(f.slice(0, -".bin".length));
  }
  const same = path.join(dir, "same.bin");
  if (fs.existsSync(same)) {
    const b = fs.readFileSync(same);
    if (b.length === 0 || b[0] !== pairsVersion || (b.length - 1) % (2 * localeSize) !== 0) {
      throw new Error(`${same} is not a table of locale pairs of version ${pairsVersion}`);
    }
    for (let at = 1; at < b.length; at += 2 * localeSize) tags.add(tagOf(b.subarray(at, at + localeSize)));
  }
  return [...tags].sort();
}

module.exports = { dataLocales };
