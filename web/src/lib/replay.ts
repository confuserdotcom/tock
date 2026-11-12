import type { Row } from "./api";

/** Frame is the day as it stood at one moment of a replay. */
export type Frame = {
  now: Row | null; // the row under the playhead, null in a gap
  // trail: the apps before it, oldest first: idle and rows under a minute
  // left out, an app's rows in a row joined into one.
  trail: Row[];
  workedMS: number; deepMS: number; distractMS: number; awayMS: number;
  switches: number; // changes of app between worked rows
  longestMS: number; // the longest run on one app so far
};

/** frame plays rows (ascending, as /api/day sends them) up to at, keeping a
 *  trail of the last n apps. */
export function frame(rows: Row[], at: number, n: number): Frame {
  const f: Frame = { now: null, trail: [], workedMS: 0, deepMS: 0, distractMS: 0, awayMS: 0, switches: 0, longestMS: 0 };
  let last: Row | null = null;
  let run = 0;
  const done: Row[] = [];
  for (const r of rows) {
    if (r.start > at) break;
    const ms = Math.min(r.end, at) - r.start;
    if (at < r.end) f.now = r;
    if (r.idle) {
      f.awayMS += ms;
      continue;
    }
    f.workedMS += ms;
    if (r.kind === "deep") f.deepMS += ms;
    if (r.kind === "distract") f.distractMS += ms;
    if (last && last.app !== r.app) {
      f.switches++;
      run = 0;
    }
    run += ms;
    f.longestMS = Math.max(f.longestMS, run);
    if (r.end <= at) done.push(r);
    last = r;
  }
  const joined: Row[] = [];
  for (const r of done) {
    if (r === f.now || r.end - r.start < 60_000) continue;
    const prev = joined[joined.length - 1];
    if (prev && prev.app === r.app) joined[joined.length - 1] = { ...prev, end: r.end };
    else joined.push(r);
  }
  f.trail = joined.slice(-n);
  return f;
}
