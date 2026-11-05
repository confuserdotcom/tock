// Durations and numbers as DESIGN.md writes them: 2h24 in cards, 2:24 in
// tables, a true minus sign for decreases.

const MIN = 60_000;

/** dur(8_640_000) = "2h24", dur(2_700_000) = "45m". */
export function dur(ms: number): string {
  const m = Math.round(Math.abs(ms) / MIN);
  if (m < 60) return `${m}m`;
  return `${Math.floor(m / 60)}h${String(m % 60).padStart(2, "0")}`;
}

/** clock(8_640_000) = "2:24". */
export function clock(ms: number): string {
  const m = Math.round(Math.abs(ms) / MIN);
  return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, "0")}`;
}

/** delta(2_880_000) = "+48m", delta(-1_380_000) = "−23m". */
export function delta(ms: number): string {
  const sign = ms < 0 ? "−" : "+";
  return sign + dur(ms);
}

/** pct(1, 4) = "25 %"; a zero whole is "0 %". */
export function pct(part: number, whole: number): string {
  if (whole <= 0) return "0 %";
  return `${Math.round((part / whole) * 100)} %`;
}

/** hhmm(ms) is the local wall-clock time, "09:05". */
export function hhmm(ms: number): string {
  const d = new Date(ms);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** isoDate(d) is the local date as YYYY-MM-DD. */
export function isoDate(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** parseDate("2026-11-05") is local midnight of that day. */
export function parseDate(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}

/** addDays returns a new date n days later (n may be negative). */
export function addDays(d: Date, n: number): Date {
  const out = new Date(d);
  out.setDate(out.getDate() + n);
  return out;
}

const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** dayTitle(2026-11-05) = "Thu 5 Nov". */
export function dayTitle(d: Date): string {
  return `${DAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]}`;
}

export function shortDay(d: Date): string {
  return DAYS[d.getDay()];
}

export function monthTitle(d: Date): string {
  return `${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}
