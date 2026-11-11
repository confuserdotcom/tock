// Routing lives in the fragment once the token is gone: #week/2026-11-05,
// #library/goals, #library/rules.
import { useEffect, useState } from "react";
import { isoDate } from "./format";

/** VIEWS are the views in the nav, in order: 1–6 switch to them. */
export const VIEWS = ["today", "week", "trends", "top", "inbox", "library"] as const;
/** ALL_VIEWS adds the ones reached another way: Settings, from the gear. */
export const ALL_VIEWS = [...VIEWS, "settings"] as const;
export type View = (typeof ALL_VIEWS)[number];
/** DATED views show the date control; the arrows step by their unit. */
export const DATED: Partial<Record<View, "day" | "week" | "month">> = { today: "day", week: "week", trends: "month" };
export const LIB_SECTIONS = ["projects", "categories", "goals", "rules"] as const;
export type LibSection = (typeof LIB_SECTIONS)[number];
export type Route = { view: View; date: string; sub: LibSection };

export function parse(): Route {
  const [v, d] = location.hash.replace(/^#/, "").split("/");
  const q = new URLSearchParams(location.search).get("v");
  const views = ALL_VIEWS as readonly string[];
  const view = views.includes(v) ? (v as View) : views.includes(q ?? "") ? (q as View) : "today";
  const date = DATED[view] && /^\d{4}-\d{2}-\d{2}$/.test(d ?? "") ? d : isoDate(new Date());
  const sub = view === "library" && (LIB_SECTIONS as readonly string[]).includes(d ?? "") ? (d as LibSection) : "projects";
  return { view, date, sub };
}

export function go(r: Partial<Route>) {
  const cur = parse();
  const next = { ...cur, ...r };
  if (next.view === "library") {
    location.hash = next.sub === "projects" ? "library" : `library/${next.sub}`;
    return;
  }
  const today = isoDate(new Date());
  location.hash = next.date === today || !DATED[next.view] ? next.view : `${next.view}/${next.date}`;
}

export function useRoute(): Route {
  const [r, setR] = useState(parse);
  useEffect(() => {
    const on = () => setR(parse());
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  return r;
}
