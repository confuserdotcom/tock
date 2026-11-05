import { useMemo, useState } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { Change, Empty, Panel } from "@/components/Panel";
import { Donut } from "@/components/charts/Donut";
import { GoalRings } from "@/components/charts/GoalRings";
import { HourBars } from "@/components/charts/HourBars";
import { Timeline } from "@/components/charts/Timeline";
import { TimeTable } from "@/components/Tables";
import { LabelSheet, type Target } from "@/components/LabelSheet";
import { useApi, type Day } from "@/lib/api";
import { delta, dur, hhmm } from "@/lib/format";
import { byKindRows, kindColor, kindLabel } from "@/lib/kinds";

export default function Today({ date, tick }: { date: string; tick: number }) {
  const { data: d, error, loading, reload } = useApi<Day>(`/api/day?date=${date}`, tick);
  const [target, setTarget] = useState<Target | null>(null);

  const apps = useMemo(() => {
    if (!d) return [];
    const m = new Map<string, { ms: number; kind: string }>();
    for (const r of d.timeline) {
      if (r.idle) continue;
      const a = m.get(r.app) ?? { ms: 0, kind: r.kind };
      a.ms += r.end - r.start;
      m.set(r.app, a);
    }
    return [...m.entries()].map(([name, a]) => ({ name, ...a })).sort((a, b) => b.ms - a.ms);
  }, [d]);

  if (error) return <Empty>Couldn't load this day: {error}</Empty>;
  if (loading && !d) return <TodaySkeleton />;
  if (!d) return null;
  if (d.firstRun) return <Empty>Nothing recorded yet. Leave tock running for a few minutes and this page fills in.</Empty>;

  const u = d.usual;
  const kinds = byKindRows(d.byKind).map((k) => ({ name: kindLabel(k.kind), ms: k.ms, color: kindColor(k.kind) }));
  const sph = d.score.switchesPerHour;

  return (
    <div className="space-y-5">
      <Panel label="Timeline" aside={<span className="num">{dur(d.trackedMS)} tracked · {dur(d.idleMS)} away</span>}>
        {d.timeline.length ? (
          <Timeline rows={d.timeline} date={d.date} first={d.hourFirst} last={d.hourLast} today={d.today}
            onPick={(r) => setTarget({ title: r.app, sub: `${r.text ? r.text + " · " : ""}${hhmm(r.start)}–${hhmm(r.end)}`, ids: r.ids })} />
        ) : (
          <Empty>No activity on this day.</Empty>
        )}
      </Panel>

      <div className="grid gap-5 lg:grid-cols-[300px_1fr_1fr]">
        <Panel label="Focus score">
          <div className="num text-[56px] font-semibold leading-none tracking-[-0.03em]">{d.score.value}</div>
          <div className="mt-1.5 text-sm">
            {u ? (
              <>
                <Change v={d.score.value - u.score} text={`${d.score.value - u.score >= 0 ? "+" : "−"}${Math.abs(d.score.value - u.score)}`} />{" "}
                <span className="text-muted-foreground">vs your usual {new Date(d.date).toLocaleDateString("en-GB", { weekday: "long" })} ({u.score})</span>
              </>
            ) : (
              <span className="text-muted-foreground">No usual day to compare with yet</span>
            )}
          </div>
          <dl className="mt-5 grid grid-cols-[1fr_auto_auto] gap-x-3.5 gap-y-2">
            <Stat k="Focus time" v={dur(d.stats.focusMS)} c={u ? d.stats.focusMS - u.focusMS : null} />
            <Stat k="Longest block" v={dur(d.stats.longestMS)} c={u ? d.stats.longestMS - u.longestMS : null} />
            <Stat k="Distracting" v={dur(d.stats.distractMS)} c={u ? d.stats.distractMS - u.distractMS : null} lowerIsGood />
            <dt>Switches / hour</dt>
            <dd className="num text-right">{sph.toFixed(1)}</dd>
            <dd className="num text-right text-xs">{u ? <Change v={sph - u.switchesPerHour} text={(sph - u.switchesPerHour >= 0 ? "+" : "−") + Math.abs(sph - u.switchesPerHour).toFixed(1)} lowerIsGood /> : null}</dd>
          </dl>
        </Panel>

        <Panel label="Minutes per hour">
          <HourBars hours={d.hours} first={d.hourFirst} last={d.hourLast} />
        </Panel>

        <Panel label="Kinds of time">
          {kinds.length ? <Donut data={kinds} center={dur(kinds.reduce((s, k) => s + k.ms, 0))} sub="tracked" /> : <Empty>Nothing labelled yet.</Empty>}
        </Panel>
      </div>

      <div className="grid gap-5 lg:grid-cols-3">
        <Panel label="Goals" aside={d.goals.length ? `${d.goals.filter((g) => g.state !== "short" && g.state !== "over").length} of ${d.goals.length} on track` : undefined}>
          {d.goals.length ? <GoalRings goals={d.goals} /> : <Empty>No goals yet. Add one with tock goal add.</Empty>}
        </Panel>
        <Panel label="Categories">
          <TimeTable head="Category" rows={d.categories.map((c) => ({ name: c.name, ms: c.ms, kind: c.kind }))} />
        </Panel>
        <Panel label="Apps">
          <TimeTable head="App" rows={apps} />
        </Panel>
      </div>

      {d.blocks.length > 0 && (
        <Panel label="Work blocks">
          <ul className="divide-y">
            {d.blocks.map((b) => (
              <li key={b.start} className="flex items-center gap-4 py-2">
                <span className="num w-28 shrink-0 text-muted-foreground">{hhmm(b.start)}–{hhmm(b.end)}</span>
                <i className="h-2 w-2 shrink-0 rounded-sm" style={{ background: kindColor(b.kind) }} />
                <span className="min-w-0 flex-1 truncate">
                  {b.name}
                  <span className="ml-2 text-muted-foreground">{b.top.slice(0, 3).map((t) => t.name).join(", ")}</span>
                </span>
                <span className="num">{dur(b.activeMS)}</span>
                <span className="num w-20 text-right text-muted-foreground">{dur(b.deepMS)} deep</span>
              </li>
            ))}
          </ul>
        </Panel>
      )}
      <LabelSheet target={target} onClose={() => setTarget(null)} onDone={reload} />
    </div>
  );
}

function Stat({ k, v, c, lowerIsGood }: { k: string; v: string; c: number | null; lowerIsGood?: boolean }) {
  return (
    <>
      <dt>{k}</dt>
      <dd className="num text-right">{v}</dd>
      <dd className="num text-right text-xs">{c === null ? null : <Change v={c} text={delta(c)} lowerIsGood={lowerIsGood} />}</dd>
    </>
  );
}

function TodaySkeleton() {
  return (
    <div className="space-y-5">
      <Skeleton className="h-40 w-full" />
      <div className="grid gap-5 lg:grid-cols-[300px_1fr_1fr]">
        <Skeleton className="h-72" />
        <Skeleton className="h-72" />
        <Skeleton className="h-72" />
      </div>
    </div>
  );
}
