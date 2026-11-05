// Routing lives in the fragment once the token is gone: #week/2026-11-05.
import { useEffect, useState } from "react";
import { isoDate } from "./format";

export const VIEWS = ["today", "week", "trends", "top", "inbox"] as const;
export type View = (typeof VIEWS)[number];
export type Route = { view: View; date: string };

function parse(): Route {
  const [v, d] = location.hash.replace(/^#/, "").split("/");
  const q = new URLSearchParams(location.search).get("v");
  const view = (VIEWS as readonly string[]).includes(v) ? (v as View) : (VIEWS as readonly string[]).includes(q ?? "") ? (q as View) : "today";
  const date = /^\d{4}-\d{2}-\d{2}$/.test(d ?? "") ? d : isoDate(new Date());
  return { view, date };
}

export function go(r: Partial<Route>) {
  const cur = parse();
  const next = { ...cur, ...r };
  const today = isoDate(new Date());
  location.hash = next.date === today ? next.view : `${next.view}/${next.date}`;
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
