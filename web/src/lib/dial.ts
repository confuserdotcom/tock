// The day dial: the 24 hours of a day as a ring, midnight at the top.

/** Arc is a share of the day, 0..1 from midnight, in one kind. */
export type Arc = { from: number; to: number; kind: string; idle: boolean };

const DAY = 86_400_000;
const GAP = 120_000; // runs of a kind closer than this read as one
const SLIVER = 60_000; // arcs shorter than this are not drawn

/** arcs turns the timeline rows of the day starting at day0 into arcs:
 *  cut to the day, merged by kind across small gaps, slivers dropped. */
export function arcs(rows: { start: number; end: number; kind: string; idle: boolean }[], day0: number): Arc[] {
  const out: { from: number; to: number; kind: string; idle: boolean }[] = [];
  const sorted = [...rows].sort((a, b) => a.start - b.start);
  for (const r of sorted) {
    const from = Math.max(r.start, day0), to = Math.min(r.end, day0 + DAY);
    if (to <= from) continue;
    const last = out[out.length - 1];
    if (last && last.kind === r.kind && last.idle === r.idle && from - last.to <= GAP) last.to = Math.max(last.to, to);
    else out.push({ from, to, kind: r.kind, idle: r.idle });
  }
  return out.filter((a) => a.to - a.from >= SLIVER).map((a) => ({ ...a, from: (a.from - day0) / DAY, to: (a.to - day0) / DAY }));
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
