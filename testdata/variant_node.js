// Writes testdata/variant_node.txt.gz: what each Intl service does with
// "-u-va-posix", ICU's variant POSIX.
//
//	node testdata/variant_node.js
//
// ICU turns "-u-va-posix" into the locale's variant, not a keyword, so it
// survives V8's ResolveLocale in every service, and en_US_POSIX, ICU's one
// variant locale, has a collation, number patterns and word breaks of its
// own. Each line is a JSON array: [service, tag, resolved locale, outputs].
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const tags = ["en-US-u-va-posix", "en-u-va-posix", "en-GB-u-va-posix", "de-DE-u-va-posix", "ja-u-va-posix",
  "en-US-u-nu-arab-va-posix", "en-US-u-co-emoji-va-posix", "en-US-u-ca-buddhist-va-posix",
  "en-US-u-hc-h23-va-posix", "en-US", "fr-u-va-posix"];
const numbers = [1234567.891, -0.5, Infinity, NaN, 0.1234, 1e21];
const numberOptions = [{}, { style: "percent" }, { style: "currency", currency: "USD" },
  { style: "currency", currency: "EUR", currencyDisplay: "code" }, { notation: "compact" },
  { notation: "scientific" }, { style: "unit", unit: "kilometer" }, { maximumFractionDigits: 0 },
  { useGrouping: "always" }, { currencySign: "accounting", style: "currency", currency: "USD" },
  { notation: "engineering" }, { style: "unit", unit: "percent" }, { signDisplay: "always" }];
const words = ["b", "A", "a", "B", "_", "1", "é", "e", "[", "~", " ", "Z", "z", "0", "ä"];
const date = new Date(Date.UTC(2020, 5, 15, 13, 4, 5));
const text = "e-mail: someone@example.com; a:b 12:30 1.5 x_y";

const lines = [];
const out = (service, tag, resolved, result) => lines.push(JSON.stringify([service, tag, resolved, result]));

for (const tag of tags) {
  for (const o of numberOptions) {
    const nf = new Intl.NumberFormat(tag, o);
    out("NumberFormat", tag, nf.resolvedOptions().locale, [o, numbers.map(v => nf.format(v))]);
  }
  const c = new Intl.Collator(tag);
  out("Collator", tag, c.resolvedOptions().locale, [...words].sort(c.compare));
  for (const o of [{ dateStyle: "full", timeStyle: "full" }, { dateStyle: "short", timeStyle: "short" }]) {
    const d = new Intl.DateTimeFormat(tag, { ...o, timeZone: "UTC" });
    out("DateTimeFormat", tag, d.resolvedOptions().locale, [o, d.format(date)]);
  }
  const r = new Intl.RelativeTimeFormat(tag);
  out("RelativeTimeFormat", tag, r.resolvedOptions().locale, [r.format(1234567.5, "day"), r.format(-1, "day")]);
  const df = new Intl.DurationFormat(tag);
  out("DurationFormat", tag, df.resolvedOptions().locale, [df.format({ hours: 1234567, minutes: 5 })]);
  const p = new Intl.PluralRules(tag);
  out("PluralRules", tag, p.resolvedOptions().locale, [p.select(1), p.select(2)]);
  const l = new Intl.ListFormat(tag);
  out("ListFormat", tag, l.resolvedOptions().locale, [l.format(["a", "b", "c"])]);
  const s = new Intl.Segmenter(tag, { granularity: "word" });
  out("Segmenter", tag, s.resolvedOptions().locale, [[...s.segment(text)].map(x => x.segment)]);
  const n = new Intl.DisplayNames(tag, { type: "region" });
  out("DisplayNames", tag, n.resolvedOptions().locale, [n.of("US")]);
}

fs.writeFileSync(path.join(__dirname, "variant_node.txt.gz"),
  zlib.gzipSync(`# Node ${process.version}, ICU ${process.versions.icu}\n` + lines.join("\n") + "\n", { level: 9 }));
console.log(lines.length, "cases");
