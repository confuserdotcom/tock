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

/** request sends path with the token, as a POST of body as JSON when there
 *  is one. A failed request throws an ApiError with the server's message. */
async function request(path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = { "X-Tock-Token": token };
  const init: RequestInit = { headers, cache: "no-store" };
  if (body !== undefined) {
    init.method = "POST";
    init.body = JSON.stringify(body);
    headers["Content-Type"] = "application/json";
  }
  const resp = await fetch(path, init);
  if (!resp.ok) {
    const text = await resp.text();
    let msg = text || resp.statusText;
    try {
      msg = (JSON.parse(text) as { error?: string } | null)?.error || msg;
    } catch {
      /* not JSON */
    }
    throw new ApiError(resp.status, msg);
  }
  return resp;
}

async function call<T>(path: string, body?: unknown): Promise<T> {
  const text = await (await request(path, body)).text();
  try {
    return (text ? JSON.parse(text) : null) as T;
  } catch {
    return null as T; // not JSON
  }
}

export const get = <T,>(path: string) => call<T>(path);

/** getRaw fetches path with the token and returns the response itself, for
 *  downloads; a failed request throws like call does. */
export const getRaw = (path: string) => request(path);
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
  invoice?: string; // the issued invoice that bills it
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
  billedMS?: number;
  date: string; today: boolean; trackedMS: number; idleMS: number; byKind: ByKind;
  score: Score; stats: Stats; categories: Cat[]; hourFirst: number; hourLast: number;
  hours: Record<string, number[]>; timeline: Row[]; goals: Goal[]; blocks: Block[];
  summary: string; usual: Usual | null; firstRun: boolean; outages: Outage[];
  work: { from: number; to: number; off: boolean; outsideMS: number; closed: boolean } | null;
};
export type Status = {
  recording: boolean; lastSeenMS: number; rulesError?: string; configError?: string; paused?: string;
  session: { goal: string; startMS: number; plannedMS: number } | null;
  focus?: boolean; billing?: boolean;
};
export type WeekDay = { billedMS?: number; date: string; trackedMS: number; byKind: ByKind; future: boolean; score: number; deepMS: number };
export type Week = {
  monday: string; days: WeekDay[]; trackedMS: number; byKind: ByKind; categories: Cat[];
  projects: NameMS[]; apps: NameMS[]; distractions: NameMS[]; goals: Goal[]; plan: string; outages: Outage[];
};
/** Outage: time trackd did not record, and why. Never part of the timeline or the totals.
 *  partial: apps were recorded without window titles (Accessibility off). */
export type Outage = { from: number; to: number; kind: string; why: string; partial: boolean };
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
export type Done = { spans: number; ms: number; label: string; note?: string };
export type LibCategory = { name: string; kind: string; afkMS: number; interrupt: boolean; projects: number; last30MS: number; lastSeenMS: number };
export type LibProject = { name: string; category: string; aliases: string[]; rate?: number | null; client?: string; archived: boolean; last30MS: number; lastSeenMS: number };
export type LibGoal = { category: string; project: string; name?: string; text?: string; atMost: boolean; week: boolean; ms: number };
export type LibDup = { kind: "category" | "project"; a: string; b: string };
export type Library = { categories: LibCategory[]; projects: LibProject[]; goals: LibGoal[]; lookalikes: LibDup[]; currency: string; billing?: boolean; clients?: string[]; nowMS: number };
export type LibChange = { what: string; undo: boolean };

export type Rule = {
  file: string; index: number; fingerprint: string; learned: boolean; bundle: string[];
  app: string; title: string; url: string; dir: string; category: string; project: string; kind: string;
  matchedMS: number; decidedMS: number; status: "" | "unused" | "shadowed";
  since?: string; // local "2026-11-20 14:05": the rule labels only time from then on
};
export type Rules = { rules: Rule[]; days: number };
export type RuleExample = { text: string; ms: number; from: string; to: string };
export type RulePreview = { matchedMS: number; changedMS: number; examples: RuleExample[] };
export type InvoicePreview = {
  project: string; number: string; seconds: number; total: number; currency: string; days: number;
  modelOnlyS: number; overlaps: string[];
};
export type InvoiceIssued = { number: string; total: number; currency: string };
export type Feature = {
  id: string; group: string; name: string; note: string; on: boolean; live: boolean;
  isGroup: boolean; needs?: string; waits: boolean;
};
export type WorkHours = { set: boolean; start: string; end: string; days: string[] };
export type InvoiceClient = { name: string; address: string[]; taxId: string };
export type InvoiceDetails = {
  name: string; address: string[]; currency: string; note: string; taxId: string;
  round: string; roundMode: string; minorUnits: number; taxLabel: string; taxRate: string; taxNote: string;
  clients: InvoiceClient[];
};
export type Settings = {
  features: Feature[]; hideApps: string[]; hideSites: string[]; configPath: string; configError?: string;
  hours: WorkHours; presets: number[]; maxSession: number; invoice: InvoiceDetails;
};
