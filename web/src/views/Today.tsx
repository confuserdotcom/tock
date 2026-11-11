import { useMemo, useRef, useState } from "react";
import { X } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import { Change, Empty, Panel } from "@/components/Panel";
import { Donut } from "@/components/charts/Donut";
import { GoalRings } from "@/components/charts/GoalRings";
import { HourBars } from "@/components/charts/HourBars";
import { Timeline, type Highlight } from "@/components/charts/Timeline";
import { TimeTable } from "@/components/Tables";
import { LabelSheet, type Target } from "@/components/LabelSheet";
import { Button } from "@/components/ui/button";
import { useApi, type Day } from "@/lib/api";
import { addDays, delta, dur, hhmm, parseDate } from "@/lib/format";
import { go } from "@/lib/route";
import { fit, type Win } from "@/lib/zoom";
import { byKindRows, kindColor, kindLabel } from "@/lib/kinds";
import { onDay } from "@/lib/outage";
import { useGroups } from "@/lib/groups";
import { cn } from "@/lib/utils";

export default function Today({ date, tick }: { date: string; tick: number }) {
  const { data: d, error, loading, reload } = useApi<Day>(`/api/day?date=${date}`, tick);
  const [target, setTarget] = useState<Target | null>(null);
  const { focus } = useGroups();
  // The timeline's zoom and what it picks out, both cleared on another day.
  const [win, setWin] = useState<{ date: string; w: Win } | null>(null);
  const [hl, setHl] = useState<{ date: string; h: Highlight } | null>(null);
  const timelineRef = useRef<HTMLDivElement>(null);

  const apps = useMemo(() => {
    if (!d) return [];
    const m = new Map<string, { ms: number; kind: string }>();
    const day0 = parseDate(d.date).getTime(), day1 = addDays(parseDate(d.date), 1).getTime();
    for (const r of d.timeline) {
      if (r.idle) continue;
      const ms = Math.min(r.end, day1) - Math.max(r.start, day0); // a run from before midnight counts from midnight
      if (ms <= 0) continue;
      const a = m.get(r.app) ?? { ms: 0, kind: r.kind };
      a.ms += ms;
      m.set(r.app, a);
    }
    return [...m.entries()].map(([name, a]) => ({ name, ...a })).sort((a, b) => b.ms - a.ms);
  }, [d]);

  if (error) return <Empty>Couldn't load this day: {error}</Empty>;
  if (loading && !d) return <TodaySkeleton />;
  if (!d) return null;
  if (d.firstRun) return <Empty>Nothing recorded yet. Leave tock running for a few minutes and this page fills in.</Empty>;

  const u = d.usual;
  const now = Date.now();
  const day0 = parseDate(d.date).getTime();
  // A new entry starts as the half hour before now, or 09:00–10:00 on a past day.
  const addTime = () => {
    const to = d.today ? Math.floor(now / 60_000) * 60_000 : day0 + 10 * 3_600_000;
    const from = Math.max(day0, to - (d.today ? 1_800_000 : 3_600_000));
    setTarget({ title: "Add time", sub: "Time tock didn't see: a meeting, a call, paper.", range: { date: d.date, from, to, mode: "add" } });
  };
  const kindRows = byKindRows(d.byKind);
  const kinds = kindRows.map((k) => ({ name: kindLabel(k.kind), ms: k.ms, color: kindColor(k.kind) }));
  const zoom = win?.date === d.date ? win.w : null;
  const highlight = hl?.date === d.date ? hl.h : null;
  const bounds = { from: day0 + d.hourFirst * 3_600_000, to: day0 + (d.hourLast + 1) * 3_600_000 };
  // zoomTo points the timeline at a range and brings it into view.
  const zoomTo = (from: number, to: number) => {
    setWin({ date: d.date, w: fit(from, to, bounds) });
    timelineRef.current?.scrollIntoView({ block: "nearest", behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
  };
  const pickOut = (h: Highlight) => setHl((x) => (x?.date === d.date && x.h.label === h.label ? null : { date: d.date, h }));
  const sph = d.score.switchesPerHour;
  const notRec = onDay(d.outages, day0, day0 + 86_400_000).ms;

  return (
    <div className="space-y-5">
      <div ref={timelineRef} className="scroll-mt-4">
      <Panel
        label="Timeline"
        aside={
          <span className="flex items-center gap-3">
            {highlight && (
              <button className="flex items-center gap-1 rounded-md border bg-secondary px-1.5 py-0.5 text-foreground hover:bg-accent" onClick={() => setHl(null)} title="Show everything">
                Showing {highlight.label} <X className="h-3 w-3" />
              </button>
            )}
            <span className="num">{dur(d.trackedMS)} tracked · {dur(d.idleMS)} away{notRec > 0 && ` · ${dur(notRec)} not recorded`}</span>
            <Button variant="outline" size="sm" className="h-6 px-2 text-xs" onClick={addTime}>Add time</Button>
          </span>
        }
      >
        {d.timeline.length || notRec > 0 ? (
          <>
            <Timeline rows={d.timeline} outages={d.outages} date={d.date} first={d.hourFirst} last={d.hourLast} today={d.today}
              win={zoom} onWin={(w) => setWin(w ? { date: d.date, w } : null)} highlight={highlight}
              onPick={(r) => setTarget({
                title: r.idle ? "Away" : r.app,
                sub: r.idle ? "Label it to count it as worked." : `${r.text ? r.text + " · " : ""}${hhmm(r.start)}–${hhmm(r.end)}${r.invoice ? ` · on invoice ${r.invoice}` : ""}`,
                ids: r.idle ? undefined : r.ids,
                range: { date: d.date, from: Math.max(r.start, day0), to: Math.min(r.end, now, day0 + 86_400_000) },
              })}
              onRange={(from, to) => setTarget({ title: "Edit the timeline", range: { date: d.date, from, to } })} />
            <p className="mt-3 text-xs text-faint">
              Click a bar to label or delete it, drag across the lanes to pick a range. ⌘-scroll or pinch to zoom, or click a lane's name.
              {d.billedMS ? ` Underlined: ${dur(d.billedMS)} on issued invoices, which stay as issued.` : ""}
            </p>
          </>
        ) : (
          <Empty>No activity on this day.</Empty>
        )}
      </Panel>
      </div>

      <div className="grid gap-5 lg:grid-cols-[300px_1fr_1fr]">
        <Panel label={focus ? "Focus score" : "The day"} aside={focus ? <ScoreHelp d={d} /> : undefined}>
          {focus && (
          <>
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
          </>
          )}
          <dl className={cn("grid grid-cols-[1fr_auto_auto] gap-x-3.5 gap-y-2", focus && "mt-5")}>
            <Stat k="Focus time" v={dur(d.stats.focusMS)} c={u ? d.stats.focusMS - u.focusMS : null} />
            <Stat k="Longest block" v={dur(d.stats.longestMS)} c={u ? d.stats.longestMS - u.longestMS : null} />
            <Stat k="Distracting" v={dur(d.stats.distractMS)} c={u ? d.stats.distractMS - u.distractMS : null} lowerIsGood />
            <dt>Switches / hour</dt>
            <dd className="num text-right">{sph.toFixed(1)}</dd>
            <dd className="num text-right text-xs">{u ? <Change v={sph - u.switchesPerHour} text={(sph - u.switchesPerHour >= 0 ? "+" : "−") + Math.abs(sph - u.switchesPerHour).toFixed(1)} lowerIsGood /> : null}</dd>
          </dl>
        </Panel>

        <Panel label="Minutes per hour" aside="click an hour to zoom">
          <HourBars hours={d.hours} first={d.hourFirst} last={d.hourLast} onHour={(h) => zoomTo(day0 + h * 3_600_000, day0 + (h + 1) * 3_600_000)} />
        </Panel>

        <Panel label="Kinds of time">
          {kinds.length ? (
            <Donut data={kinds} center={dur(kinds.reduce((s, k) => s + k.ms, 0))} sub="tracked" active={highlight?.kind !== undefined ? highlight.label : null}
              onSelect={(name) => { const k = kindRows.find((x) => kindLabel(x.kind) === name); if (k) pickOut({ kind: k.kind, label: name }); }} />
          ) : <Empty>Nothing labelled yet.</Empty>}
        </Panel>
      </div>

      <div className="grid gap-5 lg:grid-cols-3">
        <Panel label="Goals" aside={
          <span className="flex items-center gap-3">
            {d.goals.length > 0 && `${d.goals.filter((g) => g.state !== "short" && g.state !== "over").length} of ${d.goals.length} on track`}
            <button className="underline-offset-2 hover:text-foreground hover:underline" onClick={() => go({ view: "library", sub: "goals" })}>Edit</button>
          </span>
        }>
          {d.goals.length ? <GoalRings goals={d.goals} /> : (
            <div className="py-6 text-center">
              <p className="text-sm text-muted-foreground">No goals yet: set a time to reach, or a limit, for a category or a project.</p>
              <Button variant="outline" size="sm" className="mt-3" onClick={() => go({ view: "library", sub: "goals" })}>Add a goal</Button>
            </div>
          )}
        </Panel>
        <Panel label="Categories">
          <TimeTable head="Category" rows={d.categories.map((c) => ({ name: c.name || "Unlabelled", ms: c.ms, kind: c.kind }))}
            rowHint="Show it on the timeline" active={highlight?.category ?? null}
            onRow={(r) => pickOut({ category: r.name, label: r.name })} />
        </Panel>
        <Panel label="Apps">
          <TimeTable head="App" rows={apps} rowHint="Show it on the timeline" active={highlight?.app ?? null}
            onRow={(r) => pickOut({ app: r.name, label: r.name })} />
        </Panel>
      </div>

      {d.blocks.length > 0 && (
        <Panel label="Work blocks" aside="click one to see it on the timeline">
          <ul className="divide-y">
            {d.blocks.map((b) => (
              <li key={b.start}>
                <button className="-mx-2 flex w-[calc(100%+1rem)] items-center gap-4 rounded-md px-2 py-2 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  onClick={() => zoomTo(b.start, b.end)}>
                <span className="num w-28 shrink-0 text-muted-foreground">{hhmm(b.start)}–{hhmm(b.end)}</span>
                <i className="h-2 w-2 shrink-0 rounded-sm" style={{ background: kindColor(b.kind) }} />
                <span className="min-w-0 flex-1 truncate">
                  {b.name}
                  <span className="ml-2 text-muted-foreground">{b.top.slice(0, 3).map((t) => t.name).join(", ")}</span>
                </span>
                <span className="num">{dur(b.activeMS)}</span>
                <span className="num w-20 text-right text-muted-foreground">{dur(b.deepMS)} deep</span>
                </button>
              </li>
            ))}
          </ul>
        </Panel>
      )}
      <LabelSheet target={target} onClose={() => setTarget(null)} onDone={reload} />
    </div>
  );
}

/** ScoreHelp opens what the score is made of, as tock explain score does. */
function ScoreHelp({ d }: { d: Day }) {
  const s = d.score;
  const part = (v: number) => `${Math.round(v * 100)} %`;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button className="underline-offset-2 hover:text-foreground hover:underline">How it's scored</button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 text-sm">
        <p className="mb-3 text-muted-foreground">Four parts, weighted in config.toml [score], less a little for distraction.</p>
        <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1.5">
          <dt>Deep time against your {dur(s.targetMS)} target</dt><dd className="num text-right">{part(s.depth)}</dd>
          <dt>Focus in blocks of 25m or more</dt><dd className="num text-right">{part(s.blocks)}</dd>
          <dt>Few app switches ({s.switchesPerHour.toFixed(1)} an hour)</dt><dd className="num text-right">{part(s.calm)}</dd>
          <dt>Longest run without a switch</dt><dd className="num text-right">{part(s.stretch)}</dd>
          <dt>Distraction</dt><dd className="num text-right">{s.penalty ? `−${s.penalty}` : "0"}</dd>
        </dl>
      </PopoverContent>
    </Popover>
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
