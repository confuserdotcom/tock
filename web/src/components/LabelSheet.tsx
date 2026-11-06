import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Button } from "@/components/ui/button";
import { get, post, type Done, type Labels, type Propose } from "@/lib/api";
import { dur } from "@/lib/format";
import { kindColor } from "@/lib/kinds";
import { atClock, clockOf, rangeError } from "@/lib/range";

/** A range on the timeline: the times show as HH:MM on date and can be
 *  changed. With ids too, labelling the untouched range tags those entries. */
export type Range = { date: string; from: number; to: number; mode?: Mode };
export type Target = { title: string; sub?: string; key?: string; ids?: number[]; range?: Range };
type Mode = "label" | "add" | "cut";

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

const MODES: { v: Mode; label: string; hint: string }[] = [
  { v: "label", label: "Label", hint: "Labels everything in the range; time away in it counts as worked." },
  { v: "add", label: "Add entry", hint: "Replaces the range with one entry, for time tock didn't see." },
  { v: "cut", label: "Delete", hint: "Removes the range from the timeline." },
];

/** LabelSheet picks a category (and project) for a key, some entries, or a
 *  range of the timeline. With a key, Return also saves the rule tock
 *  proposes. A range can be labelled, replaced by an entry, or deleted. */
export function LabelSheet({ target, onClose, onDone }: { target: Target | null; onClose: () => void; onDone: () => void }) {
  const [labels, setLabels] = useState<Labels | null>(null);
  const [q, setQ] = useState("");
  const [mode, setMode] = useState<Mode>("label");
  const [fromText, setFromText] = useState("");
  const [toText, setToText] = useState("");
  useEffect(() => {
    if (target && !labels) get<Labels>("/api/labels").then(setLabels).catch((e) => toast.error(e.message));
    if (!target) return;
    setQ("");
    setMode(target.range?.mode ?? "label");
    if (target.range) {
      setFromText(clockOf(target.range.date, target.range.from));
      setToText(clockOf(target.range.date, target.range.to));
    }
  }, [target, labels]);

  const r = target?.range;
  const from = r ? atClock(r.date, fromText) : null;
  const to = r ? atClock(r.date, toText) : null;
  const bad = r ? rangeError(from, to, Date.now()) : "";
  const moved = !!r && (from !== r.from || to !== r.to);
  const what = r && from !== null && to !== null && to > from ? `${clockOf(r.date, from)}–${clockOf(r.date, to)} (${dur(to - from)})` : "";

  async function finish(msg: string) {
    onClose();
    onDone();
    undoToast(msg, onDone);
  }

  async function edit(kind: Mode, label: string) {
    if (bad || from === null || to === null) return toast.error(bad);
    await post<{ id: number }>("/api/edit", { kind, from, to, label });
    const verb = kind === "add" ? "Added" : kind === "cut" ? "Deleted" : "Labelled";
    await finish(kind === "cut" ? `${verb} ${what}` : `${verb} ${what} · ${label}`);
  }

  async function apply(label: string, learn: boolean) {
    if (!target) return;
    try {
      if (r && (mode !== "label" || moved || !target.ids?.length)) return await edit(mode, label);
      let res: Done;
      if (learn && target.key) {
        const p = await get<Propose>(`/api/propose?key=${encodeURIComponent(target.key)}&label=${encodeURIComponent(label)}`);
        res = p.candidates.length
          ? await post<Done>("/api/learn", { key: target.key, label, rule: p.candidates[0].rule })
          : await post<Done>("/api/tag", { key: target.key, label });
      } else {
        res = await post<Done>("/api/tag", target.key ? { key: target.key, label } : { ids: target.ids, label });
      }
      await finish(`${res.label} · ${res.spans} entries, ${dur(res.ms)}`);
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function cut() {
    try {
      await edit("cut", "");
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  const projects = labels?.projects ?? [];
  const learn = !!target?.key;
  return (
    <Sheet open={!!target} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="flex w-[420px] flex-col gap-4 sm:max-w-[420px]">
        <SheetHeader>
          <SheetTitle className="truncate">{target?.title}</SheetTitle>
          {target?.sub && <SheetDescription className="truncate">{target.sub}</SheetDescription>}
        </SheetHeader>
        {r && (
          <div className="space-y-3">
            <div className="flex items-end gap-2">
              <Clock label="From" value={fromText} onChange={setFromText} />
              <span className="pb-2 text-muted-foreground">–</span>
              <Clock label="To" value={toText} onChange={setToText} />
              <span className={`num ml-auto pb-2 text-xs ${bad ? "text-bad" : "text-muted-foreground"}`}>{bad || dur((to ?? 0) - (from ?? 0))}</span>
            </div>
            <ToggleGroup type="single" value={mode} onValueChange={(v) => v && setMode(v as Mode)} variant="outline" size="sm" className="justify-start">
              {MODES.map((m) => (
                <ToggleGroupItem key={m.v} value={m.v}>{m.label}</ToggleGroupItem>
              ))}
            </ToggleGroup>
            <p className="text-xs text-muted-foreground">{MODES.find((m) => m.v === mode)?.hint}</p>
          </div>
        )}
        {mode === "cut" && r ? (
          <Button onClick={cut} disabled={!!bad} className="self-start">Delete {what}</Button>
        ) : (
          <Command className="flex-1 rounded-lg border">
            <CommandInput placeholder="Category, or Category / project" value={q} onValueChange={setQ} autoFocus={!r} />
            <CommandList className="max-h-none flex-1">
              <CommandEmpty>
                {q.trim() ? (
                  <button className="w-full px-2 py-1 text-left" onClick={() => apply(q.trim(), learn)}>
                    Use “{q.trim()}”
                  </button>
                ) : (
                  "No categories"
                )}
              </CommandEmpty>
              <CommandGroup heading="Categories">
                {labels?.categories.map((c) => (
                  <CommandItem key={c.name} value={c.name} onSelect={() => apply(c.name, learn)}>
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
                      <CommandItem key={p} value={`${cat} / ${p}`} onSelect={() => apply(`${cat} / ${p}`, learn)}>
                        {cat} / {p}
                      </CommandItem>
                    );
                  })}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        )}
        {!r && (
          <p className="text-xs text-muted-foreground">
            {learn ? "Return labels it and saves a rule, so tock files the same thing by itself next time." : "Return labels these entries only."}
          </p>
        )}
      </SheetContent>
    </Sheet>
  );
}

function Clock({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[11px] uppercase tracking-[0.06em] text-faint">{label}</span>
      <input
        className="num h-8 w-20 rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        value={value}
        inputMode="numeric"
        maxLength={5}
        placeholder="HH:MM"
        onChange={(e) => onChange(e.target.value)}
      />
    </label>
  );
}
