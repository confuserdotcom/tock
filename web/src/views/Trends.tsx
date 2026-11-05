import { useState } from "react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Change, Empty, Panel } from "@/components/Panel";
import { CalendarHeat } from "@/components/charts/CalendarHeat";
import { DeepArea } from "@/components/charts/DeepArea";
import { ScoreLine } from "@/components/charts/ScoreLine";
import { TimeTable } from "@/components/Tables";
import { useApi, type Hours, type Month } from "@/lib/api";
import { delta, dur, monthTitle, parseDate } from "@/lib/format";

function lastMonths(n: number, from: Date): { value: string; label: string }[] {
  return Array.from({ length: n }, (_, i) => {
    const d = new Date(from.getFullYear(), from.getMonth() - i, 1);
    return { value: `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`, label: monthTitle(d) };
  });
}

export default function Trends({ date }: { date: string }) {
  const months = lastMonths(12, new Date());
  const [month, setMonth] = useState(date.slice(0, 7));
  const { data: m, error, loading } = useApi<Month>(`/api/month?month=${month}`);
  const hours = useApi<Hours>("/api/hours");

  const best = hours.data
    ? Array.from({ length: 24 }, (_, h) => ({ h: String(h).padStart(2, "0"), min: Math.round(hours.data!.grid.reduce((s, row) => s + row[h], 0) / 7) })).filter((_, h) => h >= 6 && h <= 23)
    : [];

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-xl font-semibold tracking-tight">Trends</h1>
        <Select value={month} onValueChange={setMonth}>
          <SelectTrigger className="w-40"><SelectValue /></SelectTrigger>
          <SelectContent>
            {months.map((o) => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}
          </SelectContent>
        </Select>
      </div>
      {error ? <Empty>Couldn't load this month: {error}</Empty> : loading && !m ? <Skeleton className="h-96" /> : m && <MonthBody m={m} />}
      <Panel label="Your best hours" aside="mean focus minutes per hour, across weekdays">
        {best.length ? (
          <ChartContainer config={{}} className="h-[180px] w-full">
            <BarChart data={best} margin={{ top: 4, right: 0, left: -24, bottom: 0 }}>
              <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="hsl(var(--border))" />
              <XAxis dataKey="h" tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} />
              <YAxis tickLine={false} axisLine={false} tick={{ fill: "hsl(var(--faint))", fontSize: 11 }} />
              <ChartTooltip cursor={{ fill: "hsl(var(--secondary))" }} content={({ active, payload, label }) => active && payload?.length ? <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow">{label}:00 · {payload[0].value} min</div> : null} />
              <Bar dataKey="min" fill="hsl(var(--k-deep))" radius={[2, 2, 0, 0]} isAnimationActive={false} />
            </BarChart>
          </ChartContainer>
        ) : (
          <Skeleton className="h-[180px]" />
        )}
      </Panel>
    </div>
  );
}

function MonthBody({ m }: { m: Month }) {
  const label = (s: string) => String(parseDate(s).getDate());
  const c = m.cur, p = m.prev;
  const hasPrev = p.trackedMS > 0;
  return (
    <>
      <div className="grid gap-px overflow-hidden rounded-lg border bg-border sm:grid-cols-2 lg:grid-cols-4">
        <Card hasPrev={hasPrev} k="Tracked" v={dur(c.trackedMS)} d={c.trackedMS - p.trackedMS} t={delta(c.trackedMS - p.trackedMS)} />
        <Card hasPrev={hasPrev} k="Focus" v={dur(c.focusMS)} d={c.focusMS - p.focusMS} t={delta(c.focusMS - p.focusMS)} />
        <Card hasPrev={hasPrev} k="Average score" v={c.workdays ? c.avgScore.toFixed(0) : "–"} d={c.avgScore - p.avgScore} t={(c.avgScore - p.avgScore >= 0 ? "+" : "−") + Math.abs(c.avgScore - p.avgScore).toFixed(0)} />
        <Card hasPrev={hasPrev} k="Switches / hour" v={c.switchesPerHour.toFixed(1)} d={c.switchesPerHour - p.switchesPerHour} t={(c.switchesPerHour - p.switchesPerHour >= 0 ? "+" : "−") + Math.abs(c.switchesPerHour - p.switchesPerHour).toFixed(1)} lowerIsGood />
      </div>
      <div className="grid items-start gap-5 lg:grid-cols-[1fr_1.3fr]">
        <Panel label="Deep time by day" aside={`${c.workdays} workdays`}>
          <CalendarHeat first={m.first} days={m.days} />
        </Panel>
        <div className="space-y-5">
          <Panel label="Deep time">
            <DeepArea points={m.days.map((d) => ({ label: label(d.date), ms: d.deepMS }))} />
          </Panel>
          <Panel label="Focus score">
            <ScoreLine points={m.days.map((d) => ({ label: label(d.date), score: d.score < 0 ? null : d.score }))} height={160} />
          </Panel>
        </div>
      </div>
      <Panel label="Categories against last month">
        <TimeTable head="Category" rows={m.categories.map((x) => ({ name: x.name, ms: x.ms, kind: x.kind, sub: !hasPrev ? undefined : x.prevMS ? `${delta(x.ms - x.prevMS)} vs last month` : "new" }))} limit={12} />
      </Panel>
    </>
  );
}

function Card({ k, v, d, t, lowerIsGood, hasPrev }: { k: string; v: string; d: number; t: string; lowerIsGood?: boolean; hasPrev: boolean }) {
  return (
    <div className="bg-card p-4">
      <div className="lbl">{k}</div>
      <div className="num mt-2 text-2xl font-semibold tracking-tight">{v}</div>
      <div className="mt-1 text-xs">
        {hasPrev ? (
          <>
            <Change v={Math.round(d * 10)} text={t} lowerIsGood={lowerIsGood} /> <span className="text-muted-foreground">vs last month</span>
          </>
        ) : (
          <span className="text-muted-foreground">Nothing recorded last month</span>
        )}
      </div>
    </div>
  );
}
