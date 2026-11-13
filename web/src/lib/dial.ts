// The day dial: the 24 hours of a day as a ring, midnight at the top.

/** Arc is a share of the day, 0..1 from midnight, in one kind. */
export type Arc = { from: number; to: number; kind: string; idle: boolean };

const DAY = 86_400_000;
const CELL = 300_000; // the dial reads the day in 5-minute cells

/** arcs turns the timeline rows of the day starting at day0 into arcs: each
 *  5-minute cell takes the kind it holds most of (none when less than a
 *  third of it is covered), and neighbouring cells of a kind join. A
 *  choppy day reads as its main runs, not as confetti. */
export function arcs(rows: { start: number; end: number; kind: string; idle: boolean }[], day0: number): Arc[] {
  const n = DAY / CELL;
  const cells: Map<string, number>[] = Array.from({ length: n }, () => new Map());
  for (const r of rows) {
    const from = Math.max(r.start, day0), to = Math.min(r.end, day0 + DAY);
    if (to <= from) continue;
    const key = r.idle ? "\u0000away" : r.kind;
    for (let i = Math.floor((from - day0) / CELL); i < n && day0 + i * CELL < to; i++) {
      const lo = day0 + i * CELL, hi = lo + CELL;
      const m = cells[i];
      m.set(key, (m.get(key) ?? 0) + Math.min(hi, to) - Math.max(lo, from));
    }
  }
  const out: Arc[] = [];
  cells.forEach((m, i) => {
    let best = "", bestMS = 0, total = 0;
    for (const [k, ms] of m) {
      total += ms;
      if (ms > bestMS) [best, bestMS] = [k, ms];
    }
    if (total < CELL / 3) return;
    const idle = best === "\u0000away", kind = idle ? "" : best;
    const last = out[out.length - 1];
    if (last && last.kind === kind && last.idle === idle && Math.abs(last.to - i / n) < 1e-9) last.to = (i + 1) / n;
    else out.push({ from: i / n, to: (i + 1) / n, kind, idle });
  });
  return out;
}

/** hourAt is the hour (0..23) under the point (x, y) from the dial's centre,
 *  y down, midnight at the top and the day running clockwise. */
export function hourAt(x: number, y: number): number {
  const turn = (Math.atan2(x, -y) / (2 * Math.PI) + 1) % 1;
  return Math.min(23, Math.floor(turn * 24));
}

/** point is where share t of the day sits on a circle of radius r around (c, c). */
export function point(c: number, r: number, t: number): [number, number] {
  const a = t * 2 * Math.PI;
  return [c + r * Math.sin(a), c - r * Math.cos(a)];
}

/** arcPath is an SVG path along the circle from share a to share b. */
export function arcPath(c: number, r: number, a: number, b: number): string {
  const [x0, y0] = point(c, r, a), [x1, y1] = point(c, r, b);
  const large = b - a > 0.5 ? 1 : 0;
  return `M${x0.toFixed(2)} ${y0.toFixed(2)}A${r} ${r} 0 ${large} 1 ${x1.toFixed(2)} ${y1.toFixed(2)}`;
}
