// Writes testdata/localeinfo_node.txt.gz: what Node's Intl.Locale says of
// every locale Intl.Locale knows, its language alone, and a few with
// keywords that change the answers: maximize, minimize, getCalendars,
// getCollations, getHourCycles, getNumberingSystems, getTimeZones,
// getTextInfo and getWeekInfo.
//
//	node testdata/localeinfo_node.js
//
// Each line is [tag, {maximize, minimize, calendars, collations,
// hourCycles, numberingSystems, timeZones, textInfo, weekInfo}], a value
// being null where Node throws or answers undefined.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const tags = new Set();
const all = fs.readdirSync(path.join(__dirname, "..", "data", "dates")).map(f => f.replace(/\.bin$/, ""));
for (const t of Intl.DateTimeFormat.supportedLocalesOf(all)) {
  tags.add(t);
  tags.add(t.split("-")[0]);
}
for (const t of [
  "und", "und-Arab", "und-Hant", "und-TW", "und-150", "zh-TW", "zh-Hant", "sr-ME", "uz-AF", "pa-PK",
  "en-u-hc-h23", "en-u-ca-buddhist", "en-u-co-emoji", "en-u-nu-thai", "en-u-fw-mon", "en-u-rg-gbzzzz",
  "en-US-u-rg-dezzzz", "de-u-co-phonebk", "ja-JP-u-ca-japanese", "ar-u-nu-latn", "fa-IR", "th-TH",
  "he", "ku", "ks", "ug", "yi", "dv", "syr", "ckb", "ps", "ur", "sd", "mzn", "lrc",
  "en-Arab", "ar-Latn", "zh-Latn", "sr-Latn", "und-Latn-RU", "und-Cyrl", "und-419", "und-001",
]) tags.add(t);

function safe(f) {
  try { const v = f(); return v === undefined ? null : v; } catch (e) { return null; }
}
const lines = [`# Node ${process.version}, ICU ${process.versions.icu}`];
for (const tag of [...tags].sort()) {
  let l;
  try { l = new Intl.Locale(tag); } catch (e) { continue; }
  lines.push(JSON.stringify([tag, {
    maximize: safe(() => l.maximize().toString()),
    minimize: safe(() => l.minimize().toString()),
    calendars: safe(() => l.getCalendars()),
    collations: safe(() => l.getCollations()),
    hourCycles: safe(() => l.getHourCycles()),
    numberingSystems: safe(() => l.getNumberingSystems()),
    timeZones: safe(() => l.getTimeZones()),
    textInfo: safe(() => l.getTextInfo()),
    weekInfo: safe(() => l.getWeekInfo()),
  }]));
}
const out = path.join(__dirname, "localeinfo_node.txt.gz");
fs.writeFileSync(out, zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
console.log(`${lines.length - 1} locales`);
