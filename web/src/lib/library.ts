// Pure helpers for the Library view.
const DAY = 86_400_000;

/** seenText says when a name was last seen; 0 = not in the last 90 days. */
export function seenText(lastSeenMS: number, nowMS: number): string {
  if (!lastSeenMS) return "over 90 d";
  const d = Math.floor((nowMS - lastSeenMS) / DAY);
  return d < 1 ? "today" : `${d} d ago`;
}

/** isStale: not seen in the last 30 days. */
export function isStale(lastSeenMS: number, nowMS: number): boolean {
  return !lastSeenMS || nowMS - lastSeenMS > 30 * DAY;
}

/** rateText prints an hourly rate the way the invoice does: amount, then
 *  the currency when one is set. */
export function rateText(rate: number | null, currency: string): string {
  if (rate == null) return "";
  return currency ? `${rate} ${currency}/h` : `${rate}/h`;
}

/** matches: q (case folded) in the name or any extra text. */
export function matches(name: string, extra: string[], q: string): boolean {
  const s = q.trim().toLowerCase();
  return !s || [name, ...extra].some((t) => t.toLowerCase().includes(s));
}
