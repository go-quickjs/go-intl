// Writes testdata/available_node.txt.gz: which locales each of Node's Intl
// services is available in, among every name ICU has a bundle for, those
// names without their scripts and cut short, and every locale go-intl
// carries dates for.
//
//	node testdata/available_node.js <icu4c-78.3-data.zip>
//
// A tag is available to a service when resolving it with the "lookup"
// matcher answers with the tag itself: ECMA-402's BestAvailableLocale finds
// it in the list without cutting it short. Each line is [service, tag,
// available].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");
const { execFileSync } = require("child_process");

const zip = process.argv[2];
if (!zip) {
  console.error("usage: node testdata/available_node.js <icu4c-78.3-data.zip>");
  process.exit(2);
}
const listing = execFileSync("unzip", ["-l", zip], { maxBuffer: 1 << 26 }).toString();
const names = new Set();
for (const m of listing.matchAll(/data\/(?:locales|coll|lang|region|unit|curr|zone)\/([A-Za-z0-9_]+)\.txt/g)) {
  names.add(m[1]);
}
const plurals = execFileSync("unzip", ["-p", zip, "data/misc/plurals.txt"], { maxBuffer: 1 << 26 }).toString();
for (const m of plurals.matchAll(/^        ([a-z]{2,3}(?:_[A-Za-z0-9]+)*)\{"set/gm)) names.add(m[1]);
for (const f of fs.readdirSync(path.join(__dirname, "..", "data", "dates"))) names.add(f.replace(/\.bin$/, ""));

const candidates = new Set();
for (const name of names) {
  const parts = name.split(/[_-]/).filter(p => p !== "");
  for (let n = parts.length; n >= 1; n--) candidates.add(parts.slice(0, n).join("-"));
  if (parts.length > 1 && parts[1].length === 4) {
    candidates.add([parts[0], ...parts.slice(2)].join("-"));
  }
}

const services = {
  NumberFormat: t => new Intl.NumberFormat(t, { localeMatcher: "lookup" }),
  DurationFormat: t => new Intl.DurationFormat(t, { localeMatcher: "lookup" }),
  DateTimeFormat: t => new Intl.DateTimeFormat(t, { localeMatcher: "lookup" }),
  RelativeTimeFormat: t => new Intl.RelativeTimeFormat(t, { localeMatcher: "lookup" }),
  DisplayNames: t => new Intl.DisplayNames(t, { localeMatcher: "lookup", type: "region" }),
  ListFormat: t => new Intl.ListFormat(t, { localeMatcher: "lookup" }),
  Collator: t => new Intl.Collator(t, { localeMatcher: "lookup" }),
  PluralRules: t => new Intl.PluralRules(t, { localeMatcher: "lookup" }),
  Segmenter: t => new Intl.Segmenter(t, { localeMatcher: "lookup" }),
};

const lines = [`# Node ${process.version}, ICU ${process.versions.icu}`];
for (const raw of [...candidates].sort()) {
  let tag;
  try { tag = Intl.getCanonicalLocales(raw)[0]; } catch (e) { continue; }
  if (tag !== raw) continue;
  for (const [service, make] of Object.entries(services)) {
    const resolved = make(tag).resolvedOptions().locale;
    lines.push(JSON.stringify([service, tag, resolved === tag]));
  }
}
const out = path.join(__dirname, "available_node.txt.gz");
fs.writeFileSync(out, zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
console.log(`${lines.length - 1} lines`);
