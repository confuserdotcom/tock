import { useEffect, useMemo, useRef, useState } from "react";
import { Minus, Plus, Maximize2 } from "lucide-react";
import type { Outage, Row } from "@/lib/api";
import { dur, hhmm, parseDate } from "@/lib/format";
import { kindColor, kindLabel } from "@/lib/kinds";
import { snapAt } from "@/lib/range";
import { onDay } from "@/lib/outage";
import { clampWin, fit, isFull, pan, ticks, zoomAt, type Win } from "@/lib/zoom";
import { cn } from "@/lib/utils";

type Lane = { name: string; kind: string; ms: number; rows: Row[]; away?: boolean };

/** Highlight dims every bar that is not of this kind, category or app. */
export type Highlight = { kind?: string; category?: string; app?: string; label: string };

export function matchesHighlight(r: Row, h: Highlight | null): boolean {
  if (!h) return true;
  if (h.kind !== undefined) return !r.idle && r.kind === h.kind;
  if (h.category !== undefined) return !r.idle && (r.category || "Unlabelled") === h.category;
  if (h.app !== undefined) return !r.idle && r.app === h.app;
  return true;
}

const LABEL_W = "9rem"; // lane names, and the left edge of every overlay

/** Timeline: one lane per category on a shared time axis, Away hatched,
 *  a line for now, and last a "Not recording" lane, outlined, for the time
 *  trackd did not record (outages are not rows: nothing was recorded).
 *
 *  It zooms: ⌘ or ctrl with the wheel (or a pinch) around the pointer, the
 *  + and − buttons and keys, a double click on the track, or a shift-drag
 *  across a range; a sideways scroll or the arrow keys pan, and the strip
 *  under it shows the whole day with the window on it to drag. win is the
 *  window, controlled by the caller so other charts can point into it.
 *
 *  A click on a bar hands its row to onPick; a drag across the lanes hands
 *  the range it covers (whole minutes, not past now) to onRange. */
