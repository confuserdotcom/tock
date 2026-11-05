import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { get, post, type Done, type Labels, type Propose } from "@/lib/api";
import { dur } from "@/lib/format";
import { kindColor } from "@/lib/kinds";

export type Target = { title: string; sub?: string; key?: string; ids?: number[] };

/** undoToast reports a change with an Undo button that reverses it. */
export function undoToast(msg: string, after: () => void) {
  toast(msg, {
    action: {
      label: "Undo",
      onClick: async () => {
        try {
          const r = await post<{ what: string }>("/api/undo", {});
          toast(r.what ? `Undid: ${r.what}` : "Nothing to undo");
        } catch (e) {
          toast.error((e as Error).message);
        }
        after();
      },
    },
  });
}

/** LabelSheet picks a category (and project) for a key or some spans. With
 *  a key, "Label and learn" also saves the rule tock proposes. */
export function LabelSheet({ target, onClose, onDone }: { target: Target | null; onClose: () => void; onDone: () => void }) {
  const [labels, setLabels] = useState<Labels | null>(null);
  const [q, setQ] = useState("");
  useEffect(() => {
    if (target && !labels) get<Labels>("/api/labels").then(setLabels).catch((e) => toast.error(e.message));
    if (target) setQ("");
  }, [target, labels]);

  async function apply(label: string, learn: boolean) {
    if (!target) return;
    try {
      let res: Done;
      if (learn && target.key) {
        const p = await get<Propose>(`/api/propose?key=${encodeURIComponent(target.key)}&label=${encodeURIComponent(label)}`);
        res = p.candidates.length
          ? await post<Done>("/api/learn", { key: target.key, label, rule: p.candidates[0].rule })
          : await post<Done>("/api/tag", { key: target.key, label });
      } else {
        res = await post<Done>("/api/tag", target.key ? { key: target.key, label } : { ids: target.ids, label });
      }
      onClose();
      onDone();
      undoToast(`${res.label} · ${res.spans} entries, ${dur(res.ms)}`, onDone);
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  const projects = labels?.projects ?? [];
  return (
    <Sheet open={!!target} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="flex w-[420px] flex-col gap-4 sm:max-w-[420px]">
        <SheetHeader>
          <SheetTitle className="truncate">{target?.title}</SheetTitle>
          {target?.sub && <SheetDescription className="truncate">{target.sub}</SheetDescription>}
        </SheetHeader>
        <Command className="flex-1 rounded-lg border">
          <CommandInput placeholder="Category, or Category / project" value={q} onValueChange={setQ} autoFocus />
          <CommandList className="max-h-none flex-1">
            <CommandEmpty>
              {q.trim() ? (
                <button className="w-full px-2 py-1 text-left" onClick={() => apply(q.trim(), !!target?.key)}>
                  Use “{q.trim()}”
                </button>
              ) : (
                "No categories"
              )}
            </CommandEmpty>
            <CommandGroup heading="Categories">
              {labels?.categories.map((c) => (
                <CommandItem key={c.name} value={c.name} onSelect={() => apply(c.name, !!target?.key)}>
                  <i className="mr-2 h-2 w-2 rounded-sm" style={{ background: kindColor(c.kind) }} />
                  {c.name}
                </CommandItem>
              ))}
            </CommandGroup>
            {projects.length > 0 && q.includes("/") && (
              <CommandGroup heading="With a project">
                {projects.map((p) => {
                  const cat = q.split("/")[0].trim();
                  return (
                    <CommandItem key={p} value={`${cat} / ${p}`} onSelect={() => apply(`${cat} / ${p}`, !!target?.key)}>
                      {cat} / {p}
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
        <p className="text-xs text-muted-foreground">
          {target?.key ? "Return labels it and saves a rule, so tock files the same thing by itself next time." : "Return labels these entries only."}
        </p>
      </SheetContent>
    </Sheet>
  );
}
