import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { undoToast } from "@/components/LabelSheet";
import { post, type LibChange, type Library, type Rule, type RulePreview } from "@/lib/api";
import { dur } from "@/lib/format";
import { labelValue } from "@/lib/rules";

const field = "h-8 w-full rounded-md border bg-background px-2.5 font-mono text-[13px] placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const FIELDS = [
  { key: "app", label: "App", hint: "Zed" },
  { key: "url", label: "Site or URL", hint: "github\\.com" },
  { key: "title", label: "Title", hint: "^Inbox" },
  { key: "dir", label: "Folder (terminals)", hint: "~/code/tock" },
] as const;
type Key = (typeof FIELDS)[number]["key"] | "label";
type Form = Record<Key, string>;

/** RuleSheet adds a rule (rule null) or edits one, with a live preview of
 *  what it would label over the last 30 days. */
export function RuleSheet({ open, rule, lib, onClose, onDone }: {
  open: boolean; rule: Rule | null; lib: Library | null; onClose: () => void; onDone: () => void;
}) {
  const [v, setV] = useState<Form>({ app: "", url: "", title: "", dir: "", label: "" });
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [prev, setPrev] = useState<RulePreview | null>(null);
  const [prevErr, setPrevErr] = useState("");

  useEffect(() => {
    setErr("");
    setPrev(null);
    setPrevErr("");
    setV(rule ? { app: rule.app, url: rule.url, title: rule.title, dir: rule.dir, label: labelValue(rule) } : { app: "", url: "", title: "", dir: "", label: "" });
  }, [rule, open]);

  const ref = rule ? { file: rule.file, index: rule.index, fingerprint: rule.fingerprint } : {};
  const body = (op: string) => ({ op, ...ref, rule: v });

  // The preview follows the form 400 ms after typing stops.
  useEffect(() => {
    if (!open) return;
    const empty = !v.app && !v.url && !v.title && !v.dir;
    if (empty || !v.label) {
      setPrev(null);
      setPrevErr("");
      return;
    }
    let live = true;
    const t = setTimeout(() => {
      post<RulePreview>("/api/rules/preview", body(rule ? "set" : "add"))
        .then((p) => live && (setPrev(p), setPrevErr("")))
        .catch((e: Error) => live && (setPrev(null), setPrevErr(e.message)));
    }, 400);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [v, open]);

  if (!open) return null;
  const set = (k: Key, x: string) => setV((o) => ({ ...o, [k]: x }));

  async function send(op: string) {
    setBusy(true);
    setErr("");
    try {
      const r = await post<LibChange>("/api/rules", body(op));
      const what = r.what.charAt(0).toUpperCase() + r.what.slice(1);
      if (r.undo) undoToast(what, onDone);
      else toast(what);
      onDone();
      onClose();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const labels = lib ? [...lib.categories.map((c) => c.name), ...lib.projects.filter((p) => p.category && !p.archived).map((p) => `${p.category}/${p.name}`)] : [];
  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-[420px] space-y-5 overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{rule ? "Edit rule" : "Add rule"}</SheetTitle>
          <SheetDescription>
            {rule?.learned ? "Learned from a label you gave. Change it, or delete it if it files things wrongly." : "Patterns ignore case. Fill in one or more; time must match all of them."}
          </SheetDescription>
        </SheetHeader>
        <form className="space-y-4" onSubmit={(e) => (e.preventDefault(), send(rule ? "set" : "add"))}>
          {rule && rule.bundle.length > 0 && (
            <p className="text-xs text-muted-foreground">Also matches apps {rule.bundle.join(", ")} (edit in {rule.file}).</p>
          )}
          {FIELDS.map((f, i) => (
            <label key={f.key} className="block space-y-1.5">
              <span className="lbl">{f.label}</span>
              <input className={field} value={v[f.key]} placeholder={f.hint} onChange={(e) => set(f.key, e.target.value)} autoFocus={i === 0} spellCheck={false} />
            </label>
          ))}
          <label className="block space-y-1.5">
            <span className="lbl">Files it as</span>
            <input className={field} list="rule-labels" value={v.label} placeholder="Category or Category/Project" onChange={(e) => set("label", e.target.value)} spellCheck={false} />
            <datalist id="rule-labels">{labels.map((l) => <option key={l} value={l} />)}</datalist>
          </label>
          <Preview p={prev} err={prevErr} />
          {err && <p className="text-sm text-bad">{err}</p>}
          <div className="flex items-center gap-2 pt-2">
            <Button type="submit" size="sm" disabled={busy}>{rule ? "Save" : "Add"}</Button>
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>Cancel</Button>
            {rule && <Button type="button" variant="ghost" size="sm" className="ml-auto text-bad" disabled={busy} onClick={() => send("remove")}>Delete</Button>}
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}

function Preview({ p, err }: { p: RulePreview | null; err: string }) {
  if (err) return <p className="rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground">{err}</p>;
  if (!p) return null;
  return (
    <div className="space-y-2 rounded-md border px-3 py-2.5">
      <p className="text-xs text-muted-foreground">
        Last 30 days: it would label <b className="num font-medium text-foreground">{dur(p.matchedMS)}</b>
        {" "}and change <b className="num font-medium text-foreground">{dur(p.changedMS)}</b>.
      </p>
      {p.examples.length > 0 && (
        <ul className="space-y-1 text-xs">
          {p.examples.map((e) => (
            <li key={e.text + e.from + e.to} className="flex gap-2">
              <span className="min-w-0 flex-1 truncate">{e.text}</span>
              <span className="num text-muted-foreground">{dur(e.ms)}</span>
              <span className="w-40 truncate text-right text-muted-foreground">{(e.from || "Unlabelled") + " → " + (e.to || "Unlabelled")}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
