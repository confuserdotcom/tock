import { useMemo, useRef, useState } from "react";
import type { Outage, Row } from "@/lib/api";
import { dur, hhmm, parseDate } from "@/lib/format";
import { kindColor, kindLabel } from "@/lib/kinds";
import { snapAt } from "@/lib/range";
import { onDay } from "@/lib/outage";

type Lane = { name: string; kind: string; ms: number; rows: Row[]; away?: boolean };

/** Timeline: one lane per category on a shared hour axis, Away hatched,
 *  a line for now, and last a "Not recording" lane, outlined, for the time
 *  trackd did not record (outages are not rows: nothing was recorded).
 *  A click on a bar hands its row to onPick; a drag across the lanes hands
 *  the range it covers (whole minutes, not past now) to onRange. */
export function Timeline({ rows, outages, date, first, last, today, onPick, onRange }: {
  rows: Row[]; outages?: Outage[]; date: string; first: number; last: number; today: boolean;
  onPick?: (r: Row) => void; onRange?: (from: number, to: number) => void;
}) {
  const day = parseDate(date).getTime();
  const from = day + first * 3_600_000;
  const to = day + (last + 1) * 3_600_000;
  const span = Math.max(1, to - from);
  const x = (t: number) => ((Math.min(Math.max(t, from), to) - from) / span) * 100;
  const [hover, setHover] = useState<{ r?: Row; o?: Outage; left: number; lane: number } | null>(null);
  const track = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x0: number; f0: number; moved: boolean } | null>(null);
  const [sel, setSel] = useState<{ a: number; b: number } | null>(null);
  const dragged = useRef(false);
  const frac = (clientX: number) => {
    const b = track.current?.getBoundingClientRect();
    return b && b.width > 0 ? (clientX - b.left) / b.width : 0;
  };
  const end = Math.min(to, Date.now());
  const pick = (f: number) => Math.min(snapAt(f, from, to), Math.max(from, Math.floor(end / 60_000) * 60_000));

  function down(e: React.PointerEvent) {
    if (!onRange || e.button !== 0) return;
    drag.current = { x0: e.clientX, f0: frac(e.clientX), moved: false };
  }
  function move(e: React.PointerEvent) {
    const d = drag.current;
    if (!d) return;
    if (!d.moved && Math.abs(e.clientX - d.x0) < 4) return;
    if (!d.moved) (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    d.moved = true;
    setHover(null);
    const f = frac(e.clientX);
    setSel({ a: Math.min(d.f0, f), b: Math.max(d.f0, f) });
  }
  function up(e: React.PointerEvent) {
    const d = drag.current;
    drag.current = null;
    if (!d?.moved) return;
    dragged.current = true;
    setTimeout(() => (dragged.current = false), 0);
    setSel(null);
    const f = frac(e.clientX);
    const a = pick(Math.min(d.f0, f)), b = pick(Math.max(d.f0, f));
    if (b > a) onRange?.(a, b);
  }
  function clickCapture(e: React.MouseEvent) {
    // The click that ends a drag is not a click on the bar under it.
    if (dragged.current) e.stopPropagation();
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

  const hours = [];
  for (let h = first; h <= last + 1; h++) hours.push(h);
  const now = Date.now();

  return (
    <div className="relative" onMouseLeave={() => setHover(null)}>
      <div className="ml-36 flex justify-between pb-1.5 text-[11px] text-faint">
        {hours.map((h) => (
          <span key={h} className="num w-0 text-center">{String(h % 24).padStart(2, "0")}</span>
        ))}
      </div>
      <div
        className={`relative space-y-1 ${onRange ? "cursor-crosshair select-none" : ""}`}
        onPointerDown={down} onPointerMove={move} onPointerUp={up}
        onPointerCancel={() => { drag.current = null; setSel(null); }}
        onClickCapture={clickCapture}
      >
        {lanes.map((l, i) => (
          <div key={l.name} className="flex items-center gap-3">
            <div className="flex shrink-0 items-center justify-between gap-2 pr-1" style={{ width: "8.25rem" }}>
              <span className="truncate">{l.name}</span>
              <span className="num text-xs text-muted-foreground">{dur(l.ms)}</span>
            </div>
            <div ref={i === 0 ? track : undefined} className="relative h-6 flex-1 rounded-sm bg-secondary/40">
              {l.rows.map((r) => {
                const left = x(r.start);
                const w = Math.max(0.15, x(r.end) - left);
                return (
                  <button
                    key={r.start + r.key}
                    className={l.away ? "hatch absolute inset-y-0 rounded-[2px] hover:brightness-125 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring" : "absolute inset-y-0 rounded-[2px] hover:brightness-125 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"}
                    style={{ left: `${left}%`, width: `${w}%`, background: l.away ? undefined : kindColor(r.kind) }}
                    onMouseEnter={() => setHover({ r, left: left + w / 2, lane: i })}
                    onFocus={() => setHover({ r, left: left + w / 2, lane: i })}
                    onClick={() => onPick?.(r)}
                    aria-label={`${r.app} ${hhmm(r.start)} to ${hhmm(r.end)}${r.invoice ? `, on invoice ${r.invoice}` : ""}`}
                  >
                    {r.invoice && <i className="pointer-events-none absolute inset-x-0 -bottom-[3px] h-px bg-foreground/70" />}
                  </button>
                );
              })}
              {today && now > from && now < to && <div className="absolute -inset-y-1 w-px bg-foreground" style={{ left: `${x(now)}%` }} />}
            </div>
          </div>
        ))}
        {gaps.items.length > 0 && (
          <div className="flex items-center gap-3">
            <div className="flex shrink-0 items-center justify-between gap-2 pr-1 text-muted-foreground" style={{ width: "8.25rem" }}>
              <span className="truncate">Not recording</span>
              <span className="num text-xs">{dur(gaps.ms)}</span>
            </div>
            <div ref={lanes.length === 0 ? track : undefined} className="relative h-6 flex-1 rounded-sm bg-secondary/40">
              {gaps.items.map((o) => {
                const left = x(o.from);
                const w = Math.max(0.15, x(o.to) - left);
                return (
                  <div
                    key={o.from}
                    tabIndex={0}
                    className={`absolute inset-y-0 rounded-[2px] border border-muted-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring ${o.partial ? "border-dotted" : "border-dashed"}`}
                    style={{ left: `${left}%`, width: `${w}%` }}
                    onMouseEnter={() => setHover({ o, left: left + w / 2, lane: lanes.length })}
                    onFocus={() => setHover({ o, left: left + w / 2, lane: lanes.length })}
                    aria-label={`${o.partial ? "Apps only" : "Not recording"} ${hhmm(o.from)} to ${hhmm(o.to)}: ${o.why}`}
                  />
                );
              })}
            </div>
          </div>
        )}
        {sel && (
          <div
            className="pointer-events-none absolute -inset-y-1 border-x border-foreground bg-foreground/10"
            style={{ left: `calc(9rem + (100% - 9rem) * ${Math.max(0, sel.a)})`, width: `calc((100% - 9rem) * ${Math.min(1, sel.b) - Math.max(0, sel.a)})` }}
          >
            <span className="num absolute -top-5 left-0 whitespace-nowrap text-[11px] text-muted-foreground">
              {hhmm(pick(sel.a))}–{hhmm(pick(sel.b))}
            </span>
          </div>
        )}
      </div>
      {hover && (
        <div
          className="pointer-events-none absolute z-10 w-64 -translate-x-1/2 rounded-md border bg-popover px-3 py-2 text-xs shadow-lg"
          style={{ left: `calc(9rem + (100% - 9rem) * ${hover.left / 100})`, top: `${1.75 + (hover.lane + 1) * 1.75}rem` }}
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
            </>
          )}
        </div>
      )}
    </div>
  );
}
