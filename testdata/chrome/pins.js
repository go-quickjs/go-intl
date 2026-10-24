// Writes testdata/chrome/pins.json: every stable Chrome for Windows from
// 140, grouped by the ICU it builds with, and the Chrome for Testing build
// each group's recordings are made with.
//
//	node testdata/chrome/pins.js
//
// A Chrome's ICU is the chromium/deps/icu commit its DEPS pins; the pin's
// README.chromium gives its ICU version and the tz data it carries. The
// recorded build is the newest of the group that Chrome for Testing has a
// win64 download of: its recordings go in testdata/chrome/<version>.
//
//	{"pins": [{"icu": commit, "icuVersion": "78.2", "tz": "2026c",
//	           "versions": [...], "recorded": "155.0.8059.84"}, ...]}
//
// in the order the pins were taken.
"use strict";
const fs = require("fs");
const path = require("path");

const first = 140;

async function get(url) {
  for (let i = 0; ; i++) {
    try {
      const r = await fetch(url);
      if (!r.ok) throw new Error(`${url}: ${r.status}`);
      return await r.text();
    } catch (e) {
      if (i === 3) throw e;
      await new Promise(r => setTimeout(r, 2000 * (i + 1)));
    }
  }
}

// gitiles serves a file's text as base64.
const file = async (url) => Buffer.from(await get(url + "?format=TEXT"), "base64").toString();

const order = (a, b) => {
  const x = a.split(".").map(Number), y = b.split(".").map(Number);
  for (let i = 0; i < 4; i++) if (x[i] !== y[i]) return x[i] - y[i];
  return 0;
};

async function main() {
  const releases = JSON.parse(await get("https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Windows&num=1000"));
  const versions = [...new Set(releases.map(r => r.version))].filter(v => Number(v.split(".")[0]) >= first).sort(order);
  const known = JSON.parse(await get("https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json"));
  const testing = new Set(known.versions
    .filter(v => (v.downloads.chrome || []).some(d => d.platform === "win64")).map(v => v.version));

  const pins = [];
  for (const v of versions) {
    const deps = await file(`https://chromium.googlesource.com/chromium/src/+/refs/tags/${v}/DEPS`);
    const m = deps.match(/\/chromium\/deps\/icu\.git'\s*\+\s*'@'\s*\+\s*'([0-9a-f]{40})'/);
    if (!m) throw new Error(`${v}: no ICU pin in DEPS`);
    let pin = pins.find(p => p.icu === m[1]);
    if (!pin) {
      const readme = await file(`https://chromium.googlesource.com/chromium/deps/icu/+/${m[1]}/README.chromium`);
      const version = readme.match(/^Version:\s*(\S+)/m);
      const tz = readme.match(/latest version is (\d{4}[a-z])/);
      if (!version || !tz) throw new Error(`${m[1]}: README.chromium names no version or tz data`);
      pin = { icu: m[1], icuVersion: version[1].replace("-", "."), tz: tz[1], versions: [], recorded: null };
      pins.push(pin);
    }
    pin.versions.push(v);
    if (testing.has(v)) pin.recorded = v;
  }
  const out = path.join(__dirname, "pins.json");
  fs.writeFileSync(out, JSON.stringify({ pins }, null, 1) + "\n");
  for (const p of pins) {
    console.log(`${p.icu.slice(0, 10)} ICU ${p.icuVersion} tz ${p.tz}: ${p.versions[0]} to ${p.versions[p.versions.length - 1]}, recorded ${p.recorded}`);
  }
}

main().catch((e) => {
  console.error(e.stack || e.message);
  process.exit(1);
});
