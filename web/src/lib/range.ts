// Time ranges for timeline edits: typed as HH:MM on the shown day, or
// dragged on the timeline and snapped to whole minutes.
import { addDays, hhmm, parseDate } from "./format";

const MIN = 60_000;

/** atClock(date, "09:05") is that wall-clock time on the day, or null if
 *  the text is not HH:MM. "24:00" is the end of the day. */
export function atClock(date: string, text: string): number | null {
  const m = text.trim().match(/^(\d{1,2}):(\d{2})$/);
  if (!m) return null;
  const h = Number(m[1]), mm = Number(m[2]);
  if (h > 24 || mm > 59 || (h === 24 && mm > 0)) return null;
  const d = parseDate(date);
  d.setHours(h, mm, 0, 0);
  return d.getTime();
}

/** snapAt is the time at fraction f of the axis [from, to], clamped to it
 *  and rounded to the minute. */
export function snapAt(f: number, from: number, to: number): number {
  const t = from + Math.min(Math.max(f, 0), 1) * (to - from);
  return Math.min(Math.max(Math.round(t / MIN) * MIN, from), to);
}

/** rangeError is "" for a range the server takes, else why not. */
export function rangeError(from: number | null, to: number | null, now: number): string {
  if (from === null || to === null) return "Times are HH:MM";
  if (to <= from) return "The range ends before it starts";
  if (to > now) return "The range reaches into the future";
  return "";
}

/** clockOf is t as HH:MM on the day, with the next midnight as "24:00" so
 *  atClock reads it back as the end of the day. */
export function clockOf(date: string, t: number): string {
  return t === addDays(parseDate(date), 1).getTime() ? "24:00" : hhmm(t);
}
