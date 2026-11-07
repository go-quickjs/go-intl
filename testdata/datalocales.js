// The locales the recordings ask about, which recorded_locales.txt fixes. A
// recording is compared with what each Chrome and Node answers to the same
// cases, so they do not follow what the data directory holds now: these are
// the locales it had dates for when the recordings were made. A line is a
// locale, and "same" after it where its dates were another locale's.
//
//	const { recordedLocales } = require("./datalocales.js");
//	const locales = recordedLocales();
"use strict";
const fs = require("fs");
const path = require("path");

const lines = fs.readFileSync(path.join(__dirname, "recorded_locales.txt"), "latin1")
  .split("\n").filter(l => l !== "").map(l => l.split(" "));

// recordedLocales is every one of them, sorted as file names sort.
const recordedLocales = () => lines.map(l => l[0]);

// recordedOwnLocales is those whose dates were their own.
const recordedOwnLocales = () => lines.filter(l => l.length === 1).map(l => l[0]);

module.exports = { recordedLocales, recordedOwnLocales };
