import { Skeleton } from "@/components/ui/skeleton";
import { Empty, Panel } from "@/components/Panel";
import { DayBars } from "@/components/charts/DayBars";
import { ScoreLine } from "@/components/charts/ScoreLine";
import { HeatGrid } from "@/components/charts/HeatGrid";
import { Donut, ramp } from "@/components/charts/Donut";
import { GoalRings } from "@/components/charts/GoalRings";
import { TimeTable } from "@/components/Tables";
import { useApi, type Hours, type Week as WeekT } from "@/lib/api";
import { dur, parseDate, shortDay, dayTitle, addDays } from "@/lib/format";
import { byKindRows, kindColor, kindLabel } from "@/lib/kinds";

export default function Week({ date, tick }: { date: string; tick: number }) {
  const { data: w, error, loading } = useApi<WeekT>(`/api/week?date=${date}`, tick);
  const hours = useApi<Hours>("/api/hours");
  if (error) return <Empty>Couldn't load this week: {error}</Empty>;
  if (loading && !w) return <Skeleton className="h-96 w-full" />;
  if (!w) return null;

  const mon = parseDate(w.monday);
  const days = w.days.map((d) => ({ label: shortDay(parseDate(d.date)), byKind: d.byKind, faint: d.future }));
  const scores = w.days.map((d) => ({ label: shortDay(parseDate(d.date)), score: d.future || d.trackedMS < 3_600_000 ? null : d.score }));
  const projects = w.projects.slice(0, 6);
  const colors = ramp(projects.length);
  const kinds = byKindRows(w.byKind).map((k) => ({ name: kindLabel(k.kind), ms: k.ms, color: kindColor(k.kind) }));

  return (
    <div className="space-y-5">
      <div className="flex items-baseline justify-between">
        <h1 className="text-xl font-semibold tracking-tight">Week of {dayTitle(mon)} – {dayTitle(addDays(mon, 6))}</h1>
        <span className="num text-muted-foreground">{dur(w.trackedMS)} tracked</span>
      </div>
      <div className="grid gap-5 lg:grid-cols-[1.4fr_1fr]">
        <Panel label="Hours per day">
          <DayBars days={days} />
        </Panel>
        <Panel label="Kinds of time">
          {kinds.length ? <Donut data={kinds} center={dur(kinds.reduce((s, k) => s + k.ms, 0))} sub="this week" /> : <Empty>Nothing yet.</Empty>}
        </Panel>
      </div>
      <div className="grid gap-5 lg:grid-cols-[1fr_1.4fr]">
        <Panel label="Focus score">
          <ScoreLine points={scores} />
        </Panel>
        <Panel label="When you focus" aside={hours.data ? `mean focus minutes, last ${hours.data.activeDays} active days` : undefined}>
          {hours.data ? hours.data.thin ? <Empty>tock needs a few more days to see a pattern.</Empty> : <HeatGrid grid={hours.data.grid} /> : <Skeleton className="h-40" />}
        </Panel>
      </div>
      <div className="grid gap-5 lg:grid-cols-3">
        <Panel label="Projects">
          {projects.length ? <Donut data={projects.map((p, i) => ({ name: p.name || "No project", ms: p.ms, color: colors[i] }))} center={String(w.projects.length)} sub="projects" /> : <Empty>No projects this week.</Empty>}
        </Panel>
        <Panel label="Goals">
          {w.goals.length ? <GoalRings goals={w.goals} /> : <Empty>No goals yet.</Empty>}
        </Panel>
        <Panel label="Distractions">
          <TimeTable head="Where" rows={w.distractions.map((d) => ({ name: d.name, ms: d.ms, kind: "distract" }))} limit={6} />
        </Panel>
      </div>
      <div className="grid gap-5 lg:grid-cols-2">
        <Panel label="Categories">
          <TimeTable head="Category" rows={w.categories.map((c) => ({ name: c.name, ms: c.ms, kind: c.kind }))} />
        </Panel>
        <Panel label="Apps">
          <TimeTable head="App" rows={w.apps} />
        </Panel>
      </div>
    </div>
  );
}
