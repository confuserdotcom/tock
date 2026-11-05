import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { clock, pct } from "@/lib/format";
import { kindColor } from "@/lib/kinds";

export type TRow = { name: string; ms: number; kind?: string; sub?: string };

/** TimeTable: name left, h:mm and share right in tabular figures, totals. */
export function TimeTable({ rows, head, limit = 10, total }: { rows: TRow[]; head: string; limit?: number; total?: number }) {
  const sum = total ?? rows.reduce((s, r) => s + r.ms, 0);
  const shown = rows.slice(0, limit);
  const rest = rows.slice(limit).reduce((s, r) => s + r.ms, 0);
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="h-8 px-0 text-[11px] font-medium text-faint">{head}</TableHead>
          <TableHead className="h-8 w-16 px-0 text-right text-[11px] font-medium text-faint">Time</TableHead>
          <TableHead className="h-8 w-12 px-0 text-right text-[11px] font-medium text-faint">Share</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {shown.map((r) => (
          <TableRow key={r.name + (r.sub ?? "")}>
            <TableCell className="max-w-0 truncate px-0 py-1.5">
              {r.kind && <i className="mr-2 inline-block h-2 w-2 rounded-sm align-[1px]" style={{ background: kindColor(r.kind) }} />}
              {r.name || "Unlabelled"}
              {r.sub && <span className="ml-2 text-muted-foreground">{r.sub}</span>}
            </TableCell>
            <TableCell className="num px-0 py-1.5 text-right">{clock(r.ms)}</TableCell>
            <TableCell className="num px-0 py-1.5 text-right text-muted-foreground">{pct(r.ms, sum)}</TableCell>
          </TableRow>
        ))}
        {rest > 0 && (
          <TableRow>
            <TableCell className="px-0 py-1.5 text-muted-foreground">{rows.length - limit} more</TableCell>
            <TableCell className="num px-0 py-1.5 text-right text-muted-foreground">{clock(rest)}</TableCell>
            <TableCell className="num px-0 py-1.5 text-right text-muted-foreground">{pct(rest, sum)}</TableCell>
          </TableRow>
        )}
      </TableBody>
      <TableFooter className="bg-transparent">
        <TableRow className="hover:bg-transparent">
          <TableCell className="px-0 py-1.5 font-semibold">Total</TableCell>
          <TableCell className="num px-0 py-1.5 text-right font-semibold">{clock(sum)}</TableCell>
          <TableCell className="px-0 py-1.5" />
        </TableRow>
      </TableFooter>
    </Table>
  );
}
