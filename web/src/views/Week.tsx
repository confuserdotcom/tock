import { useState } from "react";
import { toast } from "sonner";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Empty, Panel } from "@/components/Panel";
import { DayBars } from "@/components/charts/DayBars";
import { ScoreLine } from "@/components/charts/ScoreLine";
import { HeatGrid } from "@/components/charts/HeatGrid";
import { Donut, ramp } from "@/components/charts/Donut";
import { GoalRings } from "@/components/charts/GoalRings";
import { TimeTable } from "@/components/Tables";
import { InvoiceDialog } from "@/components/InvoiceDialog";
import { useApi, type Hours, type Week as WeekT } from "@/lib/api";
import { dur, parseDate, shortDay, dayTitle, addDays, isoDate } from "@/lib/format";
import { download } from "@/lib/download";
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
        <span className="flex items-center gap-3">
          <span className="num text-muted-foreground">{dur(w.trackedMS)} tracked</span>
          <Export monday={w.monday} />
        </span>
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

const EXPORTS = [
  { label: "Week report", sub: "One page with no scripts, to mail or keep", path: (m: string) => `/api/report?date=${m}`, ext: "html" },
  { label: "Entries as CSV", sub: "Each entry with its app, title and URL", q: "format=csv", ext: "csv" },
  { label: "Entries as JSON", sub: "The same, for scripts", q: "format=json", ext: "json" },
  { label: "Hours by project", sub: "Hours, rate and amount, as CSV", q: "by=project", ext: "csv" },
];

/** Export saves the week as tock report and tock export write it. */
function Export({ monday }: { monday: string }) {
  const [inv, setInv] = useState(false);
  const sun = isoDate(addDays(parseDate(monday), 6));
  async function save(e: (typeof EXPORTS)[number]) {
    const path = e.path ? e.path(monday) : `/api/export?from=${monday}&to=${sun}&${e.q}`;
    try {
      const r = await download(path, `tock-${monday}.${e.ext}`);
      if (r.error) toast.error(`Couldn't save ${r.name}: ${r.error}`);
      else if (r.browser) toast(`Downloading ${r.name}`);
      else if (r.saved) toast(`Saved ${r.name}`);
      // cancelled in the save sheet: nothing to say
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  return (
    <>
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-7 px-2.5 text-xs">Export</Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-1">
        {EXPORTS.map((e) => (
          <button key={e.label} onClick={() => save(e)} className="flex w-full flex-col items-start rounded-md px-2.5 py-2 text-left hover:bg-accent">
            <span className="text-sm">{e.label}</span>
            <span className="text-xs text-muted-foreground">{e.sub}</span>
          </button>
        ))}
        <button onClick={() => setInv(true)} className="flex w-full flex-col items-start rounded-md px-2.5 py-2 text-left hover:bg-accent">
          <span className="text-sm">Invoice…</span>
          <span className="text-xs text-muted-foreground">A PDF for one project at its rate</span>
        </button>
      </PopoverContent>
    </Popover>
    <InvoiceDialog open={inv} monday={monday} onClose={() => setInv(false)} />
    </>
  );
}
