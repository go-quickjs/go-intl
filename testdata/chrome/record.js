// Runs one of the *_node.js recording scripts in a Chrome rather than in
// Node, so that a recording holds that Chrome's answers:
//
//	node testdata/chrome/record.js [--shards N] <chrome.exe> <ICU version> <tz version> <out dir> <script.js> [args...]
//
// The script runs twice. In Node first, with what it reads of the file
// system -- directory listings, files, unzip's output -- and every path it
// builds written down, and nothing written. Then in a blank Chrome page,
// over the DevTools protocol (cdp.js), with require answering from what
// Node wrote down, so the cases are the same and only the Intl answers
// are Chrome's. The scripts it requires of its own (./datalocales.js) run
// the same way in both. A "# Node ..." header line names the Chrome
// instead. What it would write goes to <out dir> as <name>.delta.gz, the
// edits that make it from Node's recording of the same name beside the
// script (delta.js), which differs in a few hundred lines where it has
// tens of thousands; a recording Node has none of is written whole. What a
// script writes to stdout is the recording beside it named for it,
// <script>.txt.gz or <script>.txt. The page's time zone is TZ, or UTC, and
// its locale en-US, as the Node scripts run, or the language --lang names:
// Chrome runs in it, which makes it ICU's default locale, and the header
// says so, ", language ko-KR". The header keeps what the script wrote after
// its Node and ICU versions: the clock date_node.js asked for its zones'
// names at, which is the Chrome's own.
//
// A script that sets process.env.TZ, as date_node.js does zone by zone,
// changes the page's time zone where it does: each "process.env.TZ = x;"
// waits while the page asks for x over the protocol, which this sets as
// the page's time zone before the script goes on.
//
// Chrome keeps what ICU loads for a locale until the process ends, which
// is more than a page has room for when a script formats in every
// locale. --shards N runs it in N Chromes one after another, each given
// the next slice of the locales dataLocales lists, and joins what they
// write, the header once. It is for a script every line of whose
// recording but the header comes of one of those locales.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");
const childProcess = require("child_process");
const { launch } = require("./cdp.js");
const { diff } = require("./delta.js");

const usage = "usage: node testdata/chrome/record.js [--shards N] [--lang tag] <chrome.exe> <ICU version> <tz version> <out dir> <script.js> [args...]";

