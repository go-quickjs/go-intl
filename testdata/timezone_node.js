// Writes testdata/timezone_node.txt.gz: the time zones Node knows, their
// offsets and the names Intl.DateTimeFormat accepts for them.
//
//	node testdata/timezone_node.js
//
// Two kinds of line, each a JSON array:
//
//	["id", input, resolved]
//
// is Intl.DateTimeFormat's resolvedOptions().timeZone for a timeZone
// option, null where the constructor throws: every name ICU knows, in its
// own spelling, upper case and lower case, and some that are not zones.
//
//	["zone", name, offset, [ms, offset, ms, offset, ...]]
//
// is a zone's offset in seconds at the start of 1800 UTC and then each
// transition Temporal reports up to 2100: its instant in milliseconds and
// the offset from it. A zone's line is written for each name the zone files
// hold rules for.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const dir = path.join(__dirname, "..", "data", "tz");
const names = [];
(function walk(prefix) {
  for (const e of fs.readdirSync(path.join(dir, prefix), { withFileTypes: true })) {
    const name = prefix ? prefix + "/" + e.name : e.name;
    if (e.isDirectory()) walk(name);
    else names.push(name.replace(/\.bin$/, ""));
  }
})("");
names.sort();

const lines = [`# Node ${process.version}, ICU ${process.versions.icu}, tz ${process.versions.tz}`];
const inputs = new Set();
for (const n of names) {
  inputs.add(n);
  inputs.add(n.toUpperCase());
  inputs.add(n.toLowerCase());
}
for (const n of [
  "Etc/Unknown", "etc/unknown", "Unknown", "", " UTC", "UTC ", "Europe/", "/Europe", "Europe//Paris",
  "GMT", "gmt", "Etc/GMT", "etc/gmt+0", "Etc/GMT+15", "Etc/GMT-15", "Etc/GMT+05", "Etc/GMT0",
  "Etc/GMT+1", "etc/gmt-14", "GMT+0", "gmt-0", "GMT0", "gmt0", "GMT+5", "UTC+5", "UCT", "Z",
  "+05:00", "-00:00", "+00:00", "+14:00", "-14:00", "+23:59", "+24:00", "+0530", "+05", "-05:30:00",
  "US/Pacific-New", "SystemV/AST4", "systemv/ast4adt", "SystemV/EST5EDT", "America/Buenos_Aires",
  "america/argentina/comodrivadavia", "antarctica/dumontdurville", "ANTARCTICA/MCMURDO",
  "Australia/Lord_Howe", "australia/lhi", "nz-chat", "w-su", "gb-eire", "mexico/bajanorte",
  "America/Port_of_Spain", "america/port_of_spain", "Africa/Dar_es_Salaam", "africa/dar_es_salaam",
  "America/Port-au-Prince", "america/port-au-prince", "Europe/Isle_of_Man", "europe/isle_of_man",
  "Asia/Ho_Chi_Minh", "America/North_Dakota/New_Salem", "america/north_dakota/new_salem",
  "America/Indiana/Tell_City", "Pacific/Port_Moresby", "EST", "est", "MST7MDT", "mst7mdt",
  "EST5EDT", "est5edt", "CST6CDT", "PST8PDT", "HST", "ROC", "roc", "PRC", "prc", "ROK", "Zulu",
  "zulu", "Universal", "Greenwich", "Etc/Greenwich", "Etc/Zulu", "Etc/UCT", "etc/utc", "UTC",
  "Asia/Kolkata", "Europe/Kyiv", "America/Nuuk", "Pacific/Kanton", "Asia/Yangon", "America/Ciudad_Juarez",
  "Europe/Kiev", "Asia/Calcutta", "Asia/Rangoon", "America/Godthab", "Pacific/Enderbury",
]) inputs.add(n);

for (const input of [...inputs].sort()) {
  let resolved = null;
  try {
    resolved = new Intl.DateTimeFormat("en", { timeZone: input }).resolvedOptions().timeZone;
  } catch (e) {}
  lines.push(JSON.stringify(["id", input, resolved]));
}

const start = Temporal.Instant.from("1800-01-01T00:00:00Z");
const end = Temporal.Instant.from("2100-01-01T00:00:00Z").epochMilliseconds;
for (const n of names) {
  let z;
  try {
    z = start.toZonedDateTimeISO(n);
  } catch (e) {
    lines.push(JSON.stringify(["zone", n, null, []]));
    continue;
  }
  const offset = z.offsetNanoseconds / 1e9;
  const trans = [];
  for (;;) {
    z = z.getTimeZoneTransition("next");
    if (z === null || z.epochMilliseconds >= end) break;
    trans.push(z.epochMilliseconds, z.offsetNanoseconds / 1e9);
  }
  lines.push(JSON.stringify(["zone", n, offset, trans]));
}

fs.writeFileSync(path.join(__dirname, "timezone_node.txt.gz"), zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
