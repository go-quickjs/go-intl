// Writes testdata/canonical_node.txt.gz: what Node's Intl.getCanonicalLocales
// makes of every alias ICU knows, each in a few surroundings, of every
// Unicode extension type and its aliases, and of the tags test262 uses to
// test canonicalization.
//
//	node testdata/canonical_node.js <icu4c-78.3-data.zip> [<test262 dir>]
//
// The aliases are read from the data archive's misc/metadata.txt,
// keyTypeData.txt and timezoneTypes.txt with unzip, so that the cases cover
// exactly the tables the canonicalization reads. Each line is [tag, the
// canonical tag] or [tag, null] where Node throws.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");
const { execFileSync } = require("child_process");

const [zip, test262] = process.argv.slice(2);
if (!zip) {
  console.error("usage: node testdata/canonical_node.js <icu4c-78.3-data.zip> [<test262 dir>]");
  process.exit(2);
}
const read = name => execFileSync("unzip", ["-p", zip, "data/misc/" + name], { maxBuffer: 1 << 26 }).toString();

// The entries of an alias table: key and replacement.
function aliases(text, table) {
  const out = [];
  const re = new RegExp("^        " + table + "\\{([\\s\\S]*?)^        \\}", "m");
  const body = text.match(re);
  if (!body) return out;
  const entry = /^            ([^\s{]+)\{\s*\n\s*reason\{"[^"]*"\}\s*\n\s*replacement\{"([^"]*)"\}/gm;
  let m;
  while ((m = entry.exec(body[1]))) out.push([m[1], m[2]]);
  return out;
}

const metadata = read("metadata.txt");
const tags = new Set();
const add = t => tags.add(t);

for (const [k] of aliases(metadata, "language")) {
  const t = k.replace(/_/g, "-");
  add(t);
  if (!/-/.test(t)) {
    add(t + "-US");
    add(t + "-Latn-DE");
  }
}
const languages = ["und", "en", "ru", "hy", "az", "uz", "sr", "sh", "ka", "uk", "et", "nl", "fr"];
for (const [k] of aliases(metadata, "territory")) {
  for (const l of languages) add(l + "-" + k);
}
for (const [k] of aliases(metadata, "script")) {
  add("und-" + k);
  add("en-" + k + "-US");
}
for (const [k] of aliases(metadata, "variant")) {
  add("en-" + k);
  add("ja-Latn-" + k + "-hepburn");
  add("sl-" + k + "-rozaj-biske");
}
for (const [k] of aliases(metadata, "subdivision")) {
  add("en-u-sd-" + k);
  add("en-u-rg-" + k);
}

// Unicode extension types: every key's types in both spellings, and the
// aliases.
const keyTypes = read("keyTypeData.txt");
function block(text, name) {
  const re = new RegExp("^    " + name + "\\{([\\s\\S]*?)^    \\}", "m");
  const m = text.match(re);
  return m ? m[1] : "";
}
const keyMap = {};
for (const m of block(keyTypes, "keyMap").matchAll(/^        ([\w-]+)\{"([^"]*)"\}/gm)) {
  keyMap[m[1]] = m[2] || m[1];
}
for (const sub of block(keyTypes, "typeMap").matchAll(/^        ([\w-]+)(?::\w+)?\{([\s\S]*?)^        \}/gm)) {
  const bcpKey = keyMap[sub[1]] || sub[1];
  for (const m of sub[2].matchAll(/^            "?([^"\s{]+)"?\{"([^"]*)"\}/gm)) {
    const legacy = m[1].replace(/:/g, "/"), bcp = m[2] || m[1];
    add("en-u-" + bcpKey + "-" + bcp);
    if (/^[a-z0-9]{3,8}(-[a-z0-9]{3,8})*$/i.test(legacy)) add("en-u-" + bcpKey + "-" + legacy);
  }
}
for (const sub of block(keyTypes, "typeAlias").matchAll(/^        ([\w-]+)(?::\w+)?\{([\s\S]*?)^        \}/gm)) {
  const bcpKey = keyMap[sub[1]] || sub[1];
  for (const m of sub[2].matchAll(/^            "?([^"\s{]+)"?\{"([^"]*)"\}/gm)) {
    const alias = m[1].replace(/:/g, "/");
    if (/^[a-z0-9]{3,8}(-[a-z0-9]{3,8})*$/i.test(alias)) add("en-u-" + bcpKey + "-" + alias);
  }
}
for (const m of block(keyTypes, "bcpTypeAlias").matchAll(/^            ([\w-]+)\{"([^"]*)"\}/gm)) {
  add("en-u-ca-" + m[1]);
}
const zones = execFileSync("unzip", ["-p", zip, "data/misc/timezoneTypes.txt"], { maxBuffer: 1 << 26 }).toString();
for (const m of block(zones, "bcpTypeAlias").matchAll(/^            ([a-z0-9]+)\{"([^"]*)"\}/gm)) {
  add("en-u-tz-" + m[1]);
}
for (const k of Object.values(keyMap)) {
  if (/^[a-z0-9]{2}$/.test(k)) {
    add("en-u-" + k + "-true");
    add("en-u-" + k + "-yes");
    add("en-u-" + k);
  }
}

// Shapes that exercise the parser and the ordering rather than the data.
for (const t of [
  "EN-us", "en_GB", "en-u-ca-gregory-co-phonebk", "de-u-co-phonebk-ka-shifted", "en-t-zh-hant",
  "en-t-zh-hant-m0-ungegn", "sgn-GR", "i-klingon", "zh-min-nan", "en-GB-oed", "art-lojban",
  "cel-gaulish", "x-private", "en-a-foo-b-bar", "und-Latn", "root", "en-u-foo-bar-nu-thai",
  "en-u-attr-ca-gregory", "en-u-nu-thai-ca-gregory", "de-DE-1996-1901", "sl-rozaj-biske-1994",
  "en-US-u-va-posix", "ja-JP-u-ca-japanese-x-lvariant-JP", "th-TH-u-nu-thai", "zh-Hant-TW",
  "en-x-u-foo", "en-u-kn-false", "en-u-kn-true-kf-upper", "und-u-rg-gbzzzz", "en-US-u-sd-usca",
  "en-001", "es-419", "pt-BR-x-private-u-ca", "cmn", "zh-cmn-Hans", "en-latn-us", "EN-LATN-US",
  "de-1901-1901", "en-u-ca", "en-u", "en--US", "e", "en-US-", "123", "en-a", "en-u-c",
  "aaaaaaaaa", "en-US-ab", "en-t-en-t-en", "en-u-ca-japanese-u-nu-thai", "hy-SU", "und-SU",
  "tl", "no", "no-bok", "nb", "iw", "in", "ji", "jw", "mo", "sh-Cyrl", "zh-guoyu", "zh-hakka",
  "zh-xiang", "en-scouse", "en-US-t-mul-latn-h0-hybrid", "en-t-ru-x0-private", "und-t-und-latn",
]) add(t);

if (test262) {
  const dirs = ["test/intl402/Intl/getCanonicalLocales", "test/intl402/Locale"];
  for (const dir of dirs) {
    const full = path.join(test262, dir);
    if (!fs.existsSync(full)) continue;
    for (const f of fs.readdirSync(full)) {
      if (!f.endsWith(".js")) continue;
      const text = fs.readFileSync(path.join(full, f), "utf8");
      for (const m of text.matchAll(/["']([A-Za-z0-9]{1,8}(?:[-_][A-Za-z0-9]{1,8})+)["']/g)) add(m[1]);
    }
  }
}

const lines = [`# Node ${process.version}, ICU ${process.versions.icu}`];
for (const t of [...tags].sort()) {
  let out = null;
  try { out = Intl.getCanonicalLocales(t)[0]; } catch (e) { /* recorded as null */ }
  lines.push(JSON.stringify([t, out]));
}
const out = path.join(__dirname, "canonical_node.txt.gz");
fs.writeFileSync(out, zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
console.log(`${lines.length - 1} tags`);
