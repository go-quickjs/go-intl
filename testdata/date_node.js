// Writes testdata/date_node.txt.gz: what Node's Date writes and reads in
// every time zone Node knows, in the locale Node runs in here.
//
//	node testdata/date_node.js
//
// The header has Date.now() and the default locale: V8 names a zone's
// standard and daylight time as ICU does at the current time, in the
// default locale. Lines are JSON arrays:
//
//	["zone", zone]                                 process.env.TZ from here on
//	["str", t, toString, toDateString, toTimeString, getTimezoneOffset]
//
// in that order for each time, in the order of the times, so that V8's
// offset cache is asked what the replay asks.
//	["local", [y, m, d, h, min, s, ms], getTime]   new Date(y, m, ...)
//	["parse", zone, string (UTF-16 units), Date.parse]
//	["utc", t, toUTCString, toISOString or null]
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const now = Date.now();
const locale = new Intl.DateTimeFormat().resolvedOptions().locale;
const lines = [`# Node ${process.version}, ICU ${process.versions.icu}, tz ${process.versions.tz}, now ${now}, locale ${locale}`];
const emit = (...a) => lines.push(JSON.stringify(a));

const fixed = [0, -1, 1, 1e12, -1e12, 1e14, -1e14, 8.64e15, -8.64e15, 2147483647000, 2147483648000,
  -2208988800000, -62135596800000, -62198755200000, 253402300799999, 1782864000000, 1767225600000,
  Date.UTC(2026, 8, 25, 12), Date.UTC(1970, 0, 1, 12), Date.UTC(1900, 6, 1), Date.UTC(1800, 0, 1),
  Date.UTC(2100, 6, 1), Date.UTC(-500, 2, 1), Date.UTC(2038, 6, 1), Date.UTC(1969, 11, 31, 23, 59, 59)];

const zones = Intl.supportedValuesOf("timeZone").concat(["Europe/Dublin", "UTC", "Etc/GMT+5", "Etc/GMT-14"]);
const start = Temporal.Instant.from("1965-01-01T00:00Z");
const end = Date.UTC(2040, 0, 1);
for (const zone of zones) {
  process.env.TZ = zone;
  emit("zone", zone);
  const times = new Set(fixed);
  let z = start.toZonedDateTimeISO(zone);
  let n = 0;
  for (;;) {
    z = z.getTimeZoneTransition("next");
    if (z === null || z.epochMilliseconds >= end || n++ >= 40) break;
    const t = z.epochMilliseconds;
    times.add(t - 1); times.add(t); times.add(t - 1000);
    // Local wall times either side of the transition, and inside a gap
    // or an overlap, read with Temporal: a Date's getters would go through
    // V8's offset cache, whose answers depend on what it was asked before,
    // and the replay could not ask the same.
    for (const ms of [t - 1, t]) {
      const w = Temporal.Instant.fromEpochMilliseconds(ms).toZonedDateTimeISO(zone);
      for (const delta of [-30, 0, 30, 60, 90]) {
        const f = [w.year, w.month - 1, w.day, w.hour, w.minute + delta, w.second, 0];
        emit("local", f, new Date(...f).getTime());
      }
    }
  }
  for (const t of [...times].sort((a, b) => a - b)) {
    const d = new Date(t);
    emit("str", t, d.toString(), d.toDateString(), d.toTimeString(), d.getTimezoneOffset());
  }
  for (const f of [[2026, 0, 1], [1970, 0, 1], [-1, 0, 1], [275760, 8, 13], [-271821, 3, 20, 1], [2026, 13, 45, 30, 70],
    [99, 0, 1], [2026, -5, -5, -5]]) {
    emit("local", f, new Date(...f).getTime());
  }
}

