import { CartesianGrid, Line, LineChart, ReferenceLine, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";

/** ScoreLine: the score per day; days that are not workdays are gaps. */
export function ScoreLine({ points, height = 200, onPoint }: { points: { label: string; score: number | null }[]; height?: number; onPoint?: (i: number) => void }) {
  const real = points.filter((p) => p.score !== null).map((p) => p.score as number);
  const mean = real.length ? Math.round(real.reduce((a, b) => a + b, 0) / real.length) : null;
  return (
    <ChartContainer config={{}} className="w-full" style={{ height }}>
      <LineChart data={points} margin={{ top: 8, right: 8, left: -24, bottom: 0 }} className={onPoint ? "cursor-pointer" : undefined}
        onClick={(e) => e?.activeTooltipIndex !== undefined && onPoint?.(e.activeTooltipIndex)}>
        <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="hsl(var(--border))" />
        <XAxis dataKey="label" tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} interval="preserveStartEnd" />
        <YAxis domain={[0, 100]} ticks={[0, 25, 50, 75, 100]} tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} />
        {mean !== null && <ReferenceLine y={mean} stroke="hsl(var(--muted-foreground))" strokeDasharray="4 4" label={{ value: `mean ${mean}`, position: "insideTopRight", fill: "hsl(var(--muted-foreground))", fontSize: 11 }} />}
        <ChartTooltip
          cursor={{ stroke: "hsl(var(--border))" }}
          content={({ active, payload, label }) =>
            active && payload?.length && payload[0].value != null ? (
              <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow">
                {label} · score <span className="num font-medium">{payload[0].value}</span>
              </div>
            ) : null
          }
        />
        <Line dataKey="score" stroke="hsl(var(--foreground))" strokeWidth={1.75} dot={{ r: 2.5, fill: "hsl(var(--foreground))" }} connectNulls={false} isAnimationActive={false} />
      </LineChart>
    </ChartContainer>
  );
}
