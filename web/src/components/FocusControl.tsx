import { useEffect, useRef, useState } from "react";
import { Play, Square } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { post } from "@/lib/api";
import { dur, hhmm } from "@/lib/format";
import { left, presetName, useLive, useNow } from "@/lib/live";
import { go } from "@/lib/route";
import { cn } from "@/lib/utils";

/** focusField puts the cursor in the session's goal field, as f does. */
export function focusField() {
  window.dispatchEvent(new Event("tock:focus-field"));
}

/** useSession is the running session and the two things you can do to it. */
function useSession() {
  const { status, reload } = useLive();
  const [busy, setBusy] = useState(false);
  const act = async (body: object, done: string) => {
    setBusy(true);
    try {
      await post("/api/session", body);
      toast(done);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
      reload();
    }
  };
  const presets = status?.presets?.length ? status.presets : [25, 50, 90];
  return {
    status,
    on: status?.focus !== false,
    sess: status?.focus === false ? null : status?.session ?? null,
    presets,
    busy,
    start: (minutes: number, goal = "") => act({ action: "start", minutes, goal }, `Focus for ${presetName(minutes)}${goal.trim() ? `: ${goal.trim()}` : ""}`),
    stop: () => act({ action: "stop" }, "Session ended"),
  };
}

/** Ring is a session's progress as a ring, elapsed in deep teal. */
function Ring({ frac, size, stroke, children }: { frac: number; size: number; stroke: number; children?: React.ReactNode }) {
  const r = (size - stroke) / 2, c = 2 * Math.PI * r;
  return (
    <div className="relative grid shrink-0 place-items-center" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="absolute inset-0 -rotate-90" aria-hidden>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="hsl(var(--secondary))" strokeWidth={stroke} />
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="hsl(var(--k-deep))" strokeWidth={stroke} strokeLinecap="round"
          strokeDasharray={`${c * Math.min(1, Math.max(0.002, frac))} ${c}`} className="transition-[stroke-dasharray] duration-700 ease-out" />
      </svg>
      {children}
    </div>
  );
}

/** FocusPanel is the Today page's control: start a session with what it is
 *  for and how long, or follow and end the running one. */