const units = s => { const u = []; for (let i = 0; i < s.length; i++) u.push(s.charCodeAt(i)); return u; };
const strings = [
  "2026-09-25", "2026-09-25T12:34", "2026-09-25T12:34:56", "2026-09-25T12:34:56.789", "2026-09-25T12:34:56.789Z",
  "2026-09-25T12:34:56+05:30", "2026-09-25T12:34:56-0800", "2026-09-25T24:00", "2026-09-25T24:00:01", "2026-09",
  "2026", "+002026-09-25", "-000001-01-01T00:00:00Z", "-000000-01-01", "+275760-09-13T00:00:00.000Z",
  "+275760-09-13T00:00:00.001Z", "-271821-04-20T00:00:00Z", "-271821-04-19T23:59:59.999Z", "2026-13-01",
  "2026-02-30", "2026-02-29", "2024-02-29", "2026-9-25", "20260925", "2026-09-25T12", "2026-09-25T1234",
  "2026-09-25T12:34:56.1", "2026-09-25T12:34:56.12345678901", "2026-09-25t12:34z", "2026-09-25 12:34",
  "Sep 25 2026", "25 Sep 2026", "September 25, 2026", "Sept 25 2026", "Thu Sep 25 2026 12:34:56 GMT-0400 (EDT)",
  "Thu, 25 Sep 2026 12:34:56 GMT", "Thursday, September 25, 2026 12:34 PM", "9/25/2026", "9/25/26", "9/25/49",
  "9/25/50", "25/9/2026", "2026/09/25", "2026/9/25 12:34:56", "Sep 25, 2026 12:34:56 PST", "12:34 Sep 25 2026",
  "Sep 25 2026 12:34 pm", "Sep 25 2026 12:34 am", "Sep 25 2026 13:34 pm", "Sep 25 2026 12 pm", "Sep 25 2026 0:00 am",
  "Sep 25 2026 12:34:56.789", "Sep 25 2026 12:34:56.", "Sep 25 2026 12::", "Sep 25 2026 12:34 +0530",
  "Sep 25 2026 12:34 GMT+5", "Sep 25 2026 12:34 UTC+05:30", "Sep 25 2026 12:34 UT", "Sep 25 2026 12:34 Z",
  "Sep 25 2026 12:34 z", "Sep 25 2026 12:34 EDT", "Sep 25 2026 12:34 edt", "Sep 25 2026 12:34 CST",
  "Sep 25 2026 12:34 MST", "Sep 25 2026 12:34 PDT", "Sep 25 2026 12:34 XYZ", "garbage Sep 25 2026",
  "Sep garbage 25 2026", "Sep 25 2026 garbage", "(comment) Sep 25 2026", "Sep 25 (comment (nested)) 2026",
  "Sep 25 2026)", "Sep 25 2026 12:34 -", "Sep 25 2026 12:34 +", "Sep 25 2026 12:34 GMT-12345",
  "Sep 25 2026 12:34 GMT-8:", "Sep-25-2026", "25-Sep-2026", "2026-Sep-25", "Sep 25", "25 Sep", "Sep 2026",
  "1 2 3", "1 2 3 4", "32 1 2026", "0 1 2026", "Jan 0 2026", "Jan 32 2026", "12:34", "12:34:56 Sep 25 2026",
  "Sep 25 2026 25:00", "Sep 25 2026 24:00", "Sep 25 2026 23:60", "Sep 25 2026 12:34:60", "Sep 25 10000",
  "Sep 25 275760", "Sep 25 -2026", "Sep 25 +2026", " Sep  25　 2026﻿", "Sep\n25\r2026 ",
  "Sep 25 2026 ", "Sep\u0000 25 2026", "Sep 25 2026 12:34 am pm", "Tue Sep 25 2026", "Mon, 25 Sep 2026 12:34:56 +0000",
  "", " ", "Invalid Date", "NaN", "0", "1", "2026", "-2026", "99", "100", "1e3", "Jan", "January 1st 2026",
  "2026-09-25T12:34:56.789+05:30junk", "2026-09-25junk", "2026-09-25 junk", "T12:34", "2026-09-25T",
  "Sep 25 2026 12:34:56 GMT+0100 (Irish Standard Time)", "Sat Jan 01 -001 00:00:00 GMT+0000",
  "Fri Dec 31 +275760", "1/1/1970 00:00:00 GMT+0000", "1970-01-01T00:00:00.000+00:00", "12/31/1969 19:00:00 EST",
  "Mar 29 2026 01:30", "Oct 25 2026 01:30", "Mar 8 2026 02:30", "Nov 1 2026 01:30",
];
for (const zone of ["UTC", "America/New_York", "Europe/Dublin", "Asia/Kolkata"]) {
  process.env.TZ = zone;
  for (const s of strings) emit("parse", zone, units(s), Date.parse(s));
}
for (const t of fixed.concat([NaN, 1.5, -1.5, 8.64e15 + 1])) {
  const d = new Date(t);
  let iso = null;
  try { iso = d.toISOString(); } catch (e) {}
  emit("utc", Number.isNaN(t) ? null : t, d.toUTCString(), iso);
}
fs.writeFileSync(path.join(__dirname, "date_node.txt.gz"), zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
