// Writes testdata/resolved_keys_node.txt: every property Node's
// resolvedOptions reports for each Intl service, over option bags that
// bring out the properties reported only with others -- a currency's with
// the currency style, compactDisplay with compact notation, the fields of
// a date with no style. A line each: the service and its properties,
// sorted.
//
//	node testdata/resolved_keys_node.js > testdata/resolved_keys_node.txt
"use strict";

const bags = {
  Collator: [{}, { usage: "search", sensitivity: "base", ignorePunctuation: true, collation: "phonebk",
    numeric: true, caseFirst: "upper" }],
  DateTimeFormat: [{}, { dateStyle: "full", timeStyle: "full" }, { hour: "numeric", hourCycle: "h23" },
    { weekday: "long", era: "long", year: "numeric", month: "long", day: "numeric", dayPeriod: "long",
      hour: "numeric", minute: "numeric", second: "numeric", fractionalSecondDigits: 3,
      timeZoneName: "long", hour12: true }],
  DisplayNames: [{ type: "language", languageDisplay: "standard", fallback: "none", style: "short" },
    { type: "region" }, { type: "dateTimeField" }],
  DurationFormat: [{}, { style: "digital", fractionalDigits: 3 },
    { years: "long", yearsDisplay: "always", hours: "numeric", minutesDisplay: "always" }],
  ListFormat: [{}, { type: "unit", style: "narrow" }],
  NumberFormat: [{}, { style: "currency", currency: "USD", currencySign: "accounting" },
    { style: "unit", unit: "meter", unitDisplay: "long" }, { notation: "compact" },
    { minimumSignificantDigits: 2 }, { roundingPriority: "morePrecision" },
    { roundingIncrement: 5, maximumFractionDigits: 2, minimumFractionDigits: 2 }],
  PluralRules: [{}, { type: "ordinal", notation: "compact" }, { minimumSignificantDigits: 2 },
    { roundingPriority: "lessPrecision" }],
  RelativeTimeFormat: [{}, { numeric: "auto", style: "short" }],
  Segmenter: [{}, { granularity: "word" }],
};

const lines = [`# Node ${process.version}`];
for (const [service, list] of Object.entries(bags)) {
  const keys = new Set();
  for (const options of list) {
    for (const k of Object.keys(new Intl[service]("de", options).resolvedOptions())) keys.add(k);
  }
  lines.push(service + " " + [...keys].sort().join(" "));
}
process.stdout.write(lines.join("\n") + "\n");
