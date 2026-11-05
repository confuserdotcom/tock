import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { MonthDay } from "@/lib/api";
import { dur, parseDate, dayTitle } from "@/lib/format";

const HEAD = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/** CalendarHeat: the month as a calendar, each day shaded by deep time. */
export function CalendarHeat({ first, days }: { first: string; days: MonthDay[] }) {
  const start = parseDate(first);
  const lead = (start.getDay() + 6) % 7;
  const max = Math.max(1, ...days.map((d) => d.deepMS));
  const cells: (MonthDay | null)[] = [...Array(lead).fill(null), ...days];
  return (
    <div className="grid grid-cols-7 gap-1.5">
      {HEAD.map((h) => (
        <span key={h} className="pb-1 text-center text-[11px] text-faint">{h}</span>
      ))}
      {cells.map((d, i) =>
        d ? (
          <Tooltip key={d.date}>
            <TooltipTrigger asChild>
              <div
                className={d.score < 0 ? "hatch flex aspect-[1.6] items-start justify-end rounded-md p-1.5" : "flex aspect-[1.6] items-start justify-end rounded-md p-1.5"}
                style={d.score < 0 ? undefined : { background: `hsl(var(--k-deep) / ${0.12 + 0.88 * (d.deepMS / max)})` }}
              >
                <span className="num text-[11px] text-foreground/80">{parseDate(d.date).getDate()}</span>
              </div>
            </TooltipTrigger>
            <TooltipContent className="num text-xs">
              <div className="font-medium">{dayTitle(parseDate(d.date))}</div>
              {d.score < 0 ? "Not a workday" : `${dur(d.deepMS)} deep · score ${d.score} · ${dur(d.trackedMS)} active`}
            </TooltipContent>
          </Tooltip>
        ) : (
          <span key={`x${i}`} />
        ),
      )}
    </div>
  );
}
