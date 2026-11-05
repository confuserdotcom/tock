import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

const DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/** HeatGrid: weekday x hour, cell strength = mean focus minutes. */
export function HeatGrid({ grid, from = 7, to = 22 }: { grid: number[][]; from?: number; to?: number }) {
  let lo = from, hi = to;
  grid.forEach((row) => row.forEach((v, h) => v > 0 && ((lo = Math.min(lo, h)), (hi = Math.max(hi, h)))));
  const hours = Array.from({ length: hi - lo + 1 }, (_, i) => lo + i);
  const max = Math.max(1, ...grid.flat());
  return (
    <div className="overflow-x-auto">
      <div className="inline-grid min-w-full gap-[3px]" style={{ gridTemplateColumns: `2.25rem repeat(${hours.length}, minmax(14px, 1fr))` }}>
        <span />
        {hours.map((h) => (
          <span key={h} className="num text-center text-[10px] text-faint">{h % 3 === 0 ? String(h).padStart(2, "0") : ""}</span>
        ))}
        {grid.map((row, d) => (
          <Row key={d} day={DAYS[d]} cells={hours.map((h) => row[h] ?? 0)} hours={hours} max={max} />
        ))}
      </div>
    </div>
  );
}

function Row({ day, cells, hours, max }: { day: string; cells: number[]; hours: number[]; max: number }) {
  return (
    <>
      <span className="text-[11px] leading-[18px] text-muted-foreground">{day}</span>
      {cells.map((v, i) => (
        <Tooltip key={i}>
          <TooltipTrigger asChild>
            <span
              className="h-[18px] rounded-[3px]"
              style={{ background: v ? `hsl(var(--k-deep) / ${0.15 + 0.85 * (v / max)})` : "hsl(var(--secondary) / 0.6)" }}
            />
          </TooltipTrigger>
          <TooltipContent className="num text-xs">
            {day} {String(hours[i]).padStart(2, "0")}:00 · {v} min focus
          </TooltipContent>
        </Tooltip>
      ))}
    </>
  );
}