export function Timeline({ rows, outages, date, first, last, today, onPick, onRange, win, onWin, highlight }: {
  rows: Row[]; outages?: Outage[]; date: string; first: number; last: number; today: boolean;
  onPick?: (r: Row) => void; onRange?: (from: number, to: number) => void;
  win: Win | null; onWin: (w: Win | null) => void; highlight?: Highlight | null;
}) {
  const day = parseDate(date).getTime();
  const bounds = useMemo(() => ({ from: day + first * 3_600_000, to: day + (last + 1) * 3_600_000 }), [day, first, last]);
  const w = win ? clampWin(win, bounds) : bounds;
  const full = isFull(w, bounds);
  const span = Math.max(1, w.to - w.from);
  const x = (t: number) => ((Math.min(Math.max(t, w.from), w.to) - w.from) / span) * 100;
  const setW = (n: Win) => onWin(isFull(n, bounds) ? null : n);

  const [hover, setHover] = useState<{ r?: Row; o?: Outage; left: number; lane: number } | null>(null);
  const track = useRef<HTMLDivElement>(null);
  const box = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x0: number; f0: number; moved: boolean; zoom: boolean } | null>(null);
  const [sel, setSel] = useState<{ a: number; b: number; zoom: boolean } | null>(null);
  const dragged = useRef(false);
  const frac = (clientX: number) => {
    const b = track.current?.getBoundingClientRect();
    return b && b.width > 0 ? (clientX - b.left) / b.width : 0;
  };
  const end = Math.min(w.to, Date.now());
  const pick = (f: number) => Math.min(snapAt(f, w.from, w.to), Math.max(w.from, Math.floor(end / 60_000) * 60_000));

  // The wheel: ⌘/ctrl (and a trackpad pinch, which arrives as ctrl) zooms
  // around the pointer; sideways, or shift, pans once zoomed in. A plain
  // vertical scroll is left to the page. Not a React handler: it must be
  // able to preventDefault, which a passive listener cannot.
  const latest = useRef({ w, bounds, setW });
  latest.current = { w, bounds, setW };
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const on = (e: WheelEvent) => {
      const { w, bounds, setW } = latest.current;
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault();
        const f = Math.min(1, Math.max(0, frac(e.clientX)));
        setW(zoomAt(w, f, Math.exp(e.deltaY * 0.01), bounds));
        return;
      }
      const sideways = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.shiftKey ? e.deltaY : 0;
      if (sideways && !isFull(w, bounds)) {
        e.preventDefault();
        const width = track.current?.getBoundingClientRect().width || 1;
        setW(pan(w, sideways / width, bounds));
      }
    };
    el.addEventListener("wheel", on, { passive: false });
    return () => el.removeEventListener("wheel", on);
  }, []);

  function down(e: React.PointerEvent) {
    if (e.button !== 0 || (!onRange && !e.shiftKey)) return;
    drag.current = { x0: e.clientX, f0: frac(e.clientX), moved: false, zoom: e.shiftKey };
  }
  function move(e: React.PointerEvent) {
    const d = drag.current;
    if (!d) return;
    if (!d.moved && Math.abs(e.clientX - d.x0) < 4) return;
    if (!d.moved) (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    d.moved = true;
    setHover(null);
    const f = frac(e.clientX);
    setSel({ a: Math.min(d.f0, f), b: Math.max(d.f0, f), zoom: d.zoom });
  }
  function up(e: React.PointerEvent) {
    const d = drag.current;
    drag.current = null;
    if (!d?.moved) return;
    dragged.current = true;
    setTimeout(() => (dragged.current = false), 0);
    setSel(null);
    const f = frac(e.clientX);
    if (d.zoom) {
      const a = w.from + Math.max(0, Math.min(d.f0, f)) * span, b = w.from + Math.min(1, Math.max(d.f0, f)) * span;
      setW(clampWin({ from: a, to: b }, bounds));
      return;
    }
    const a = pick(Math.min(d.f0, f)), b = pick(Math.max(d.f0, f));
    if (b > a) onRange?.(a, b);
  }
  function clickCapture(e: React.MouseEvent) {
    // The click that ends a drag is not a click on the bar under it.
    if (dragged.current) e.stopPropagation();
  }
  function dbl(e: React.MouseEvent) {
    if ((e.target as HTMLElement).closest("button")) return;
    setW(zoomAt(w, Math.min(1, Math.max(0, frac(e.clientX))), 0.5, bounds));
  }
  function key(e: React.KeyboardEvent) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === "+" || e.key === "=") setW(zoomAt(w, 0.5, 0.5, bounds));
    else if (e.key === "-") setW(zoomAt(w, 0.5, 2, bounds));
    else if (e.key === "0") onWin(null);
    else if (e.key === "ArrowLeft" && !full) setW(pan(w, -0.25, bounds));
    else if (e.key === "ArrowRight" && !full) setW(pan(w, 0.25, bounds));
    else return;
    e.preventDefault();
    e.stopPropagation(); // the arrows move between days only when not zoomed in
  }

  const lanes = useMemo(() => {
    // The lane totals count only this day: a run from before midnight starts at midnight.
    const m = new Map<string, Lane>();
    for (const r of rows) {
      const name = r.idle ? "Away" : r.category || "Unlabelled";
      const l = m.get(name) ?? { name, kind: r.idle ? "away" : r.kind, ms: 0, rows: [], away: r.idle };
      l.ms += Math.max(0, Math.min(r.end, day + 86_400_000) - Math.max(r.start, day));
      l.rows.push(r);
      m.set(name, l);
    }
    return [...m.values()].sort((a, b) => Number(!!a.away) - Number(!!b.away) || b.ms - a.ms);
  }, [rows, day]);

  const gaps = useMemo(() => onDay(outages, day, day + 86_400_000), [outages, day]);
  const marks = ticks(w);
  const now = Date.now();
  const inWin = (a: number, b: number) => b > w.from && a < w.to;
  const laneFit = (l: Lane) => {
    const a = Math.min(...l.rows.map((r) => r.start)), b = Math.max(...l.rows.map((r) => r.end));
    setW(fit(Math.max(a, bounds.from), Math.min(b, bounds.to), bounds));
  };

  return (
    <div ref={box} className="relative" onMouseLeave={() => setHover(null)}>
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="num text-xs text-muted-foreground" aria-live="polite">
          {full ? "Whole day" : `${hhmm(w.from)}–${hhmm(w.to)} · ${dur(w.to - w.from)}`}
        </div>
        <div className="flex items-center gap-1" role="group" aria-label="Zoom">
          <ZoomButton label="Zoom out (−)" disabled={full} onClick={() => setW(zoomAt(w, 0.5, 2, bounds))}><Minus /></ZoomButton>
          <ZoomButton label="Zoom in (+)" disabled={span <= 15 * 60_000} onClick={() => setW(zoomAt(w, 0.5, 0.5, bounds))}><Plus /></ZoomButton>
          <ZoomButton label="Show the whole day (0)" disabled={full} onClick={() => onWin(null)}><Maximize2 /></ZoomButton>
        </div>
      </div>
      <div className="relative ml-36 h-4 text-[11px] text-faint">
        {marks.map((t) => (
          <span key={t} className="num absolute -translate-x-1/2" style={{ left: `${x(t)}%` }}>{tickText(t)}</span>
        ))}
      </div>
      <div
        className={cn("relative space-y-1 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring", onRange ? "cursor-crosshair select-none" : "")}
        tabIndex={0}
        aria-label="Timeline. + and − zoom, 0 shows the whole day, arrows pan when zoomed in."
        onKeyDown={key}
        onPointerDown={down} onPointerMove={move} onPointerUp={up}
        onPointerCancel={() => { drag.current = null; setSel(null); }}
        onClickCapture={clickCapture}
        onDoubleClick={dbl}
      >
        {lanes.map((l, i) => (
          <div key={l.name} className="flex items-center gap-3">
            <button
              type="button"
              className="flex shrink-0 items-center justify-between gap-2 rounded-sm pr-1 text-left hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"
              style={{ width: "8.25rem" }}
              onClick={() => laneFit(l)}
              onDoubleClick={(e) => e.stopPropagation()}
              title={`Zoom to ${l.name}`}
            >
              <span className="truncate">{l.name}</span>
              <span className="num text-xs text-muted-foreground">{dur(l.ms)}</span>
            </button>
            <div ref={i === 0 ? track : undefined} className="relative h-6 flex-1 overflow-hidden rounded-sm bg-secondary/40">
              {marks.map((t) => <i key={t} className="pointer-events-none absolute inset-y-0 w-px bg-border/60" style={{ left: `${x(t)}%` }} />)}
              {l.rows.filter((r) => inWin(r.start, r.end)).map((r) => {
                const left = x(r.start);
                const wd = Math.max(0.15, x(r.end) - left);
                const dim = !matchesHighlight(r, highlight ?? null);
                return (
                  <button
                    key={r.start + r.key}
                    className={cn(
                      "absolute inset-y-0 rounded-[2px] transition-opacity hover:brightness-125 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring",
                      l.away && "hatch",
                      dim && "opacity-20",
                    )}
                    style={{ left: `${left}%`, width: `${wd}%`, background: l.away ? undefined : kindColor(r.kind) }}
                    onMouseEnter={() => setHover({ r, left: left + wd / 2, lane: i })}
                    onFocus={() => setHover({ r, left: left + wd / 2, lane: i })}
                    onBlur={() => setHover(null)}
                    onClick={() => onPick?.(r)}
                    aria-label={`${r.app || "Away"} ${hhmm(r.start)} to ${hhmm(r.end)}${r.invoice ? `, on invoice ${r.invoice}` : ""}`}
                  >
                    {r.invoice && <i className="pointer-events-none absolute inset-x-0 bottom-0 h-[2px] bg-foreground/80" />}
                    {wd > 9 && !l.away && (
                      <span className="pointer-events-none absolute inset-0 truncate px-1.5 text-left text-[11px] leading-6 text-background/90">{r.app}</span>
                    )}
                  </button>
                );
              })}
              {today && now > w.from && now < w.to && <div className="pointer-events-none absolute inset-y-0 w-px bg-foreground" style={{ left: `${x(now)}%` }} />}
            </div>
          </div>
        ))}
        {gaps.items.length > 0 && (
          <div className="flex items-center gap-3">
            <div className="flex shrink-0 items-center justify-between gap-2 pr-1 text-muted-foreground" style={{ width: "8.25rem" }}>
              <span className="truncate">Not recording</span>
              <span className="num text-xs">{dur(gaps.ms)}</span>
            </div>
            <div ref={lanes.length === 0 ? track : undefined} className="relative h-6 flex-1 overflow-hidden rounded-sm bg-secondary/40">
              {gaps.items.filter((o) => inWin(o.from, o.to)).map((o) => {
                const left = x(o.from);
                const wd = Math.max(0.15, x(o.to) - left);
                return (
                  <div
                    key={o.from}
                    tabIndex={0}
                    className={`absolute inset-y-0 rounded-[2px] border border-muted-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring ${o.partial ? "border-dotted" : "border-dashed"}`}
                    style={{ left: `${left}%`, width: `${wd}%` }}
                    onMouseEnter={() => setHover({ o, left: left + wd / 2, lane: lanes.length })}
                    onFocus={() => setHover({ o, left: left + wd / 2, lane: lanes.length })}
                    onBlur={() => setHover(null)}
                    aria-label={`${o.partial ? "Apps only" : "Not recording"} ${hhmm(o.from)} to ${hhmm(o.to)}: ${o.why}`}
                  />
                );
              })}
            </div>
          </div>
        )}
        {sel && (
          <div
            className={cn("pointer-events-none absolute -inset-y-1 border-x", sel.zoom ? "border-dashed border-foreground/70 bg-foreground/5" : "border-foreground bg-foreground/10")}
            style={{ left: `calc(${LABEL_W} + (100% - ${LABEL_W}) * ${Math.max(0, sel.a)})`, width: `calc((100% - ${LABEL_W}) * ${Math.min(1, sel.b) - Math.max(0, sel.a)})` }}
          >
            <span className="num absolute -top-5 left-0 whitespace-nowrap text-[11px] text-muted-foreground">
              {sel.zoom ? "Zoom to " : ""}{hhmm(pick(sel.a))}–{hhmm(pick(sel.b))}
            </span>
          </div>
        )}
      </div>
      <Overview rows={rows} bounds={bounds} w={w} full={full} setW={setW} />
      {hover && (
        <div
          className="pointer-events-none absolute z-10 w-64 -translate-x-1/2 rounded-md border bg-popover px-3 py-2 text-xs shadow-lg"
          style={{ left: `clamp(8rem, calc(${LABEL_W} + (100% - ${LABEL_W}) * ${hover.left / 100}), calc(100% - 8rem))`, top: `${3.75 + (hover.lane + 1) * 1.75}rem` }}
        >
          {hover.o ? (
            <>
              <div className="font-medium">{hover.o.partial ? "Apps only" : "Not recording"}</div>
              <div className="text-muted-foreground">{hover.o.why}</div>
              <div className="num mt-1 text-muted-foreground">{hhmm(hover.o.from)}–{hhmm(hover.o.to)} · {dur(hover.o.to - hover.o.from)}</div>
            </>
          ) : hover.r && (
            <>
              <div className="font-medium">{hover.r.app || "Away"}</div>
              {hover.r.text && <div className="truncate text-muted-foreground">{hover.r.text}</div>}
              <div className="num mt-1 flex justify-between text-muted-foreground">
                <span>{hhmm(hover.r.start)}–{hhmm(hover.r.end)} · {dur(hover.r.end - hover.r.start)}</span>
                {!hover.r.idle && <span style={{ color: kindColor(hover.r.kind) }}>{hover.r.category || kindLabel(hover.r.kind)}</span>}
              </div>
              {hover.r.invoice && <div className="mt-1 text-muted-foreground">On invoice {hover.r.invoice}</div>}
              <div className="mt-1 text-faint">Click to label or delete</div>
            </>
          )}
        </div>
      )}
    </div>
  );
}