export function FocusPanel() {
  const { on, sess, presets, busy, start, stop } = useSession();
  const now = useNow(sess ? 1000 : 0);
  const [goal, setGoal] = useState("");
  const [len, setLen] = useState<number | null>(null);
  const field = useRef<HTMLInputElement>(null);
  const minutes = len ?? presets[0];

  useEffect(() => {
    const on = () => field.current?.focus();
    window.addEventListener("tock:focus-field", on);
    return () => window.removeEventListener("tock:focus-field", on);
  }, []);

  if (!on) {
    return (
      <div className="flex h-full flex-col justify-center gap-3 text-sm text-muted-foreground">
        <p>Focus is off, so tock doesn't run sessions, breaks or distraction notices.</p>
        <Button variant="outline" size="sm" className="self-start" onClick={() => go({ view: "settings" })}>Turn Focus on</Button>
      </div>
    );
  }

  if (sess) {
    const total = Math.max(1, sess.plannedMS - sess.startMS), gone = now - sess.startMS;
    const over = gone > total;
    return (
      <div className="flex h-full items-center gap-6" aria-live="polite">
        <Ring frac={gone / total} size={148} stroke={10}>
          <div className="text-center">
            <div className="num text-[30px] font-semibold leading-none tracking-[-0.03em]">{over ? "0:00" : left(total - gone)}</div>
            <div className="mt-1.5 text-[11px] text-muted-foreground">{over ? "time's up" : "left"}</div>
          </div>
        </Ring>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-xs text-k-deep">
            <span className="relative flex h-2 w-2"><span className="absolute inset-0 animate-ping rounded-full bg-k-deep opacity-60 motion-reduce:hidden" /><span className="relative h-2 w-2 rounded-full bg-k-deep" /></span>
            In a session
          </div>
          <p className="mt-2 line-clamp-2 text-[15px] font-medium leading-snug">{sess.goal || "Focus session"}</p>
          <p className="num mt-1 text-xs text-muted-foreground">{dur(Math.min(gone, total))} of {dur(total)} · {over ? "ended" : "ends"} {hhmm(sess.plannedMS)}</p>
          <Button variant="outline" size="sm" className="mt-4" onClick={stop} disabled={busy}>
            <Square className="!size-3 fill-current" /> End session
          </Button>
        </div>
      </div>
    );
  }

  return (
    <form className="flex h-full flex-col justify-center" onSubmit={(e) => { e.preventDefault(); if (!busy) start(minutes, goal).then(() => setGoal("")); }}>
      <label htmlFor="focus-goal" className="text-[17px] font-medium tracking-[-0.01em]">What are you working on next?</label>
      <p className="mt-1 text-xs text-muted-foreground">Name it, pick a length, and tock keeps the time while you work.</p>
      <input id="focus-goal" ref={field} value={goal} onChange={(e) => setGoal(e.target.value)} maxLength={120} autoComplete="off"
        placeholder="Write the intro, fix the sync bug…"
        className="mt-4 h-11 w-full rounded-md border border-input bg-background px-3.5 text-sm outline-none transition-colors placeholder:text-faint focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30" />
      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <div className="flex gap-1.5" role="radiogroup" aria-label="Session length">
          {presets.map((p) => (
            <button key={p} type="button" role="radio" aria-checked={p === minutes} onClick={() => setLen(p)}
              className={cn("num h-9 rounded-md border px-3 text-xs transition-[background-color,color,transform] hover:bg-accent active:scale-[0.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                p === minutes ? "border-foreground/40 bg-secondary text-foreground" : "text-muted-foreground")}>
              {presetName(p)}
            </button>
          ))}
        </div>
        <Button type="submit" disabled={busy} className="ml-auto h-9 transition-transform active:scale-[0.98]">
          <Play className="fill-current" /> Start {presetName(minutes)}
        </Button>
      </div>
      <p className="num mt-3 text-xs text-faint">Ends at {hhmm(Date.now() + minutes * 60_000)}. End it early any time from here or the notch.</p>
    </form>
  );
}

/** FocusDock is the sidebar's control: always one click from a session. */
export function FocusDock() {
  const { on, sess, presets, busy, start, stop } = useSession();
  const now = useNow(sess ? 1000 : 0);
  if (!on) return null;
  if (sess) {
    const total = Math.max(1, sess.plannedMS - sess.startMS), gone = now - sess.startMS;
    return (
      <div className="rounded-lg border bg-card p-3">
        <div className="flex items-center gap-3">
          <Ring frac={gone / total} size={40} stroke={4} />
          <div className="min-w-0 flex-1">
            <div className="num text-[17px] font-semibold leading-none">{left(total - gone)}</div>
            <div className="mt-1 truncate text-xs text-muted-foreground" title={sess.goal}>{sess.goal || "Focus session"}</div>
          </div>
          <button onClick={stop} disabled={busy} aria-label="End session" title="End session"
            className="grid h-8 w-8 place-items-center rounded-md text-muted-foreground transition-[background-color,color,transform] hover:bg-accent hover:text-foreground active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <Square className="h-3 w-3 fill-current" />
          </button>
        </div>
      </div>
    );
  }
  return (
    <div className="rounded-lg border bg-card p-3">
      <div className="text-xs text-muted-foreground">Start a focus session</div>
      <div className="mt-2 grid grid-cols-3 gap-1.5">
        {presets.slice(0, 3).map((p) => (
          <button key={p} onClick={() => start(p)} disabled={busy} title={`Focus for ${presetName(p)}`}
            className="num h-8 rounded-md bg-secondary text-xs font-medium transition-[background-color,transform] hover:bg-accent active:scale-[0.96] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
            {presetName(p)}
          </button>
        ))}
      </div>
    </div>
  );
}
