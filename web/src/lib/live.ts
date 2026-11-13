// Live state every view can read: tock's status (recording, the running
// session), a way to reload it after a change, and what waits in the inbox.
import { createContext, useContext, useEffect, useState } from "react";
import type { Status } from "./api";

export type Live = { status: Status | null; reload: () => void; inboxMS: number; inboxN: number };

export const LiveContext = createContext<Live>({ status: null, reload: () => {}, inboxMS: 0, inboxN: 0 });

export const useLive = () => useContext(LiveContext);

/** useNow is the time, refreshed every ms while the page is in view; off
 *  (ms 0) it stays put. */
export function useNow(ms: number): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!ms) return;
    const id = setInterval(() => document.visibilityState === "visible" && setNow(Date.now()), ms);
    return () => clearInterval(id);
  }, [ms]);
  return now;
}

/** left is a countdown, "24:05", or "1:02:05" past an hour. */
export function left(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = s % 60;
  const mm = String(m).padStart(h ? 2 : 1, "0"), ss = String(x).padStart(2, "0");
  return h ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

/** presetName is a session length as a chip says it: 25m, 1h30. */
export function presetName(min: number): string {
  return min < 60 ? `${min}m` : `${Math.floor(min / 60)}h${min % 60 ? String(min % 60).padStart(2, "0") : ""}`;
}
