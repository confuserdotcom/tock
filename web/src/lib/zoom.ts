// The timeline's visible window: a part of the day's axis, zoomed and
// panned. Times are unix ms; bounds are the whole axis.

export type Win = { from: number; to: number };

const MIN = 60_000;
/** MIN_SPAN is the closest the timeline zooms in: a quarter of an hour. */
export const MIN_SPAN = 15 * MIN;

/** clampWin keeps w inside bounds and at least MIN_SPAN wide (or the whole
 *  axis when that is shorter), moving it rather than cutting it. */
export function clampWin(w: Win, b: Win): Win {
  const full = b.to - b.from;
  const span = Math.min(full, Math.max(MIN_SPAN, w.to - w.from));
  let from = w.from;
  if (w.to - w.from < span) from = (w.from + w.to) / 2 - span / 2;
  from = Math.min(Math.max(from, b.from), b.to - span);
  return { from: Math.round(from), to: Math.round(from + span) };
}

/** zoomAt scales w by factor (below 1 zooms in) around the point at
 *  fraction f of it, which stays where it is on screen. */
export function zoomAt(w: Win, f: number, factor: number, b: Win): Win {
  const t = w.from + f * (w.to - w.from);
  const span = (w.to - w.from) * factor;
  return clampWin({ from: t - f * span, to: t - f * span + span }, b);
}

/** pan moves w by df of its own width (negative is earlier). */
export function pan(w: Win, df: number, b: Win): Win {
  const d = df * (w.to - w.from);
  return clampWin({ from: w.from + d, to: w.to + d }, b);
}

/** fit is a window on [from, to] with a little room each side. */
export function fit(from: number, to: number, b: Win): Win {
  const pad = Math.max(2 * MIN, (to - from) * 0.08);
  return clampWin({ from: from - pad, to: to + pad }, b);
}

/** isFull is whether w shows the whole axis. */
export function isFull(w: Win, b: Win): boolean {
  return w.from <= b.from && w.to >= b.to;
}

const STEPS = [5, 10, 15, 30, 60, 120, 180].map((m) => m * MIN);

/** ticks are the axis marks for w: round times, about one per 1/12th of
 *  the width at most, and never closer than five minutes. */
export function ticks(w: Win, max = 12): number[] {
  const span = w.to - w.from;
  const step = STEPS.find((s) => span / s <= max) ?? STEPS[STEPS.length - 1];
  // Round to the step in local time, so hours land on the hour.
  const off = new Date(w.from).getTimezoneOffset() * MIN;
  const first = Math.ceil((w.from - off) / step) * step + off;
  const out: number[] = [];
  for (let t = first; t <= w.to; t += step) out.push(t);
  return out;
}
