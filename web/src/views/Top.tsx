import { useState } from "react";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, Panel } from "@/components/Panel";
import { useApi, type Top as TopT, type TopRow } from "@/lib/api";
import { clock, dur, pct } from "@/lib/format";
import { kindColor } from "@/lib/kinds";

const SECTIONS: [string, string][] = [["category", "Categories"], ["app", "Apps"], ["site", "Sites"], ["project", "Projects"]];

export default function Top({ tick }: { tick: number }) {
  const [range, setRange] = useState("week");
  const { data: t, error, loading } = useApi<TopT>(`/api/top?range=${range}`, tick);
  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-xl font-semibold tracking-tight">Top {t && <span className="num ml-2 text-base font-normal text-muted-foreground">{dur(t.totalMS)}</span>}</h1>
        <ToggleGroup type="single" value={range} onValueChange={(v) => v && setRange(v)} variant="outline" size="sm">
          <ToggleGroupItem value="today">Today</ToggleGroupItem>
          <ToggleGroupItem value="week">Week</ToggleGroupItem>
          <ToggleGroupItem value="month">Month</ToggleGroupItem>
        </ToggleGroup>
      </div>
      {error ? <Empty>Couldn't load: {error}</Empty> : loading && !t ? <Skeleton className="h-96" /> : t && (
        <div className="grid gap-5 lg:grid-cols-2">
          {SECTIONS.map(([key, label]) => (
            <Panel key={key} label={label}>
              {t.sections[key]?.length ? <BarList rows={t.sections[key]} total={t.totalMS} /> : <Empty>Nothing here.</Empty>}
            </Panel>
          ))}
        </div>
      )}
    </div>
  );
}

/** BarList: ranked names with a share bar under each. */
function BarList({ rows, total }: { rows: TopRow[]; total: number }) {
  const max = Math.max(1, ...rows.map((r) => r.ms));
  return (
    <ul className="space-y-2">
      {rows.slice(0, 12).map((r) => (
        <li key={r.name} className="relative">
          <div className="absolute inset-y-0 left-0 rounded-[3px] opacity-25" style={{ width: `${(r.ms / max) * 100}%`, background: kindColor(r.kind || "neutral") }} />
          <div className="relative flex items-center gap-3 px-2 py-1">
            <span className="min-w-0 flex-1 truncate">
              {r.name}
              {r.mostly && <span className="ml-2 text-xs text-muted-foreground">mostly {r.mostly}</span>}
            </span>
            <span className="num">{clock(r.ms)}</span>
            <span className="num w-10 text-right text-muted-foreground">{pct(r.ms, total)}</span>
          </div>
        </li>
      ))}
    </ul>
  );
}
