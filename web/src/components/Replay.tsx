import { useEffect, useMemo, useRef, useState } from "react";
import { Pause, Play, RotateCcw, X } from "lucide-react";
import { Panel } from "@/components/Panel";
import { Button } from "@/components/ui/button";
import type { Row } from "@/lib/api";
import { dur, hhmm } from "@/lib/format";
import { kindColor, kindLabel } from "@/lib/kinds";
import { frame, ordered } from "@/lib/replay";
import { cn } from "@/lib/utils";

// The longest step one animation frame may take.
const MAX_FRAME_MS = 100;

// How much of the day plays in one real second.
const SPEEDS = [
  { label: "1m/s", x: 60 },
  { label: "5m/s", x: 300 },
  { label: "15m/s", x: 900 },
  { label: "1h/s", x: 3600 },
];

/** Replay plays a day back: a playhead runs over the day's rows while the
 *  app, title and running totals change under it. It only moves when you
 *  press play. */
export function Replay({ rows: raw, from, to, onClose }: { rows: Row[]; from: number; to: number; onClose: () => void }) {
  const rows = useMemo(() => ordered(raw), [raw]);
  const start = rows.length ? Math.max(from, rows[0].start) : from;
  const [at, setAt] = useState(start);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);
  const strip = useRef<HTMLDivElement>(null);
  const atRef = useRef(at);
  atRef.current = at;

  useEffect(() => {
    if (!playing) return;
    let last = performance.now();
    let id = requestAnimationFrame(function step(t) {
      // A hidden tab gets no frames: the first one back moves a frame, not
      // the time it was away.
      const next = atRef.current + Math.min(t - last, MAX_FRAME_MS) * SPEEDS[speed].x;
      last = t;
      if (next >= to) {
        setAt(to);
        setPlaying(false);
        return;
      }
      setAt(next);
      id = requestAnimationFrame(step);
    });
    return () => cancelAnimationFrame(id);
  }, [playing, speed, to]);

  const f = useMemo(() => frame(rows, at, 6), [rows, at]);
  const span = Math.max(to - from, 1);
  const pos = (t: number) => `${((Math.min(Math.max(t, from), to) - from) / span) * 100}%`;

  const scrub = (clientX: number) => {
    const r = strip.current?.getBoundingClientRect();
    if (!r) return;
    setAt(from + Math.min(Math.max((clientX - r.left) / r.width, 0), 1) * span);
  };
  const toggle = () => {
    if (!playing && at >= to) setAt(start);
    setPlaying((p) => !p);
  };
  const onKey = (e: React.KeyboardEvent) => {
    // Space on a focused button already clicks it.
    if (e.key === " " && (e.target as HTMLElement).closest("button")) return;
    if (e.key === " ") {
      e.preventDefault();
      toggle();
    } else if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      e.preventDefault();
      setAt((a) => Math.min(Math.max(a + (e.key === "ArrowRight" ? 1 : -1) * 15 * 60_000, from), to));
    }
  };

  const now = f.now;
  return (
    <div onKeyDown={onKey} aria-label="Replay of the day">
      <Panel
        label="Replay"
        aside={
          <span className="flex items-center gap-2">
            {SPEEDS.map((s, i) => (
              <button key={s.x} onClick={() => setSpeed(i)}
                title={`${s.label.replace("/s", "")} of the day a second`}
                className={cn("num whitespace-nowrap rounded-md px-1.5 py-0.5 text-xs", i === speed ? "bg-secondary text-foreground" : "text-muted-foreground hover:text-foreground")}>
                {s.label}
              </button>
            ))}
            <button className="text-muted-foreground hover:text-foreground" onClick={onClose} title="Close the replay"><X className="h-4 w-4" /></button>
          </span>
        }
      >
        <div className="grid gap-5 md:grid-cols-[180px_1fr]">
          <div>
            <div className="num text-[56px] font-semibold leading-none tracking-[-0.03em]">{hhmm(at)}</div>
            <div className="mt-3 flex gap-2">
              <Button size="sm" variant="outline" className="h-7 px-2" onClick={toggle} title="Play or pause (space)">
                {playing ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
              </Button>
              <Button size="sm" variant="outline" className="h-7 px-2" onClick={() => { setPlaying(false); setAt(start); }} title="From the start">
                <RotateCcw className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>

          <div className="min-w-0">
            <div className="flex min-h-[52px] items-start gap-3">
              <span className="mt-1.5 h-2.5 w-2.5 shrink-0 rounded-[2px]"
                style={now ? (now.idle ? undefined : { background: kindColor(now.kind) }) : { background: "transparent" }}>
                {now?.idle && <span className="hatch block h-full w-full rounded-[2px]" />}
              </span>
              <div className="min-w-0">
                <div className="truncate text-[20px] font-semibold leading-tight">{now ? (now.idle ? "Away" : now.app) : "Nothing recorded"}</div>
                <div className="truncate text-sm text-muted-foreground">
                  {now && !now.idle ? [now.category || kindLabel(now.kind), now.project, now.text].filter(Boolean).join(" · ") : " "}
                </div>
              </div>
            </div>

            <div ref={strip} role="slider" aria-label="Time in the replay" aria-valuemin={from} aria-valuemax={to}
              aria-valuenow={Math.round(at)} aria-valuetext={hhmm(at)} tabIndex={0}
              className="relative mt-3 h-6 outline-none focus-visible:ring-1 focus-visible:ring-ring cursor-ew-resize touch-none select-none rounded-[2px] bg-secondary/40"
              onPointerDown={(e) => { e.currentTarget.setPointerCapture(e.pointerId); setPlaying(false); scrub(e.clientX); }}
              onPointerMove={(e) => { if (e.buttons & 1) scrub(e.clientX); }}>
              {rows.map((r) => (
                <div key={r.ids[0] + ":" + r.start} className={cn("absolute inset-y-0", r.idle && "hatch")}
                  style={{ left: pos(r.start), width: `calc(${pos(r.end)} - ${pos(r.start)})`, background: r.idle ? undefined : kindColor(r.kind),
                    opacity: r.start <= at ? 1 : 0.25 }} />
              ))}
              <div className="pointer-events-none absolute -inset-y-1 w-px bg-foreground" style={{ left: pos(at) }} />
            </div>
            <div className="num mt-1 flex justify-between text-[11px] text-faint"><span>{hhmm(from)}</span><span>{hhmm(to)}</span></div>

            <dl className="num mt-4 grid grid-cols-3 gap-x-5 gap-y-2 text-sm sm:grid-cols-6">
              <Stat k="Worked" v={dur(f.workedMS)} />
              <Stat k="Deep" v={dur(f.deepMS)} />
              <Stat k="Distracting" v={dur(f.distractMS)} />
              <Stat k="Away" v={dur(f.awayMS)} />
              <Stat k="Switches" v={String(f.switches)} />
              <Stat k="Longest run" v={dur(f.longestMS)} />
            </dl>

            {f.trail.length > 0 && (
              <ol className="mt-4 space-y-1 text-sm">
                {f.trail.map((r, i) => (
                  <li key={r.ids[0] + ":" + r.start} className="flex items-center gap-2" style={{ opacity: 0.35 + (0.65 * (i + 1)) / f.trail.length }}>
                    <span className="num w-11 shrink-0 text-faint">{hhmm(r.start)}</span>
                    <span className="h-2 w-2 shrink-0 rounded-[2px]" style={{ background: kindColor(r.kind) }} />
                    <span className="truncate">{r.app}</span>
                    <span className="num ml-auto shrink-0 text-muted-foreground">{dur(r.end - r.start)}</span>
                  </li>
                ))}
              </ol>
            )}
          </div>
        </div>
        <p className="mt-3 text-xs text-faint">Space plays and pauses, ← → step a quarter hour, drag the strip to scrub.</p>
      </Panel>
    </div>
  );
}

function Stat({ k, v }: { k: string; v: string }) {
  return (
    <div>
      <dt className="lbl">{k}</dt>
      <dd className="font-medium">{v}</dd>
    </div>
  );
}
