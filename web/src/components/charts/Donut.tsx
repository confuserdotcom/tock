import { Cell, Pie, PieChart } from "recharts";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { dur, pct } from "@/lib/format";

export type Slice = { name: string; ms: number; color: string };

/** Donut is the circular share chart: total in the middle, legend beside. */
/** onSelect, when given, makes the legend rows and slices buttons; active
 *  is the selected name, and the others dim. */
export function Donut({ data, center, sub, onSelect, active }: { data: Slice[]; center: string; sub: string; onSelect?: (name: string) => void; active?: string | null }) {
  const total = data.reduce((s, d) => s + d.ms, 0);
  return (
    <div className="flex items-center gap-5">
      <div className="relative h-[168px] w-[168px] shrink-0">
        <ChartContainer config={{}} className="aspect-square h-full w-full">
          <PieChart>
            <ChartTooltip
              cursor={false}
              content={({ active, payload }) =>
                active && payload?.length ? (
                  <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow">
                    <div className="font-medium">{payload[0].name}</div>
                    <div className="num text-muted-foreground">
                      {dur(Number(payload[0].value))} · {pct(Number(payload[0].value), total)}
                    </div>
                  </div>
                ) : null
              }
            />
            <Pie data={data} dataKey="ms" nameKey="name" innerRadius="66%" outerRadius="100%" paddingAngle={data.length > 1 ? 1.5 : 0} strokeWidth={0} isAnimationActive={false}>
              {data.map((d) => (
                <Cell key={d.name} fill={d.color} opacity={active && active !== d.name ? 0.25 : 1} className={onSelect ? "cursor-pointer outline-none" : undefined} onClick={() => onSelect?.(d.name)} />
              ))}
            </Pie>
          </PieChart>
        </ChartContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span className="num text-xl font-semibold tracking-tight">{center}</span>
          <span className="text-[11px] text-muted-foreground">{sub}</span>
        </div>
      </div>
      <ul className="min-w-0 flex-1 space-y-1.5">
        {data.map((d) => {
          const inner = (
            <>
              <i className="h-2 w-2 shrink-0 rounded-sm" style={{ background: d.color }} />
              <span className="min-w-0 flex-1 truncate text-left">{d.name}</span>
              <span className="num whitespace-nowrap">{dur(d.ms)}</span>
              <span className="num w-11 whitespace-nowrap text-right text-muted-foreground">{pct(d.ms, total)}</span>
            </>
          );
          return (
            <li key={d.name} className={active && active !== d.name ? "opacity-50" : undefined}>
              {onSelect ? (
                <button type="button" onClick={() => onSelect(d.name)} aria-pressed={active === d.name}
                  className={`-mx-1.5 flex w-[calc(100%+0.75rem)] items-center gap-2 rounded-md px-1.5 py-0.5 hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${active === d.name ? "bg-accent" : ""}`}>
                  {inner}
                </button>
              ) : (
                <div className="flex items-center gap-2">{inner}</div>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** ramp gives n neutral-to-teal shades for non-kind slices (projects). */
export function ramp(n: number): string[] {
  const base = ["--k-deep", "--k-shallow", "--k-browse", "--k-neutral", "--k-distract"];
  return Array.from({ length: n }, (_, i) => `hsl(var(${base[i % base.length]}) / ${1 - Math.floor(i / base.length) * 0.35})`);
}
