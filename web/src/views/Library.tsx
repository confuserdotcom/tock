import { useState } from "react";
import { toast } from "sonner";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, Panel } from "@/components/Panel";
import { LibrarySheet, type Edit } from "@/components/LibrarySheet";
import { post, useApi, type LibChange, type LibDup, type Library as Lib } from "@/lib/api";
import { clock } from "@/lib/format";
import { kindColor, kindLabel } from "@/lib/kinds";
import { goalTarget, isStale, matches, rateText, seenText } from "@/lib/library";
import { go, LIB_SECTIONS, type LibSection } from "@/lib/route";
import { cn } from "@/lib/utils";

const TITLE: Record<LibSection, string> = { projects: "Projects", categories: "Categories", goals: "Goals" };
const HIDDEN_KEY = "tock-lookalikes-hidden";

function hidden(): string[] {
  try { return JSON.parse(localStorage.getItem(HIDDEN_KEY) || "[]"); } catch { return []; }
}
function hide(key: string) {
  try { localStorage.setItem(HIDDEN_KEY, JSON.stringify([...hidden(), key])); } catch { /* blocked */ }
}

const head = "h-8 px-0 text-[11px] font-medium text-faint";
const cell = "px-0 py-1.5";

/** Library: the categories, projects and goals the rules declare, their
 *  recent use, and the edits and clean-up for them. */
