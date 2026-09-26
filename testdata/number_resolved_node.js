// Writes testdata/number_resolved_node.txt.gz: what Node's
// Intl.NumberFormat.prototype.resolvedOptions and
// Intl.PluralRules.prototype.resolvedOptions report for a matrix of
// option bags, one JSON array a line:
//
//	["NumberFormat" or "PluralRules", locale, options, resolvedOptions]
//
// resolvedOptions is {"error": name} where the constructor throws.
//
//	node testdata/number_resolved_node.js | gzip -9n > testdata/number_resolved_node.txt.gz
"use strict";

const out = [`# Node ${process.version}, ICU ${process.versions.icu}`];
function record(service, locale, options) {
  let r;
  try {
    r = new Intl[service](locale, options).resolvedOptions();
    if (service === "PluralRules") delete r.pluralCategories;
  } catch (e) {
    r = { error: e.name };
  }
  out.push(JSON.stringify([service, locale, options, r]));
}

const styles = [
  {}, { style: "percent" }, { style: "currency", currency: "USD" },
  { style: "currency", currency: "JPY", currencyDisplay: "code", currencySign: "accounting" },
  { style: "unit", unit: "meter", unitDisplay: "long" },
];
const notations = [{}, { notation: "compact" }, { notation: "compact", compactDisplay: "long" },
  { notation: "scientific" }];
const digits = [
  {}, { minimumFractionDigits: 1 }, { maximumFractionDigits: 0 }, { maximumFractionDigits: 5 },
  { minimumFractionDigits: 2, maximumFractionDigits: 4 }, { minimumSignificantDigits: 2 },
  { maximumSignificantDigits: 3 }, { minimumSignificantDigits: 2, maximumFractionDigits: 1 },
  { maximumSignificantDigits: 3, roundingPriority: "morePrecision" },
  { maximumFractionDigits: 2, roundingPriority: "lessPrecision" },
  { roundingPriority: "morePrecision" }, { minimumIntegerDigits: 3 },
  { minimumFractionDigits: 3, maximumFractionDigits: 1 },
];
const grouping = [{}, { useGrouping: false }, { useGrouping: "always" }, { useGrouping: "min2" },
  { useGrouping: "auto" }];
const rounding = [
  {}, { roundingMode: "ceil" }, { roundingMode: "halfEven", trailingZeroDisplay: "stripIfInteger" },
  { roundingIncrement: 5, maximumFractionDigits: 2, minimumFractionDigits: 2 },
  { roundingIncrement: 25, maximumFractionDigits: 1, minimumFractionDigits: 1, roundingMode: "floor" },
  { signDisplay: "exceptZero" }, { signDisplay: "negative" },
];

for (const locale of ["en", "de", "ja"]) {
  for (const s of styles) for (const n of notations) for (const d of digits) for (const g of grouping) {
    record("NumberFormat", locale, { ...s, ...n, ...d, ...g });
  }
  for (const s of styles) for (const r of rounding) record("NumberFormat", locale, { ...s, ...r });
  for (const type of ["cardinal", "ordinal"]) {
    for (const n of notations) for (const d of digits) {
      const { compactDisplay, ...plain } = n;
      record("PluralRules", locale, { type, ...plain, ...d });
    }
    for (const r of rounding) {
      const { signDisplay, ...plain } = r;
      record("PluralRules", locale, { type, ...plain });
    }
  }
}
process.stdout.write(out.join("\n") + "\n");
