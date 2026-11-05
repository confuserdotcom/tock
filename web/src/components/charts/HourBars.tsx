import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { SERIES, kindColor, kindLabel } from "@/lib/kinds";
import { dur } from "@/lib/format";

/** HourBars: minutes per hour, stacked by kind, from hourFirst to hourLast. */
export function HourBars({ hours, first, last, height = 220 }: { hours: Record<string, number[]>; first: number; last: number; height?: number }) {
  const rows = [];
  for (let h = first; h <= last; h++) {
    const r: Record<string, number | string> = { h: String(h).padStart(2, "0") };
    for (const k of SERIES) r[k || "unlabelled"] = Math.round((hours[k]?.[h] ?? 0) / 60_000);
    rows.push(r);
  }
  return (
    <ChartContainer config={{}} className="w-full" style={{ height }}>
      <BarChart data={rows} margin={{ top: 4, right: 0, left: -24, bottom: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="hsl(var(--border))" />
        <XAxis dataKey="h" tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} />
        <YAxis domain={[0, 60]} ticks={[0, 15, 30, 45, 60]} tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} />
        <ChartTooltip
          cursor={{ fill: "hsl(var(--secondary))" }}
          content={({ active, payload, label }) =>
            active && payload?.length ? (
              <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow">
                <div className="mb-1 font-medium">{label}:00</div>
                {payload.filter((p) => Number(p.value) > 0).map((p) => (
                  <div key={String(p.dataKey)} className="flex items-center gap-2">
                    <i className="h-2 w-2 rounded-sm" style={{ background: kindColor(p.dataKey === "unlabelled" ? "" : String(p.dataKey)) }} />
                    <span className="flex-1">{kindLabel(p.dataKey === "unlabelled" ? "" : String(p.dataKey))}</span>
                    <span className="num">{dur(Number(p.value) * 60_000)}</span>
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
