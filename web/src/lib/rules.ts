// Pure helpers for the Rules section of Library and the invoice dialog.
import type { Rule } from "./api";
import { addDays, isoDate, monthTitle, parseDate } from "./format";

export type Part = { field: string; value: string };

/** parts lists what a rule matches, in the order the sheet shows them. */
export function parts(r: Pick<Rule, "bundle" | "app" | "title" | "url" | "dir">): Part[] {
  const out: Part[] = [];
  if (r.bundle.length) out.push({ field: "Bundle", value: r.bundle.join(", ") });
  if (r.app) out.push({ field: "App", value: r.app });
  if (r.url) out.push({ field: "Site", value: r.url });
  if (r.title) out.push({ field: "Title", value: r.title });
  if (r.dir) out.push({ field: "Folder", value: r.dir });
  return out;
}

/** labelOf is what a rule files time as: "Code / tock", "Code", "/ tock". */
export function labelOf(r: Pick<Rule, "category" | "project">): string {
  if (!r.project) return r.category;
  return r.category ? `${r.category} / ${r.project}` : `/ ${r.project}`;
}

/** labelValue is labelOf as the sheet's field takes it: "Code/tock". */
export function labelValue(r: Pick<Rule, "category" | "project">): string {
  return r.project ? `${r.category}/${r.project}` : r.category;
}

/** statusText says why a rule decides nothing. */
export function statusText(s: Rule["status"]): string {
  return s === "unused" ? "never matched" : s === "shadowed" ? "always beaten" : "";
}

/** ruleText is everything a filter can find in a rule. */
export function ruleText(r: Rule): string[] {
  return [...parts(r).map((p) => p.value), r.category, r.project];
}

export type Period = { label: string; from: string; to: string };

/** periods are the ranges the invoice offers from the week of monday: the
 *  month before that week's month (first: what you bill), that month, and
 *  the week itself. */
export function periods(monday: string): Period[] {
  const m = parseDate(monday);
  const first = new Date(m.getFullYear(), m.getMonth(), 1);
  const prev = new Date(m.getFullYear(), m.getMonth() - 1, 1);
  const last = (d: Date) => new Date(d.getFullYear(), d.getMonth() + 1, 0);
  return [
    { label: monthTitle(prev), from: isoDate(prev), to: isoDate(last(prev)) },
    { label: monthTitle(first), from: isoDate(first), to: isoDate(last(first)) },
    { label: "This week", from: monday, to: isoDate(addDays(m, 6)) },
  ];
}
