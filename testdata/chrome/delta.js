// A recording as it differs from another, line by line: Chrome's answers
// are nearly all Node's, so a Chrome recording is kept as the edits that
// make it from Node's (testdata/*_node.*), a few hundred lines where the
// recording is tens of thousands.
//
// The edits are runs: "=" keeps n lines, "-" drops n, "+" adds the lines
// given. apply reverses diff exactly.
"use strict";

// Stretches no longer than this, in lines of both, are diffed by Myers's
// algorithm, whose memory is the square of the edits; longer ones that
// share no line with a match are replaced whole.
const myersLimit = 4000;

// diff is the edits from a to b, arrays of lines. It is a patience diff:
// the lines that occur once in each, matched in the longest order they
// keep in both, are kept, and what lies between two of them is diffed the
// same way, down to stretches with no such line, which go to Myers's
// algorithm. Its memory is linear in the lines however many differ: a
// recording a Chrome answers differently in tens of thousands of lines,
// as it does the locales it has no data for, is more than Myers's alone
// has room for.
function diff(a, b) {
  const out = [];
  const run = (op, n, lines) => {
    const last = out[out.length - 1];
    if (op === "+") {
      if (last && last[0] === "+") { for (const l of lines) last[1].push(l); } else out.push(["+", lines.slice()]);
    } else if (n > 0) {
      if (last && last[0] === op) last[1] += n; else out.push([op, n]);
    }
  };
  const section = (a0, a1, b0, b1) => {
    let start = 0;
    while (a0 + start < a1 && b0 + start < b1 && a[a0 + start] === b[b0 + start]) start++;
    let end = 0;
    while (a1 - end > a0 + start && b1 - end > b0 + start && a[a1 - end - 1] === b[b1 - end - 1]) end++;
    run("=", start);
    a0 += start; b0 += start; a1 -= end; b1 -= end;
    if (a0 === a1) {
      if (b0 < b1) run("+", 0, b.slice(b0, b1));
    } else if (b0 === b1) {
      run("-", a1 - a0);
    } else {
      const anchors = unique(a, a0, a1, b, b0, b1);
      if (anchors.length > 0) {
        let pa = a0, pb = b0;
        for (const [ia, ib] of anchors) {
          section(pa, ia, pb, ib);
          run("=", 1);
          pa = ia + 1; pb = ib + 1;
        }
        section(pa, a1, pb, b1);
      } else if ((a1 - a0) + (b1 - b0) <= myersLimit) {
        for (const [op, arg] of myers(a.slice(a0, a1), b.slice(b0, b1))) {
          if (op === "+") run("+", 0, arg); else run(op, arg);
        }
      } else {
        run("-", a1 - a0);
        run("+", 0, b.slice(b0, b1));
      }
    }
    run("=", end);
  };
  section(0, a.length, 0, b.length);
  return out;
}

// unique is the lines that occur once in a[a0:a1] and once in b[b0:b1], as
// pairs of where they are in each, the longest list of them in order in
// both.
function unique(a, a0, a1, b, b0, b1) {
  const seen = new Map();
  for (let i = a0; i < a1; i++) {
    const s = seen.get(a[i]);
    if (s) s.na++; else seen.set(a[i], { na: 1, nb: 0, ia: i, ib: -1 });
  }
  for (let j = b0; j < b1; j++) {
    const s = seen.get(b[j]);
    if (s) { s.nb++; s.ib = j; }
  }
  const pairs = [];
  for (let i = a0; i < a1; i++) {
    const s = seen.get(a[i]);
    if (s.ia === i && s.na === 1 && s.nb === 1) pairs.push([i, s.ib]);
  }
  // The longest increasing run of the b positions, by patience sorting.
  const tops = [], back = new Int32Array(pairs.length);
  for (let p = 0; p < pairs.length; p++) {
    let lo = 0, hi = tops.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (pairs[tops[mid]][1] < pairs[p][1]) lo = mid + 1; else hi = mid;
    }
    back[p] = lo > 0 ? tops[lo - 1] : -1;
    tops[lo] = p;
  }
  const lis = [];
  for (let p = tops.length > 0 ? tops[tops.length - 1] : -1; p >= 0; p = back[p]) lis.push(pairs[p]);
  return lis.reverse();
}

// myers is Myers's edits from a to b.
function myers(a, b) {
  const n = a.length, m = b.length, max = n + m;
  const v = new Int32Array(2 * max + 2);
  const trace = [];
  let found = false;
  for (let d = 0; d <= max && !found; d++) {
    // Step d reads only diagonals -d..d of the step before: keep those,
    // not the whole array, so the trace is d squared, not d times n.
    trace.push(v.slice(max - d, max + d + 1));
    for (let k = -d; k <= d; k += 2) {
      let x = k === -d || (k !== d && v[max + k - 1] < v[max + k + 1]) ? v[max + k + 1] : v[max + k - 1] + 1;
      let y = x - k;
      while (x < n && y < m && a[x] === b[y]) { x++; y++; }
      v[max + k] = x;
      if (x >= n && y >= m) { found = true; break; }
    }
  }
  // Walk back through the trace for the path, then run-length it.
  const steps = [];
  let x = n, y = m;
  for (let d = trace.length - 1; d >= 0; d--) {
    // trace[d] is v before step d: diagonal j at index j + d.
    const vd = trace[d], k = x - y;
    const at = (j) => vd[j + d];
    const prevK = k === -d || (k !== d && at(k - 1) < at(k + 1)) ? k + 1 : k - 1;
    const px = d === 0 ? 0 : at(prevK), py = px - prevK;
    while (x > px && y > py) { steps.push(["=", a[x - 1]]); x--; y--; }
    if (d > 0) {
      if (x === px) { steps.push(["+", b[y - 1]]); y--; } else { steps.push(["-", a[x - 1]]); x--; }
    }
  }
  steps.reverse();
  const out = [];
  for (const [op, line] of steps) {
    const last = out[out.length - 1];
    if (op === "+") {
      if (last && last[0] === "+") last[1].push(line); else out.push(["+", [line]]);
    } else if (last && last[0] === op) last[1]++; else out.push([op, 1]);
  }
  return out;
}

// apply makes b from a and diff(a, b).
function apply(a, edits) {
  const out = [];
  let i = 0;
  for (const [op, arg] of edits) {
    if (op === "=") { for (let j = 0; j < arg; j++) out.push(a[i++]); }
    else if (op === "-") i += arg;
    else for (const l of arg) out.push(l);
  }
  if (i !== a.length) throw new Error(`the edits cover ${i} of ${a.length} lines`);
  return out;
}

module.exports = { diff, apply };
