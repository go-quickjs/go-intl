// Records every *_node.js script in one Chrome, one script after another,
// with record.js:
//
//	node testdata/chrome/record_all.js [--lang tag] <chrome.exe> <ICU version> <tz version> <out dir> <icu4c-78.3-data.zip> <test262 dir> [script.js ...]
//
// --lang runs each Chrome in that language (record.js).
//
// The data archive and test262 are the arguments available_node.js and
// canonical_node.js take; the named scripts alone are recorded when any
// are named. The scripts that format in every locale run in shards
// (record.js), which keeps a Chrome to a few gigabytes.
"use strict";
const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

const shards = { "datetime_features_node.js": 8, "datetime_range_node.js": 8 };

function main() {
  const all = process.argv.slice(2);
  let lang = null;
  if (all[0] === "--lang") {
    lang = all[1];
    all.splice(0, 2);
  }
  const [chrome, icu, tz, outDir, zip, test262, ...only] = all;
  if (!test262) {
    console.error("usage: node testdata/chrome/record_all.js <chrome.exe> <ICU version> <tz version> <out dir> <icu4c-78.3-data.zip> <test262 dir> [script.js ...]");
    process.exit(2);
  }
  const testdata = path.join(__dirname, "..");
  const args = { "available_node.js": [zip], "canonical_node.js": [zip, test262] };
  const scripts = fs.readdirSync(testdata).filter(f => f.endsWith("_node.js")).sort()
    .filter(f => only.length === 0 || only.includes(f));
  let failed = 0;
  for (const s of scripts) {
    const start = Date.now();
    const argv = [path.join(__dirname, "record.js")];
    if (shards[s]) argv.push("--shards", String(shards[s]));
    if (lang) argv.push("--lang", lang);
    argv.push(chrome, icu, tz, outDir, path.join(testdata, s), ...(args[s] || []));
    const r = spawnSync(process.execPath, argv, { stdio: ["ignore", "pipe", "pipe"], encoding: "utf8" });
    const took = ((Date.now() - start) / 1000).toFixed(0);
    if (r.status === 0) {
      console.log(`${s}: ok, ${took}s`);
    } else {
      failed++;
      console.log(`${s}: FAILED (${r.status}), ${took}s\n${r.stderr.trim().split("\n").slice(-3).join("\n")}`);
    }
  }
  if (failed > 0) process.exit(1);
}

main();