function tickText(t: number): string {
  const d = new Date(t);
  return d.getMinutes() === 0 ? String(d.getHours()).padStart(2, "0") : hhmm(t);
}

function ZoomButton({ label, disabled, onClick, children }: { label: string; disabled?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="grid h-6 w-6 place-items-center rounded-md border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-3.5"
    >
      {children}
    </button>
  );
}

/** Overview is the whole day in one thin strip, with the zoomed window
 *  outlined in ink: drag the window to pan, or drag elsewhere to pick a new
 *  one; a click centres the window there. */
function Overview({ rows, bounds, w, full, setW }: { rows: Row[]; bounds: Win; w: Win; full: boolean; setW: (w: Win) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const st = useRef<{ mode: "move" | "brush"; f0: number; w0: Win; moved: boolean } | null>(null);
  const [brush, setBrush] = useState<{ a: number; b: number } | null>(null);
  const span = bounds.to - bounds.from;
  const x = (t: number) => ((Math.min(Math.max(t, bounds.from), bounds.to) - bounds.from) / span) * 100;
  const frac = (cx: number) => {
    const b = ref.current?.getBoundingClientRect();
    return b && b.width ? Math.min(1, Math.max(0, (cx - b.left) / b.width)) : 0;
  };
  const at = (f: number) => bounds.from + f * span;

  function down(e: React.PointerEvent) {
    if (e.button !== 0) return;
    const f = frac(e.clientX);
    const inside = !full && at(f) >= w.from && at(f) <= w.to;
    st.current = { mode: inside ? "move" : "brush", f0: f, w0: w, moved: false };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function move(e: React.PointerEvent) {
    const s = st.current;
    if (!s) return;
    const f = frac(e.clientX);
    if (!s.moved && Math.abs(f - s.f0) < 0.005) return;
    s.moved = true;
    if (s.mode === "move") {
      const d = (f - s.f0) * span;
      setW(clampWin({ from: s.w0.from + d, to: s.w0.to + d }, bounds));
    } else setBrush({ a: Math.min(s.f0, f), b: Math.max(s.f0, f) });
  }
  function up(e: React.PointerEvent) {
    const s = st.current;
    st.current = null;
    setBrush(null);
    if (!s) return;
    const f = frac(e.clientX);
    if (!s.moved) {
      // A click: centre the window there, or zoom to the hour around it.
      const width = full ? 3_600_000 : w.to - w.from;
      setW(clampWin({ from: at(f) - width / 2, to: at(f) + width / 2 }, bounds));
    } else if (s.mode === "brush") {
      setW(clampWin({ from: at(Math.min(s.f0, f)), to: at(Math.max(s.f0, f)) }, bounds));
    }
  }

  return (
    <div className="mt-3 flex items-center gap-3">
      <span className="w-[8.25rem] shrink-0 text-[11px] text-faint">{full ? "Drag to zoom" : "Drag to move"}</span>
      <div
        ref={ref}
        className="relative h-3 flex-1 cursor-pointer touch-none select-none rounded-sm bg-secondary/40"
        onPointerDown={down} onPointerMove={move} onPointerUp={up}
        onPointerCancel={() => { st.current = null; setBrush(null); }}
        role="slider"
        aria-label="Day overview: the zoomed window"
        aria-valuemin={bounds.from} aria-valuemax={bounds.to} aria-valuenow={w.from}
        aria-valuetext={full ? "whole day" : `${hhmm(w.from)} to ${hhmm(w.to)}`}
      >
        {rows.filter((r) => !r.idle).map((r) => (
          <i key={r.start + r.key} className="pointer-events-none absolute inset-y-[3px] rounded-[1px]" style={{ left: `${x(r.start)}%`, width: `${Math.max(0.1, x(r.end) - x(r.start))}%`, background: kindColor(r.kind) }} />
        ))}
        {!full && (
          <>
            <i className="pointer-events-none absolute inset-y-0 left-0 bg-background/60" style={{ width: `${x(w.from)}%` }} />
            <i className="pointer-events-none absolute inset-y-0 right-0 bg-background/60" style={{ width: `${100 - x(w.to)}%` }} />
            <i className="pointer-events-none absolute -inset-y-0.5 cursor-grab rounded-[2px] border border-foreground" style={{ left: `${x(w.from)}%`, width: `${x(w.to) - x(w.from)}%` }} />
          </>
        )}
        {brush && <i className="pointer-events-none absolute -inset-y-0.5 border-x border-foreground bg-foreground/10" style={{ left: `${brush.a * 100}%`, width: `${(brush.b - brush.a) * 100}%` }} />}
      </div>
    </div>
  );
}
