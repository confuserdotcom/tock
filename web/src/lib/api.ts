// The dashboard API (internal/web). Every request carries the per-run token,
// which arrives in the URL fragment and is kept in sessionStorage.
import { useCallback, useEffect, useState } from "react";

const TOKEN_RE = /^#(?:t=)?([0-9a-f]{64})/;
let token = "";

/** readToken takes the token from the fragment (then clears it) or storage. */
export function readToken(): string {
  const m = location.hash.match(TOKEN_RE);
  try {
    if (m) sessionStorage.setItem("tock-token", m[1]);
    token = sessionStorage.getItem("tock-token") || "";
  } catch {
    /* storage blocked: the token lives in memory only */
  }
  if (!token && m) token = m[1];
  if (m) history.replaceState(null, "", location.pathname + location.search);
  return token;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function call<T>(path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { headers: { "X-Tock-Token": token }, cache: "no-store" };
  if (body !== undefined) {
    init.method = "POST";
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)["Content-Type"] = "application/json";
  }
  const resp = await fetch(path, init);
  const text = await resp.text();
  let data: unknown = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    /* not JSON */
  }
  if (!resp.ok) {
    const msg = (data as { error?: string } | null)?.error || text || resp.statusText;
    throw new ApiError(resp.status, msg);
  }
  return data as T;
}

export const get = <T,>(path: string) => call<T>(path);
export const post = <T,>(path: string, body: unknown) => call<T>(path, body);

/** useApi loads path and reloads it when path or the refresh tick change. */
export function useApi<T>(path: string | null, tick = 0) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [n, setN] = useState(0);
  const reload = useCallback(() => setN((x) => x + 1), []);
  useEffect(() => {
    if (!path) return;
    let live = true;
    setLoading(true);
    get<T>(path)
      .then((d) => live && (setData(d), setError(null)))
      .catch((e: Error) => live && setError(e.message))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [path, tick, n]);
  return { data, error, loading, reload };
}

// ---- types: mirror internal/web/api.go ----

export type NameMS = { name: string; ms: number };
export type ByKind = Record<string, number>;

export type Score = {
  value: number; depth: number; blocks: number; calm: number; stretch: number;
  penalty: number; switchesPerHour: number; targetMS: number;
};
export type Stats = {
  activeMS: number; deepMS: number; focusMS: number; longestMS: number;
  switches: number; distractMS: number; longestRunMS: number;
};
export type Cat = { name: string; kind: string; ms: number; projects: NameMS[] };
export type Row = {
  ids: number[]; start: number; end: number; key: string; app: string; text: string;
  category: string; kind: string; project: string; source: string; idle: boolean; open: boolean;
};
export type Goal = {
  name: string; text: string; atMost: boolean; week: boolean; ms: number; doneMS: number;
  state: string; days?: number; held?: number; streak: number; best: number;
};
export type Usual = {
  days: number; score: number; trackedMS: number; focusMS: number; longestMS: number;
  distractMS: number; switchesPerHour: number;
};
export type Block = {
  start: number; end: number; name: string; category: string; kind: string;
  activeMS: number; deepMS: number; distractMS: number; top: NameMS[]; summary: string;
};
export type Day = {
  date: string; today: boolean; trackedMS: number; idleMS: number; byKind: ByKind;
  score: Score; stats: Stats; categories: Cat[]; hourFirst: number; hourLast: number;
  hours: Record<string, number[]>; timeline: Row[]; goals: Goal[]; blocks: Block[];
  summary: string; usual: Usual | null; firstRun: boolean;
  work: { from: number; to: number; off: boolean; outsideMS: number; closed: boolean } | null;
};
export type Status = {
  recording: boolean; lastSeenMS: number; rulesError?: string; configError?: string;
  session: { goal: string; startMS: number; plannedMS: number } | null;
};
export type WeekDay = { date: string; trackedMS: number; byKind: ByKind; future: boolean; score: number; deepMS: number };
export type Week = {
  monday: string; days: WeekDay[]; trackedMS: number; byKind: ByKind; categories: Cat[];
  projects: NameMS[]; apps: NameMS[]; distractions: NameMS[]; goals: Goal[]; plan: string;
};
export type TopRow = { name: string; ms: number; kind: string; mostly: string };
export type Top = { from: string; to: string; totalMS: number; sections: Record<string, TopRow[]> };
export type MonthSide = { trackedMS: number; focusMS: number; workdays: number; avgScore: number; switchesPerHour: number };
export type MonthDay = { date: string; trackedMS: number; focusMS: number; deepMS: number; longestMS: number; score: number };
export type Month = {
  first: string; partial: boolean; days: MonthDay[]; cur: MonthSide; prev: MonthSide;
  categories: { name: string; kind: string; ms: number; prevMS: number }[];
};
export type Hours = { thin: boolean; activeDays: number; grid: number[][] };
export type Item = {
  key: string; app: string; ms: number; spans: number; example: string; private: boolean;
  suggestion?: { label: string; reason?: string; share?: number };
};
export type Inbox = { days: number; totalMS: number; items: Item[]; smallKeys: number; smallMS: number; firstRun: boolean };
export type Labels = { categories: { name: string; kind: string }[]; projects: string[]; rulesError?: string };
export type Propose = {
  spans: number; ms: number; label: string;
  candidates: { rule: string; matches: number; matchMS: number; thisKey: number; new: number; newMS: number; fixed: number; replaces: number }[];
};
export type Done = { spans: number; ms: number; label: string };
