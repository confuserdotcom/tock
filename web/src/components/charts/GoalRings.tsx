import { PolarAngleAxis, RadialBar, RadialBarChart } from "recharts";
import { ChartContainer } from "@/components/ui/chart";
import type { Goal } from "@/lib/api";
import { dur } from "@/lib/format";

/** GoalRings: one ring per goal, filled to done/target. "At most" goals
 *  turn red once they pass their limit. */
export function GoalRings({ goals }: { goals: Goal[] }) {
  const shown = goals.slice(0, 4);
  const color = (g: Goal) => (g.atMost ? (!g.days && g.doneMS > g.ms ? "hsl(var(--k-distract))" : "hsl(var(--k-shallow))") : "hsl(var(--k-deep))");
  // A week's goals carry days held of days so far; a day's carry time.
  const share = (g: Goal) => (g.days ? (g.held ?? 0) / g.days : g.doneMS / Math.max(1, g.ms));
  const data = shown.map((g) => ({ name: g.name, v: Math.min(100, share(g) * 100), fill: color(g) }));
  return (
    <div className="flex items-center gap-5">
      <ChartContainer config={{}} className="aspect-square h-[168px] w-[168px] shrink-0">
        <RadialBarChart data={data} innerRadius="30%" outerRadius="100%" startAngle={90} endAngle={-270} barSize={11}>
          <PolarAngleAxis type="number" domain={[0, 100]} tick={false} />
          <RadialBar dataKey="v" background={{ fill: "hsl(var(--secondary))" }} cornerRadius={6} isAnimationActive={false} />
        </RadialBarChart>
      </ChartContainer>
      <ul className="min-w-0 flex-1 space-y-2.5">
        {shown.map((g) => (
          <li key={g.name} className="min-w-0">
            <div className="flex items-center gap-2">
              <i className="h-2 w-2 shrink-0 rounded-full" style={{ background: color(g) }} />
              <span className="min-w-0 flex-1 truncate">{g.text || g.name}</span>
            </div>
            <div className="num pl-4 text-xs text-muted-foreground">
              {g.days ? `held ${g.held ?? 0} of ${g.days} days` : `${dur(g.doneMS)} of ${g.atMost ? "at most " : ""}${dur(g.ms)}`}
              {g.streak > 1 && ` · ${g.streak}-day streak`}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
