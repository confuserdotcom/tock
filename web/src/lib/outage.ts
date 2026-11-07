import type { Outage } from "./api";

/** onDay is the outages that fall in [from, to), clipped to it, and their
 *  total: the time trackd did not record that day. */
export function onDay(outages: Outage[] | undefined, from: number, to: number): { items: Outage[]; ms: number } {
  const items: Outage[] = [];
  let ms = 0;
  for (const o of outages ?? []) {
    const a = Math.max(o.from, from), b = Math.min(o.to, to);
    if (b <= a) continue;
    items.push({ ...o, from: a, to: b });
    ms += b - a;
  }
  return { items, ms };
}
