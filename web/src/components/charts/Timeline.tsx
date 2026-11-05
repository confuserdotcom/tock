import { useMemo, useState } from "react";
import type { Row } from "@/lib/api";
import { dur, hhmm, parseDate } from "@/lib/format";
import { kindColor, kindLabel } from "@/lib/kinds";

type Lane = { name: string; kind: string; ms: number; rows: Row[]; away?: boolean };

/** Timeline: one lane per category on a shared hour axis, Away hatched,
 *  a line for now. A click on a bar hands its row to onPick. */
export function Timeline({ rows, date, first, last, today, onPick }: {
  rows: Row[]; date: string; first: number; last: number; today: boolean; onPick?: (r: Row) => void;
}) {
  const day = parseDate(date).getTime();
  const from = day + first * 3_600_000;
  const to = day + (last + 1) * 3_600_000;
  const span = Math.max(1, to - from);
  const x = (t: number) => ((Math.min(Math.max(t, from), to) - from) / span) * 100;
  const [hover, setHover] = useState<{ r: Row; left: number; lane: number } | null>(null);

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
      <div className="space-y-1">
        {lanes.map((l, i) => (
          <div key={l.name} className="flex items-center gap-3">
            <div className="flex shrink-0 items-center justify-between gap-2 pr-1" style={{ width: "8.25rem" }}>
              <span className="truncate">{l.name}</span>
              <span className="num text-xs text-muted-foreground">{dur(l.ms)}</span>
            </div>
            <div className="relative h-6 flex-1 rounded-sm bg-secondary/40">
              {l.rows.map((r) => {
                const left = x(r.start);
                const w = Math.max(0.15, x(r.end) - left);
                return (
                  <button
                    key={r.start + r.key}
                    className={l.away ? "hatch absolute inset-y-0 rounded-[2px]" : "absolute inset-y-0 rounded-[2px] hover:brightness-125 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"}
                    style={{ left: `${left}%`, width: `${w}%`, background: l.away ? undefined : kindColor(r.kind) }}
                    onMouseEnter={() => setHover({ r, left: left + w / 2, lane: i })}
                    onFocus={() => setHover({ r, left: left + w / 2, lane: i })}
                    onClick={() => !l.away && onPick?.(r)}
                    aria-label={`${r.app} ${hhmm(r.start)} to ${hhmm(r.end)}`}
                  />
                );
              })}
              {today && now > from && now < to && <div className="absolute -inset-y-1 w-px bg-foreground" style={{ left: `${x(now)}%` }} />}
            </div>
          </div>
        ))}
      </div>
      {hover && (
        <div
          className="pointer-events-none absolute z-10 w-64 -translate-x-1/2 rounded-md border bg-popover px-3 py-2 text-xs shadow-lg"
          style={{ left: `calc(9rem + (100% - 9rem) * ${hover.left / 100})`, top: `${1.75 + (hover.lane + 1) * 1.75}rem` }}
        >
          <div className="font-medium">{hover.r.app || "Away"}</div>
          {hover.r.text && <div className="truncate text-muted-foreground">{hover.r.text}</div>}
          <div className="num mt-1 flex justify-between text-muted-foreground">
            <span>{hhmm(hover.r.start)}–{hhmm(hover.r.end)} · {dur(hover.r.end - hover.r.start)}</span>
            {!hover.r.idle && <span style={{ color: kindColor(hover.r.kind) }}>{hover.r.category || kindLabel(hover.r.kind)}</span>}
          </div>
        </div>
      )}
    </div>
  );
}