async function main() {
  const argv = process.argv.slice(2);
  let shards = 1;
  let lang = null;
  while (argv[0] === "--shards" || argv[0] === "--lang") {
    if (argv[0] === "--shards") {
      shards = Number(argv[1]);
      if (!Number.isInteger(shards) || shards < 1) {
        console.error(usage);
        process.exit(2);
      }
    } else {
      lang = argv[1];
    }
    argv.splice(0, 2);
  }
  const [chromePath, icu, tz, outDir, script, ...args] = argv;
  if (!script) {
    console.error(usage);
    process.exit(2);
  }
  const scriptPath = path.resolve(script);
  const source = fs.readFileSync(scriptPath, "utf8");
  const { tape, modules, resolved } = recordInNode(scriptPath, source, args);

  // What the page writes, by path, the shards' joined.
  const written = new Map();
  let browser;
  for (let shard = 0; shard < shards; shard++) {
    const chrome = await launch(chromePath, { lang: lang || "en-US" });
    try {
      const timeZone = process.env.TZ || "UTC";
      await chrome.send("Emulation.setTimezoneOverride", { timezoneId: timeZone });
      // The time zone the script sets: an override is taken away before
      // another is set, which Chrome refuses while one is in effect.
      await chrome.send("Runtime.enable", {});
      await chrome.send("Runtime.addBinding", { name: "__recordSetTimeZone" });
      chrome.on("Runtime.bindingCalled", async ({ name, payload }) => {
        if (name !== "__recordSetTimeZone") return;
        let failed = null;
        try {
          await chrome.send("Emulation.setTimezoneOverride", { timezoneId: "" });
          await chrome.send("Emulation.setTimezoneOverride", { timezoneId: payload });
        } catch (e) {
          failed = `the time zone ${payload}: ${e.message}`;
        }
        await chrome.send("Runtime.evaluate", { expression: `__recordTimeZoneSet(${JSON.stringify(failed)})` });
      });
      await chrome.send("Emulation.setLocaleOverride", { locale: lang || "en-US" });
      browser = chrome.version; // "Chrome/155.0.8059.84"
      const page = pageProgram(source, {
        tape, modules, resolved, shard: shards > 1 ? { index: shard, count: shards } : null,
        argv: [process.execPath, scriptPath, ...args], filename: scriptPath,
        dirname: path.dirname(scriptPath), env: { TZ: process.env.TZ },
        version: browser.replace(/^[^/]*\//, "Chrome "), versions: { icu, tz },
      });
      await chrome.evaluate(page);
      for (const o of await readOutputs(chrome)) {
        const had = written.get(o.path);
        if (!had) {
          written.set(o.path, o);
          continue;
        }
        // A later shard's header is the first's.
        had.data += o.data.startsWith("#") ? o.data.slice(o.data.indexOf("\n") + 1) : o.data;
      }
    } finally {
      await chrome.close();
    }
    if (shards > 1) console.log(`shard ${shard + 1} of ${shards} done`);
  }

  fs.mkdirSync(outDir, { recursive: true });
  for (const o of written.values()) {
    // "# Node Chrome 155.0.8059.84, ICU 78.2, tz 2026c, now ...": the
    // Chrome and its ICU, then what the script wrote after them.
    const header = `# ${browser.replace("/", " ")}, ICU ${icu}` + (lang ? `, language ${lang}` : "");
    const text = lang
      ? o.data.replace(/^# [Nn]ode [^\n,]*,? ICU [^\n, ]*/, header)
      : o.data.replace(/^# [Nn]ode [^\n]*/, header);
    let name = path.basename(o.path);
    let gzip = o.gzip;
    if (o.stdout) {
      const stem = path.basename(scriptPath, ".js");
      name = fs.existsSync(path.join(path.dirname(scriptPath), stem + ".txt.gz")) ? stem + ".txt.gz" : stem + ".txt";
      gzip = name.endsWith(".gz");
    }
    // Node's recording of the same cases, where there is one: Chrome's
    // is kept as the edits from it (delta.js).
    const base = path.join(path.dirname(scriptPath), name);
    if (fs.existsSync(base)) {
      const raw = fs.readFileSync(base);
      const nodeText = (name.endsWith(".gz") ? zlib.gunzipSync(raw) : raw).toString();
      const edits = diff(nodeText.split("\n"), text.split("\n"));
      const changed = edits.filter(e => e[0] !== "=").length;
      const target = path.join(outDir, name + ".delta.gz");
      fs.writeFileSync(target, zlib.gzipSync(JSON.stringify({ base: name, edits }), { level: 9 }));
      console.log(`${target}: ${changed} runs of edits from Node's ${name}`);
      continue;
    }
    const target = path.join(outDir, name);
    fs.writeFileSync(target, gzip ? zlib.gzipSync(text, { level: 9 }) : text);
    console.log(`${target}: ${text.length} characters, no recording of Node's to differ from`);
  }
}

// recordInNode runs the script in Node with fs, path and child_process
// written down and its writes thrown away. It returns what was written
// down, the sources of the scripts of its own it required, by path, and
// the path each require named, by the directory it was required from.
function recordInNode(scriptPath, source, args) {
  const tape = {};
  const note = (name, a, value) => {
    tape[name + JSON.stringify(a)] = value;
    return value;
  };
  const read = (name, f) => (...a) => {
    let value;
    try {
      value = f(...a);
    } catch (e) {
      note(name, a, { error: e.message });
      throw e;
    }
    note(name, a, { value: taped(value) });
    return value;
  };
  // A Buffer is kept as base64 and a directory entry as what it answers,
  // which JSON would lose.
  const taped = (v) => {
    if (Buffer.isBuffer(v)) return { buffer: v.toString("base64") };
    if (Array.isArray(v) && v.length > 0 && v[0] instanceof fs.Dirent) {
      return { dirents: v.map(e => ({ name: e.name, directory: e.isDirectory(), file: e.isFile() })) };
    }
    return v;
  };
  const fakeFs = {
    readdirSync: read("fs.readdirSync", fs.readdirSync),
    readFileSync: read("fs.readFileSync", (p, enc) => fs.readFileSync(p, enc)),
    existsSync: read("fs.existsSync", fs.existsSync),
    statSync: read("fs.statSync", (p) => { const s = fs.statSync(p); return { isDirectory: s.isDirectory(), isFile: s.isFile() }; }),
    writeFileSync() {},
    mkdirSync() {},
  };
  const fakePath = {};
  for (const k of ["join", "resolve", "basename", "dirname", "extname", "relative", "normalize"]) {
    fakePath[k] = read("path." + k, path[k]);
  }
  fakePath.sep = path.sep;
  const fakeChild = { execFileSync: read("child_process.execFileSync", (...a) => childProcess.execFileSync(...a)) };
  const builtins = {
    fs: fakeFs, path: fakePath, child_process: fakeChild, zlib: { gzipSync: (d) => d },
  };
  const fakeProcess = Object.create(process, {
    argv: { value: [process.execPath, scriptPath, ...args] },
    env: { value: { ...process.env } },
    exit: { value: (code) => { throw new Error("the script exited with " + code); } },
    stdout: { value: { write() {} } },
  });
  const fakeConsole = { log() {}, error: console.error, warn() {} };

  const modules = {};
  const resolved = {};
  const loaded = {};
  const run = (file, src) => {
    const dir = path.dirname(file);
    const module = { exports: {} };
    const req = (name) => {
      if (name in builtins) return builtins[name];
      if (!name.startsWith("./") && !name.startsWith("../")) throw new Error(`${file} requires ${name}, which the page has not`);
      const p = path.resolve(dir, name);
      resolved[dir + "\n" + name] = p;
      if (!(p in loaded)) {
        modules[p] = { dir: path.dirname(p), source: fs.readFileSync(p, "utf8") };
        loaded[p] = run(p, modules[p].source);
      }
      return loaded[p];
    };
    const wrapped = new Function("require", "process", "__dirname", "__filename", "module", "exports", "console", src);
    wrapped(req, fakeProcess, dir, file, module, module.exports, fakeConsole);
    return module.exports;
  };
  run(scriptPath, source);
  return { tape, modules, resolved };
}

// pageProgram is the script wrapped to run in the page with require
// answering from the tape and writes kept for readOutputs.
function pageProgram(source, env) {
  // The script's settings of its time zone, each waited for (main).
  const settings = /process\.env\.TZ\s*=\s*([^;]+);/g;
  const main = source.replace(settings, "await __recordTimeZone($1);");
  return `(async () => {
  const env = ${JSON.stringify(env)};
  const outputs = [];
  globalThis.__recordOutputs = outputs;
  // A Buffer as far as the scripts use one: bytes, and toString.
  class Bytes extends Uint8Array {
    toString(encoding) {
      if (encoding === "latin1" || encoding === "binary") {
        let s = "";
        for (const b of this) s += String.fromCharCode(b);
        return s;
      }
      return new TextDecoder().decode(this);
    }
  }
  const replay = (name) => (...a) => {
    const key = name + JSON.stringify(a);
    if (!(key in env.tape)) throw new Error("not recorded in Node: " + key);
    const r = env.tape[key];
    if (r.error) throw new Error(r.error);
    if (r.value && r.value.buffer !== undefined) return Bytes.from(atob(r.value.buffer), c => c.charCodeAt(0));
    if (r.value && r.value.dirents !== undefined) {
      return r.value.dirents.map(e => ({ name: e.name, isDirectory: () => e.directory, isFile: () => e.file }));
    }
    return r.value;
  };
  const gzipped = (d) => ({ __gzip: String(d) });
  const fs = {
    readdirSync: replay("fs.readdirSync"), readFileSync: replay("fs.readFileSync"),
    existsSync: replay("fs.existsSync"),
    statSync: (...a) => { const s = replay("fs.statSync")(...a); return { isDirectory: () => s.isDirectory, isFile: () => s.isFile }; },
    mkdirSync() {},
    writeFileSync(p, d) {
      outputs.push(d && d.__gzip !== undefined ? { path: p, data: d.__gzip, gzip: true } : { path: p, data: String(d), gzip: false });
    },
  };
  const path = { sep: ${JSON.stringify(path.sep)} };
  for (const k of ["join", "resolve", "basename", "dirname", "extname", "relative", "normalize"]) path[k] = replay("path." + k);
  const builtins = {
    fs, path, zlib: { gzipSync: gzipped }, child_process: { execFileSync: replay("child_process.execFileSync") },
  };
  let stdout = "";
  const process = {
    argv: env.argv, env: env.env, version: env.version, versions: env.versions,
    exit(code) { throw new Error("the script exited with " + code); },
    stdout: { write(s) { stdout += s; } },
  };
  const console = { log() {}, error() {}, warn() {} };
  const __recordTimeZone = (zone) => new Promise((resolve, reject) => {
    globalThis.__recordTimeZoneSet = (failed) => failed === null ? resolve() : reject(new Error(failed));
    process.env.TZ = zone;
    __recordSetTimeZone(String(zone));
  });
  const loaded = {};
  const requireFrom = (dir) => (name) => {
    if (name in builtins) return builtins[name];
    const p = env.resolved[dir + "\\n" + name];
    if (p === undefined) throw new Error("no " + name + " in the page");
    if (!(p in loaded)) loaded[p] = shard(run(p, env.modules[p].dir, env.modules[p].source));
    return loaded[p];
  };
  const run = (file, dir, src) => {
    const module = { exports: {} };
    new Function("require", "process", "__dirname", "__filename", "module", "exports", "console", src)(
      requireFrom(dir), process, dir, file, module, module.exports, console);
    return module.exports;
  };
  // The shard's slice of the locales, where the script is run in shards.
  const shard = (exports) => {
    if (!env.shard || typeof exports.dataLocales !== "function") return exports;
    const all = exports.dataLocales;
    return { ...exports, dataLocales: (...a) => {
      const locales = all(...a);
      const size = Math.ceil(locales.length / env.shard.count);
      return locales.slice(env.shard.index * size, (env.shard.index + 1) * size);
    } };
  };
  const AsyncFunction = (async () => {}).constructor;
  await new AsyncFunction("require", "process", "__dirname", "__filename", "module", "exports", "console",
    "__recordTimeZone", ${JSON.stringify(main)})(
    requireFrom(env.dirname), process, env.dirname, env.filename, { exports: {} }, {}, console, __recordTimeZone);
  if (stdout !== "") outputs.push({ path: env.filename, data: stdout, gzip: false, stdout: true });
  return outputs.length;
})()`;
}

// readOutputs fetches what the page wrote, a piece at a time.
async function readOutputs(chrome) {
  const n = await chrome.evaluate("__recordOutputs.length");
  const out = [];
  for (let i = 0; i < n; i++) {
    const meta = await chrome.evaluate(`({ path: __recordOutputs[${i}].path, gzip: __recordOutputs[${i}].gzip, stdout: !!__recordOutputs[${i}].stdout, length: __recordOutputs[${i}].data.length })`);
    let data = "";
    const piece = 8 << 20;
    for (let at = 0; at < meta.length; at += piece) {
      data += await chrome.evaluate(`__recordOutputs[${i}].data.slice(${at}, ${at + piece})`);
    }
    out.push({ path: meta.path, gzip: meta.gzip, stdout: meta.stdout, data });
  }
  return out;
}

main().catch((e) => {
  console.error(e.stack || e.message);
  process.exit(1);
});
