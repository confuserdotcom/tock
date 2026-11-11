import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { SERIES, kindColor, kindLabel } from "@/lib/kinds";
import { dur } from "@/lib/format";

export type DayPoint = { label: string; byKind: Record<string, number>; faint?: boolean };

/** DayBars: hours per day, stacked by kind. */
/** onDay gets the index of a clicked day. */
export function DayBars({ days, height = 240, onDay }: { days: DayPoint[]; height?: number; onDay?: (i: number) => void }) {
  const rows = days.map((d) => {
    const r: Record<string, number | string> = { label: d.label };
    for (const k of SERIES) r[k || "unlabelled"] = (d.byKind[k] ?? 0) / 3_600_000;
    return r;
  });
  return (
    <ChartContainer config={{}} className="w-full" style={{ height }}>
      <BarChart data={rows} margin={{ top: 4, right: 0, left: -24, bottom: 0 }} className={onDay ? "cursor-pointer" : undefined}
        onClick={(e) => e?.activeTooltipIndex !== undefined && onDay?.(e.activeTooltipIndex)}>
        <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="hsl(var(--border))" />
        <XAxis dataKey="label" tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} />
        <YAxis tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} tickFormatter={(v) => `${v}h`} allowDecimals={false} />
        <ChartTooltip
          cursor={{ fill: "hsl(var(--secondary))" }}
          content={({ active, payload, label }) =>
            active && payload?.length ? (
              <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow">
                <div className="mb-1 font-medium">{label}</div>
                {onDay && <div className="mb-1 text-faint">Click to open the day</div>}
                {payload.filter((p) => Number(p.value) > 0).map((p) => (
                  <div key={String(p.dataKey)} className="flex items-center gap-2">
                    <i className="h-2 w-2 rounded-sm" style={{ background: kindColor(p.dataKey === "unlabelled" ? "" : String(p.dataKey)) }} />
                    <span className="flex-1">{kindLabel(p.dataKey === "unlabelled" ? "" : String(p.dataKey))}</span>
                    <span className="num">{dur(Number(p.value) * 3_600_000)}</span>
                  </div>
                ))}
              </div>
            ) : null
          }
        />
        {SERIES.map((k, i) => (
          <Bar key={k || "unlabelled"} dataKey={k || "unlabelled"} stackId="a" fill={kindColor(k)} radius={i === SERIES.length - 1 ? [2, 2, 0, 0] : 0} isAnimationActive={false} />
        ))}
      </BarChart>
    </ChartContainer>
  );
}
