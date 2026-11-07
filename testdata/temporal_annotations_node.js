// Writes testdata/temporal_annotations_node.txt.gz: what Node's Temporal
// answers for annotations of one character and for those malformed at each
// point a parser can go wrong, in every format, through every call that
// reads a string. Chrome 158 parses them otherwise (AnnotationLookahead).
//
//	node testdata/temporal_annotations_node.js
//
// Each line is as testdata/temporal_node.js writes it: [op, receiver, args,
// result].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const formats = ["2020-06-15T12:00", "2020-06-15T12:00Z", "2020-06-15T12:00+01:00", "2020-06-15", "2020-06",
  "06-15", "12:00"];
const annotations = [
  // A key and a value of a character, and the same with what they may hold.
  "[u-ca=x]", "[a=b]", "[!a=b]", "[_=b]", "[a=1]", "[aa=b]", "[a=bb]", "[a1=b]", "[a-=b]", "[a_=b]", "[u-ca=a-b]",
  // A value's first character, and what follows a hyphen.
  "[aa=-b]", "[aa=!bc]", "[aa=_c]", "[aa=b-c]", "[aa=bc-d]", "[aa=bc-de]", "[aa=bc-d-e]", "[aa=b--c]", "[aa=bc-!d]",
  "[aa=bc-!de]", "[aa=b-]", "[aa=bc-]", "[aa=b_]", "[aa=b!c]",
  // Each place one can end, or lack a part.
  "[", "[a", "[aa", "[a=", "[aa=", "[aa=b", "[aa=bb", "[aa=bb-", "[aa=bb-c", "[=b]", "[=]", "[a]", "[aa]", "[A=b]",
  "[aA=b]", "[]",
  // Time zones, whose names are read the same way.
  "[Asia/Calcutta]", "[Asia/Calcutta/]", "[Asia/]", "[Asia/!]", "[Asia/!b]", "[Asia//Calcutta]", "[/Asia]", "[a/]",
  "[a/b]", "[UTC/]", "[UTC", "[UTC/", "[UTC!]", "[.]", "[_]", "[+01:00]",
  // After a zone, and two together.
  "[UTC][a=b]", "[UTC][u-ca=x]", "[UTC][!a=b]", "[UTC][aa=!bc]", "[a=b][c=d]", "[a=b][u-ca=iso8601]",
  "[u-ca=iso8601][a=b]", "[u-ca=x][u-ca=y]", "[!u-ca=x][u-ca=y]",
];
const names = ["a", "a/", "a/b", "Asia/", "Asia/Calcutta", "Asia/Calcutta/", "UTC/", "/UTC", "UTC", "."];
const types = ["Instant", "PlainDate", "PlainDateTime", "PlainMonthDay", "PlainTime", "PlainYearMonth",
  "ZonedDateTime"];

function enc(v) {
  if (v instanceof Temporal.Duration) return { $Duration: v.toString() };
  if (v instanceof Temporal.Instant) return { $Instant: v.toString() };
  if (v instanceof Temporal.PlainTime) return { $PlainTime: v.toString() };
  if (v instanceof Temporal.ZonedDateTime) {
    return { $ZonedDateTime: v.toString({ calendarName: "always" }) + " " + v.offset };
  }
  return { ["$" + v.constructor.name]: v.toString({ calendarName: "always" }) };
}

const lines = [];
function run(op, recv, args, call) {
  let result;
  try {
    result = enc(call());
  } catch (e) {
    result = { $err: e.name, msg: e.message };
  }
  lines.push(JSON.stringify([op, recv, args, result]));
}

const date = Temporal.PlainDate.from("2020-01-01");
const instant = Temporal.Instant.from("2020-01-01T00:00:00Z");
const day = Temporal.Duration.from("P1D");
const strings = [...names];
for (const f of formats) for (const a of annotations) strings.push(f + a);
for (const s of strings) {
  for (const t of types) run(`static:${t}.from`, null, [s], () => Temporal[t].from(s));
  run("method:PlainDate.withCalendar", { $PlainDate: "2020-01-01" }, [s], () => date.withCalendar(s));
  run("method:Instant.toZonedDateTimeISO", { $Instant: "2020-01-01T00:00:00Z" }, [s],
    () => instant.toZonedDateTimeISO(s));
  const options = { largestUnit: "hour", relativeTo: s };
  run("method:Duration.round", { $Duration: "P1D" }, [options], () => day.round(options));
}

fs.writeFileSync(path.join(__dirname, "temporal_annotations_node.txt.gz"),
  zlib.gzipSync(`# Node ${process.version}, ICU ${process.versions.icu}, tz ${process.versions.tz}\n` +
    lines.join("\n") + "\n", { level: 9 }));
console.log(lines.length, "cases");
