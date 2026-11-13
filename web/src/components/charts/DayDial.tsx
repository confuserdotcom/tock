import { useMemo, useState } from "react";
import type { Day } from "@/lib/api";
import { arcPath, arcs, hourAt, point } from "@/lib/dial";
import { dur, hhmm, parseDate } from "@/lib/format";
import { kindColor } from "@/lib/kinds";
import { useNow } from "@/lib/live";
import { SERIES } from "@/lib/kinds";

const SIZE = 248, C = SIZE / 2, R = 100, W = 16;

/** DayDial is the whole day as a 24-hour ring, midnight at the top: what
 *  you did in kind colours, your work hours outside it, a hand at now.
 *  Pointing at an hour shows its minutes; a click zooms the timeline there. */
export function DayDial({ d, onHour }: { d: Day; onHour: (h: number) => void }) {
  const now = useNow(d.today ? 30_000 : 0);
  const day0 = parseDate(d.date).getTime();
  const as = useMemo(() => arcs(d.timeline, day0), [d.timeline, day0]);
  const [hover, setHover] = useState<number | null>(null);
  const nowT = d.today ? Math.min(1, Math.max(0, (now - day0) / 86_400_000)) : 1;
  const work = d.work && !d.work.off ? { a: (d.work.from - day0) / 86_400_000, b: (d.work.to - day0) / 86_400_000 } : null;

  const hourMS = (h: number) => SERIES.reduce((s, k) => s + (d.hours[k]?.[h] ?? 0), 0);
  const pick = (e: React.MouseEvent<SVGSVGElement>) => {
    const box = e.currentTarget.getBoundingClientRect();
    const x = ((e.clientX - box.left) / box.width) * SIZE - C, y = ((e.clientY - box.top) / box.height) * SIZE - C;
    const r = Math.hypot(x, y);
    return r > R - W * 2.4 && r < R + W ? hourAt(x, y) : null;
  };
  const [hx, hy] = point(C, R - W / 2 - 1, nowT);

  return (
    <div className="relative mx-auto aspect-square w-full max-w-[248px]">
      <svg viewBox={`0 0 ${SIZE} ${SIZE}`} className="h-full w-full cursor-pointer select-none" role="img"
        aria-label={`The day as a clock: ${dur(d.trackedMS)} tracked. Click an hour to zoom the timeline.`}
        onPointerMove={(e) => setHover(pick(e))} onPointerLeave={() => setHover(null)}
        onClick={(e) => { const h = pick(e); if (h !== null) onHour(h); }}>
        <defs>
          <pattern id="dial-hatch" width="4" height="4" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
            <rect width="4" height="4" fill="hsl(var(--hatch-b))" /><rect width="2" height="4" fill="hsl(var(--hatch-a))" />
          </pattern>
        </defs>
        {/* the track: the hours gone a shade lighter than the hours to come */}
        <circle cx={C} cy={C} r={R} fill="none" stroke="hsl(var(--muted))" strokeWidth={W} />
        {nowT > 0.001 && <path d={arcPath(C, R, 0, Math.min(nowT, 0.9999))} fill="none" stroke="hsl(var(--secondary))" strokeWidth={W} />}
        {work && work.b > work.a && (
          <path d={arcPath(C, R + W / 2 + 6, Math.max(0, work.a), Math.min(0.9999, work.b))} fill="none" stroke="hsl(var(--faint))" strokeWidth={2} strokeLinecap="round" opacity={0.7}>
            <title>Work hours {hhmm(d.work!.from)}–{hhmm(d.work!.to)}</title>
          </path>
        )}
        {hover !== null && (
          <path d={arcPath(C, R, hover / 24, (hover + 1) / 24)} fill="none" stroke="hsl(var(--foreground))" strokeOpacity={0.12} strokeWidth={W + 8} />
        )}
        {as.map((a, i) => (
          <path key={i} d={arcPath(C, R, a.from, Math.max(a.to, a.from + 0.0015))} fill="none" strokeWidth={W}
            stroke={a.idle ? "url(#dial-hatch)" : kindColor(a.kind)} />
        ))}
        {Array.from({ length: 24 }, (_, h) => {
          const [x0, y0] = point(C, R - W / 2 - 3, h / 24), [x1, y1] = point(C, R - W / 2 - (h % 6 ? 6 : 9), h / 24);
          return <line key={h} x1={x0} y1={y0} x2={x1} y2={y1} stroke="hsl(var(--faint))" strokeWidth={h % 6 ? 1 : 1.5} opacity={h % 6 ? 0.5 : 1} />;
        })}
        {[0, 6, 12, 18].map((h) => {
          const [x, y] = point(C, R - W / 2 - 19, h / 24);
          return <text key={h} x={x} y={y} textAnchor="middle" dominantBaseline="central" fontSize={10} fill="hsl(var(--faint))" className="num">{String(h).padStart(2, "0")}</text>;
        })}
        {d.today && (
          <g>
            <line x1={hx} y1={hy} x2={point(C, R + W / 2 + 2, nowT)[0]} y2={point(C, R + W / 2 + 2, nowT)[1]} stroke="hsl(var(--foreground))" strokeWidth={2.5} strokeLinecap="round" />
            <circle cx={point(C, R + W / 2 + 2, nowT)[0]} cy={point(C, R + W / 2 + 2, nowT)[1]} r={3.5} fill="hsl(var(--foreground))" />
          </g>
        )}
      </svg>
      <div className="pointer-events-none absolute inset-0 grid place-items-center text-center">
        {hover !== null ? (
          <div>
            <div className="num text-[26px] font-semibold leading-none tracking-[-0.02em]">{String(hover).padStart(2, "0")}:00</div>
            <div className="num mt-1.5 text-xs text-muted-foreground">{dur(hourMS(hover))} active</div>
            <div className="mt-0.5 text-[11px] text-faint">click to zoom</div>
          </div>
        ) : (
          <div>
            <div className="num text-[30px] font-semibold leading-none tracking-[-0.03em]">
              {d.today ? hhmm(now) : parseDate(d.date).toLocaleDateString("en-GB", { weekday: "short" })}
            </div>
            <div className="num mt-1.5 text-xs text-muted-foreground">{dur(d.trackedMS)} tracked</div>
            <div className="num mt-0.5 text-[11px] text-k-deep">{dur(d.stats.focusMS)} focus</div>
          </div>
        )}
      </div>
    </div>
  );
}
