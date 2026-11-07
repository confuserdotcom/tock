import type { Outage } from "./api";

/** onDay is the outages that fall in [from, to), clipped to it, and the
 *  time trackd did not record that day. Apps-only time (partial) is listed
 *  but not counted, and left out with withPartial false. */
export function onDay(outages: Outage[] | undefined, from: number, to: number, withPartial = true): { items: Outage[]; ms: number } {
  const items: Outage[] = [];
  let ms = 0;
  for (const o of outages ?? []) {
    const a = Math.max(o.from, from), b = Math.min(o.to, to);
    if (b <= a || (o.partial && !withPartial)) continue;
    items.push({ ...o, from: a, to: b });
    if (!o.partial) ms += b - a;
  }
  return { items, ms };
}
