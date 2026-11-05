import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, Panel } from "@/components/Panel";
import { LabelSheet, undoToast, type Target } from "@/components/LabelSheet";
import { get, post, useApi, type Done, type Inbox as InboxT, type Item, type Labels, type Propose } from "@/lib/api";
import { clock, dur } from "@/lib/format";
import { kindColor } from "@/lib/kinds";
import { cn } from "@/lib/utils";

/** Inbox: what tock could not file by itself. j/k move, Return accepts the
 *  guess and learns a rule, l opens the label sheet, i ignores, u undoes. */
export default function Inbox({ onChange }: { onChange: () => void }) {
  const { data, error, loading, reload } = useApi<InboxT>("/api/inbox?days=7");
  const labels = useApi<Labels>("/api/labels");
  const [at, setAt] = useState(0);
  const [target, setTarget] = useState<Target | null>(null);
  const items = data?.items ?? [];
  const done = () => (reload(), onChange());
  const kindOf = (label?: string) => labels.data?.categories.find((c) => c.name === label?.split(" / ")[0])?.kind ?? "neutral";

  async function accept(it: Item) {
    if (!it.suggestion) return setTarget({ title: it.key, sub: it.example, key: it.key });
    try {
      const p = await get<Propose>(`/api/propose?key=${encodeURIComponent(it.key)}&label=${encodeURIComponent(it.suggestion.label)}`);
      const r = p.candidates.length
        ? await post<Done>("/api/learn", { key: it.key, label: it.suggestion.label, rule: p.candidates[0].rule })
        : await post<Done>("/api/tag", { key: it.key, label: it.suggestion.label });
      done();
      undoToast(`${r.label} · ${r.spans} entries, ${dur(r.ms)}`, done);
    } catch (e) {
      toast.error((e as Error).message);
    }
  }
  async function ignore(it: Item) {
    try {
      await post("/api/ignore", { key: it.key });
      done();
      toast(`Ignored ${it.key}`, { action: { label: "Undo", onClick: async () => (await post("/api/ignore", { key: it.key, undo: true }), done()) } });
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  useEffect(() => {
    const on = async (e: KeyboardEvent) => {
      if (target || e.metaKey || e.ctrlKey || (e.target as HTMLElement).closest("input,textarea,[role=dialog]")) return;
      const it = items[at];
      if (e.key === "j" || e.key === "ArrowDown") setAt((a) => Math.min(items.length - 1, a + 1));
      else if (e.key === "k" || e.key === "ArrowUp") setAt((a) => Math.max(0, a - 1));
      else if (e.key === "Enter" && it) accept(it);
      else if (e.key === "l" && it) setTarget({ title: it.key, sub: it.example, key: it.key });
      else if (e.key === "i" && it) ignore(it);
      else if (e.key === "u") {
        const r = await post<{ what: string }>("/api/undo", {}).catch((x: Error) => (toast.error(x.message), null));
        if (r) (toast(r.what ? `Undid: ${r.what}` : "Nothing to undo"), done());
      } else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  });
  useEffect(() => setAt((a) => Math.min(a, Math.max(0, items.length - 1))), [items.length]);

  if (error) return <Empty>Couldn't load the inbox: {error}</Empty>;
  if (loading && !data) return <Skeleton className="h-96" />;
  if (!data) return null;

  return (
    <div className="space-y-5">
      <div className="flex items-baseline justify-between gap-4">
        <h1 className="text-xl font-semibold tracking-tight">
          {items.length ? `${items.length} things need you` : "Inbox is empty"}
          <span className="num ml-2 text-base font-normal text-muted-foreground">last {data.days} days · {dur(data.totalMS)}</span>
        </h1>
        <span className="text-xs text-faint">j k move · return accept · l label · i ignore · u undo</span>
      </div>
      <Panel label="To label" aside={data.smallKeys ? `${data.smallKeys} small things (${dur(data.smallMS)}) left out` : undefined}>
        {items.length === 0 ? (
          <Empty>Everything is filed. tock asks again when something new shows up.</Empty>
        ) : (
          <ul className="divide-y">
            {items.map((it, i) => (
              <li
                key={it.key}
                onClick={() => setAt(i)}
                className={cn("grid cursor-default grid-cols-[1fr_auto_auto] items-center gap-x-4 rounded-md px-3 py-2.5", i === at && "bg-secondary")}
              >
                <div className="min-w-0">
                  <div className="truncate font-medium">{it.key}</div>
                  <div className="truncate text-xs text-muted-foreground">
                    {it.app} · {it.spans} entries{it.example && !it.private ? ` · ${it.example}` : ""}
                  </div>
                </div>
                <span className="num">{clock(it.ms)}</span>
                <div className="flex items-center gap-2">
                  {it.suggestion ? (
                    <Badge variant="outline" className="gap-1.5 font-normal" title={it.suggestion.reason}>
                      <i className="h-2 w-2 rounded-sm" style={{ background: kindColor(kindOf(it.suggestion.label)) }} />
                      {it.suggestion.label}?
                    </Badge>
                  ) : (
                    <span className="text-xs text-faint">no guess</span>
                  )}
                  <Button size="sm" variant={i === at ? "default" : "secondary"} className="h-7" onClick={() => accept(it)}>
                    {it.suggestion ? "Accept" : "Label"}
                  </Button>
                  <Button size="sm" variant="ghost" className="h-7" onClick={() => setTarget({ title: it.key, sub: it.example, key: it.key })}>
                    Other…
                  </Button>
                  <Button size="sm" variant="ghost" className="h-7 text-muted-foreground" onClick={() => ignore(it)}>
                    Ignore
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Panel>
      <LabelSheet target={target} onClose={() => setTarget(null)} onDone={done} />
    </div>
  );
}
