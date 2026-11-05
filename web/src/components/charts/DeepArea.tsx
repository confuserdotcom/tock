import { Area, CartesianGrid, ComposedChart, Line, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { dur } from "@/lib/format";

/** DeepArea: deep time per day with its 7-day mean. */
export function DeepArea({ points, height = 220 }: { points: { label: string; ms: number }[]; height?: number }) {
  const rows = points.map((p, i) => {
    const win = points.slice(Math.max(0, i - 6), i + 1);
    return { label: p.label, h: p.ms / 3_600_000, mean: win.reduce((s, x) => s + x.ms, 0) / win.length / 3_600_000 };
  });
  return (
    <ChartContainer config={{}} className="w-full" style={{ height }}>
      <ComposedChart data={rows} margin={{ top: 8, right: 8, left: -24, bottom: 0 }}>
        <defs>
          <linearGradient id="deepFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="hsl(var(--k-deep))" stopOpacity={0.45} />
            <stop offset="100%" stopColor="hsl(var(--k-deep))" stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="hsl(var(--border))" />
        <XAxis dataKey="label" tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} interval="preserveStartEnd" />
        <YAxis tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} tickFormatter={(v) => `${v}h`} allowDecimals={false} />
        <ChartTooltip
          cursor={{ stroke: "hsl(var(--border))" }}
          content={({ active, payload, label }) =>
            active && payload?.length ? (
              <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow">
                <div className="font-medium">{label}</div>
                <div className="num">{dur(Number(payload[0].value) * 3_600_000)} deep · 7-day mean {dur(Number(payload[1]?.value ?? 0) * 3_600_000)}</div>
              </div>
            ) : null
          }
        />
        <Area dataKey="h" type="monotone" stroke="hsl(var(--k-deep))" strokeWidth={1.75} fill="url(#deepFill)" isAnimationActive={false} />
        <Line dataKey="mean" type="monotone" stroke="hsl(var(--muted-foreground))" strokeDasharray="4 4" dot={false} strokeWidth={1.25} isAnimationActive={false} />
      </ComposedChart>
    </ChartContainer>
  );
}