export default function Library({ tick, sub }: { tick: number; sub: LibSection }) {
  const [n, setN] = useState(0);
  const { data: lib, error, loading } = useApi<Lib>("/api/library", tick + n);
  const reload = () => setN((x) => x + 1);
  const [q, setQ] = useState("");
  const [staleOnly, setStaleOnly] = useState(false);
  const [edit, setEdit] = useState<Edit | null>(null);
  const [merge, setMerge] = useState<LibDup | null>(null);
  const [gone, setGone] = useState(hidden);

  async function doMerge(d: LibDup) {
    try {
      const r = await post<LibChange>("/api/library/merge", { kind: d.kind, from: d.b, into: d.a });
      toast(r.what.charAt(0).toUpperCase() + r.what.slice(1));
      reload();
    } catch (e) {
      toast.error((e as Error).message);
    }
    setMerge(null);
  }

  const now = lib?.nowMS ?? Date.now();
  const kindOf = (c: string) => lib?.categories.find((x) => x.name === c)?.kind ?? "";
  const dups = (lib?.lookalikes ?? []).filter((d) => (sub === "projects" ? d.kind === "project" : sub === "categories" && d.kind === "category") && !gone.includes(`${d.kind}:${d.a}:${d.b}`));
  const addKind = sub === "projects" ? "project" : sub === "categories" ? "category" : "goal";

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h1 className="text-xl font-semibold tracking-tight">Library</h1>
        <ToggleGroup type="single" value={sub} onValueChange={(v) => v && go({ view: "library", sub: v as LibSection })} variant="outline" size="sm">
          {LIB_SECTIONS.map((s) => <ToggleGroupItem key={s} value={s}>{TITLE[s]}</ToggleGroupItem>)}
        </ToggleGroup>
      </div>
      {error ? <Empty>Couldn't load: {error}</Empty> : loading && !lib ? <Skeleton className="h-96" /> : lib && (
        <Panel
          label={TITLE[sub]}
          aside={
            <div className="flex items-center gap-3">
              {sub !== "goals" && (
                <>
                  <input className="h-7 w-44 rounded-md border bg-background px-2 text-xs" placeholder="Filter" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Filter" />
                  <label className="flex items-center gap-1.5"><input type="checkbox" checked={staleOnly} onChange={(e) => setStaleOnly(e.target.checked)} /> Stale only</label>
                </>
              )}
              <Button size="sm" variant="outline" className="h-7 px-2.5 text-xs" onClick={() => setEdit({ kind: addKind, item: null } as Edit)}>Add</Button>
            </div>
          }
        >
          {dups.map((d) => (
            <div key={`${d.a}:${d.b}`} className="mb-2 flex items-center gap-3 rounded-md border border-dashed px-3 py-2 text-sm">
              <span className="flex-1"><b className="font-medium">{d.b}</b> looks like <b className="font-medium">{d.a}</b></span>
              <Button size="sm" variant="outline" className="h-7 px-2.5 text-xs" onClick={() => setMerge(d)}>Merge into {d.a}</Button>
              <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={() => { const k = `${d.kind}:${d.a}:${d.b}`; hide(k); setGone((g) => [...g, k]); }}>Dismiss</Button>
            </div>
          ))}
          {sub === "projects" && <Projects lib={lib} now={now} q={q} staleOnly={staleOnly} kindOf={kindOf} onEdit={(p) => setEdit({ kind: "project", item: p })} onArchive={async (name) => {
            try { const r = await post<LibChange>("/api/library/project", { op: "set", name, archived: true }); reload(); toast(r.what); } catch (e) { toast.error((e as Error).message); }
          }} />}
          {sub === "categories" && <Categories lib={lib} now={now} q={q} staleOnly={staleOnly} onEdit={(c) => setEdit({ kind: "category", item: c })} />}
          {sub === "goals" && (lib.goals.length ? (
            <Table>
              <TableHeader><TableRow className="hover:bg-transparent"><TableHead className={head}>On</TableHead><TableHead className={head}>Goal</TableHead></TableRow></TableHeader>
              <TableBody>
                {lib.goals.map((g) => (
                  <TableRow key={`${g.name}${g.week}${g.atMost}`} className="cursor-pointer" onClick={() => setEdit({ kind: "goal", item: g })}>
                    <TableCell className={cell}>{g.name}</TableCell>
                    <TableCell className={cn(cell, "num")}>{goalTarget(g)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : <Empty>No goals yet. Add one to see it on Today and Week.</Empty>)}
        </Panel>
      )}
      <LibrarySheet edit={edit} lib={lib} onClose={() => setEdit(null)} onDone={reload} />
      <Dialog open={!!merge} onOpenChange={(o) => !o && setMerge(null)}>
        {merge && (
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Merge {merge.b} into {merge.a}?</DialogTitle>
              <DialogDescription>Every entry, rule and goal on {merge.b} moves to {merge.a}, and {merge.b} goes away. This can't be undone.</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="ghost" onClick={() => setMerge(null)}>Cancel</Button>
              <Button onClick={() => doMerge(merge)}>Merge</Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
    </div>
  );
}

function Seen({ ms, now }: { ms: number; now: number }) {
  return <TableCell className={cn(cell, "w-24 text-right text-muted-foreground")}>{seenText(ms, now)}</TableCell>;
}

function Projects({ lib, now, q, staleOnly, kindOf, onEdit, onArchive }: {
  lib: Lib; now: number; q: string; staleOnly: boolean; kindOf: (c: string) => string;
  onEdit: (p: Lib["projects"][number]) => void; onArchive: (name: string) => void;
}) {
  const rows = lib.projects.filter((p) => matches(p.name, [p.category, ...p.aliases], q) && (!staleOnly || isStale(p.lastSeenMS, now)));
  const live = rows.filter((p) => !p.archived);
  const archived = rows.filter((p) => p.archived);
  if (!rows.length) return <Empty>No projects match.</Empty>;
  const row = (p: Lib["projects"][number]) => {
    const stale = isStale(p.lastSeenMS, now);
    return (
      <TableRow key={p.name} className={cn("cursor-pointer", (stale || p.archived) && "text-muted-foreground")} onClick={() => onEdit(p)}>
        <TableCell className={cn(cell, "max-w-0 truncate")}>
          {p.name}
          {p.aliases.length > 0 && <span className="ml-2 text-muted-foreground">{p.aliases.join(", ")}</span>}
        </TableCell>
        <TableCell className={cn(cell, "w-40 truncate")}>
          {p.category ? <><i className="mr-2 inline-block h-2 w-2 rounded-sm align-[1px]" style={{ background: kindColor(kindOf(p.category)) }} />{p.category}</> : "—"}
        </TableCell>
        <TableCell className={cn(cell, "num w-28 text-right")}>{rateText(p.rate, lib.currency)}</TableCell>
        <TableCell className={cn(cell, "num w-20 text-right")}>{clock(p.last30MS)}</TableCell>
        <Seen ms={p.lastSeenMS} now={now} />
        <TableCell className={cn(cell, "w-20 text-right")}>
          {stale && !p.archived && <Button size="sm" variant="ghost" className="h-6 px-2 text-xs" onClick={(e) => { e.stopPropagation(); onArchive(p.name); }}>Archive</Button>}
        </TableCell>
      </TableRow>
    );
  };
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className={head}>Project</TableHead><TableHead className={head}>Category</TableHead>
          <TableHead className={cn(head, "text-right")}>Rate</TableHead><TableHead className={cn(head, "text-right")}>30 days</TableHead>
          <TableHead className={cn(head, "text-right")}>Last seen</TableHead><TableHead className={head} />
        </TableRow>
      </TableHeader>
      <TableBody>
        {live.map(row)}
        {archived.length > 0 && (
          <TableRow className="hover:bg-transparent"><TableCell colSpan={6} className="lbl px-0 pt-4">Archived ({archived.length})</TableCell></TableRow>
        )}
        {archived.map(row)}
      </TableBody>
    </Table>
  );
}

function Categories({ lib, now, q, staleOnly, onEdit }: { lib: Lib; now: number; q: string; staleOnly: boolean; onEdit: (c: Lib["categories"][number]) => void }) {
  const rows = lib.categories.filter((c) => matches(c.name, [c.kind], q) && (!staleOnly || isStale(c.lastSeenMS, now)));
  if (!rows.length) return <Empty>No categories match.</Empty>;
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className={head}>Category</TableHead><TableHead className={head}>Kind</TableHead>
          <TableHead className={cn(head, "text-right")}>Away after</TableHead><TableHead className={cn(head, "text-right")}>Projects</TableHead>
          <TableHead className={cn(head, "text-right")}>30 days</TableHead><TableHead className={cn(head, "text-right")}>Last seen</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((c) => (
          <TableRow key={c.name} className={cn("cursor-pointer", isStale(c.lastSeenMS, now) && "text-muted-foreground")} onClick={() => onEdit(c)}>
            <TableCell className={cn(cell, "max-w-0 truncate")}>
              <i className="mr-2 inline-block h-2 w-2 rounded-sm align-[1px]" style={{ background: kindColor(c.kind) }} />{c.name}
              {!c.interrupt && <span className="ml-2 text-muted-foreground">never interrupts</span>}
            </TableCell>
            <TableCell className={cn(cell, "w-28")}>{kindLabel(c.kind)}</TableCell>
            <TableCell className={cn(cell, "num w-24 text-right")}>{c.afkMS ? `${c.afkMS / 60_000} min` : "default"}</TableCell>
            <TableCell className={cn(cell, "num w-20 text-right")}>{c.projects || ""}</TableCell>
            <TableCell className={cn(cell, "num w-20 text-right")}>{clock(c.last30MS)}</TableCell>
            <Seen ms={c.lastSeenMS} now={now} />
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
