import { useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { addDays, holdsToday, isoDate, mondayOf, monthTitle, parseDate, periodTitle, stepDate, type Unit } from "@/lib/format";
import { cn } from "@/lib/utils";

const HEAD = ["Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"];

/** DateControl: back, the period's name (a calendar to pick any day), on.
 *  The arrows step by the view's unit: a day, a week or a month. */
export function DateControl({ date, unit, onDate }: { date: string; unit: Unit; onDate: (d: string) => void }) {
  const [open, setOpen] = useState(false);
  const current = holdsToday(date, unit);
  const noun = unit === "day" ? "day" : unit === "week" ? "week" : "month";
  return (
    <div className="flex items-center gap-0.5">
      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => onDate(stepDate(date, unit, -1))} aria-label={`Previous ${noun} (←)`} title={`Previous ${noun} (←)`}>
        <ChevronLeft className="h-4 w-4" />
      </Button>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <button
            className="min-w-28 rounded-md px-2 py-1 text-center font-medium transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            title="Pick a date"
          >
            {periodTitle(date, unit)}
          </button>
        </PopoverTrigger>
        <PopoverContent align="center" className="w-auto p-3">
          <Calendar date={date} unit={unit} onPick={(d) => (onDate(d), setOpen(false))} />
        </PopoverContent>
      </Popover>
      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => onDate(stepDate(date, unit, 1))} disabled={current} aria-label={`Next ${noun} (→)`} title={`Next ${noun} (→)`}>
        <ChevronRight className="h-4 w-4" />
      </Button>
      {!current && (
        <Button variant="outline" size="sm" className="ml-1 h-7 px-2.5 text-xs" onClick={() => onDate(isoDate(new Date()))} title="Back to today (t)">
          {unit === "day" ? "Today" : unit === "week" ? "This week" : "This month"}
        </Button>
      )}
    </div>
  );
}

/** Calendar is one month of days to pick from; the days of the shown
 *  period are marked, future days can't be picked. */
function Calendar({ date, unit, onPick }: { date: string; unit: Unit; onPick: (d: string) => void }) {
  const sel = parseDate(date);
  const [month, setMonth] = useState(new Date(sel.getFullYear(), sel.getMonth(), 1));
  const today = isoDate(new Date());
  const start = mondayOf(month);
  const cells = Array.from({ length: 42 }, (_, i) => addDays(start, i));
  const inPeriod = (d: Date) =>
    unit === "day" ? isoDate(d) === date
      : unit === "week" ? isoDate(mondayOf(d)) === isoDate(mondayOf(sel))
        : d.getMonth() === sel.getMonth() && d.getFullYear() === sel.getFullYear();
  const now = new Date();
  const lastMonth = month.getFullYear() === now.getFullYear() && month.getMonth() === now.getMonth();
  return (
    <div className="w-64">
      <div className="mb-2 flex items-center justify-between">
        <Button variant="ghost" size="icon" className="h-7 w-7" aria-label="Previous month" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))}>
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <span className="font-medium">{monthTitle(month)}</span>
        <Button variant="ghost" size="icon" className="h-7 w-7" aria-label="Next month" disabled={lastMonth} onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))}>
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
      <div className="grid grid-cols-7 gap-0.5 text-center">
        {HEAD.map((h) => <span key={h} className="pb-1 text-[11px] text-faint">{h}</span>)}
        {cells.map((d) => {
          const iso = isoDate(d);
          const future = iso > today;
          const other = d.getMonth() !== month.getMonth();
          return (
            <button
              key={iso}
              disabled={future}
              onClick={() => onPick(unit === "month" ? isoDate(new Date(d.getFullYear(), d.getMonth(), 1)) : iso)}
              className={cn(
                "num h-8 rounded-md text-xs transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-30",
                other && "text-faint",
                inPeriod(d) && "bg-secondary text-foreground",
                iso === date && "bg-foreground text-background hover:bg-foreground/90",
                iso === today && iso !== date && "ring-1 ring-inset ring-foreground/50",
              )}
              aria-label={iso}
              aria-current={iso === today ? "date" : undefined}
            >
              {d.getDate()}
            </button>
          );
        })}
      </div>
    </div>
  );
}
