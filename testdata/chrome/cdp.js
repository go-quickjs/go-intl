// Runs JavaScript in a Chrome, for recording what that Chrome's Intl
// answers as the *_node.js scripts record Node's. It drives Chrome for
// Testing headless over the DevTools protocol, with nothing but Node.
//
//	const chrome = await launch("/path/to/chrome.exe");
//	const value = await chrome.evaluate("Intl.DateTimeFormat().resolvedOptions().locale");
//	await chrome.close();
//
// evaluate runs an expression in a blank page and returns its value, which
// must be JSON; an expression that throws rejects with the exception's text.
"use strict";
const { spawn, spawnSync } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

async function launch(executable, { lang = "en-US", timeZone = "UTC" } = {}) {
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), "chrome-rec-"));
  const proc = spawn(executable, [
    "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
    "--remote-debugging-port=0", `--user-data-dir=${profile}`, `--lang=${lang}`, "about:blank",
  ], { stdio: "ignore", env: { ...process.env, TZ: timeZone } });

  // Chrome writes the port it chose to DevToolsActivePort in the profile.
  const portFile = path.join(profile, "DevToolsActivePort");
  let port;
  for (let i = 0; i < 300 && !port; i++) {
    if (fs.existsSync(portFile)) port = fs.readFileSync(portFile, "utf8").split("\n")[0].trim();
    if (!port) await new Promise(r => setTimeout(r, 100));
  }
  if (!port) {
    proc.kill();
    throw new Error("Chrome did not start: no DevToolsActivePort");
  }
  const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const page = targets.find(t => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });

  let next = 1;
  const waiting = new Map();
  const handlers = new Map();
  ws.onmessage = e => {
    const msg = JSON.parse(e.data);
    if (msg.id === undefined) {
      const h = handlers.get(msg.method);
      if (h) h(msg.params);
      return;
    }
    const w = waiting.get(msg.id);
    if (!w) return;
    waiting.delete(msg.id);
    if (msg.error) w.reject(new Error(msg.error.message));
    else w.resolve(msg.result);
  };
  const send = (method, params) => new Promise((resolve, reject) => {
    const id = next++;
    waiting.set(id, { resolve, reject });
    ws.send(JSON.stringify({ id, method, params }));
  });

  return {
    version: (await (await fetch(`http://127.0.0.1:${port}/json/version`)).json()).Browser,
    // send is one DevTools protocol command to the page, and on what is
    // done with each of an event's notifications.
    send,
    on(method, handler) { handlers.set(method, handler); },
    async evaluate(expression) {
      const r = await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
      if (r.exceptionDetails) {
        const d = r.exceptionDetails;
        throw new Error((d.exception && d.exception.description) || d.text);
      }
      return r.result.value;
    },
    async close() {
      ws.close();
      // The whole tree: on Windows, killing the browser leaves its
      // renderers running, each holding what it had loaded.
      const exited = proc.exitCode !== null || proc.signalCode !== null
        ? Promise.resolve() : new Promise(r => proc.once("exit", r));
      if (process.platform === "win32") {
        spawnSync("taskkill", ["/pid", String(proc.pid), "/T", "/F"], { stdio: "ignore" });
      } else {
        proc.kill();
      }
      await exited;
      fs.rmSync(profile, { recursive: true, force: true, maxRetries: 5 });
    },
  };
}

module.exports = { launch };
