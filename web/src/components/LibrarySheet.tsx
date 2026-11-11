import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { post, type LibCategory, type LibChange, type LibGoal, type LibProject, type Library } from "@/lib/api";
import { KINDS, kindLabel } from "@/lib/kinds";
import { undoToast } from "@/components/LabelSheet";

export type Edit =
  | { kind: "category"; item: LibCategory | null }
  | { kind: "project"; item: LibProject | null }
  | { kind: "goal"; item: LibGoal | null };

const field = "h-8 w-full rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const MIN = 60_000;

function Row({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="lbl">{label}</span>
      {children}
      {error && <span className="block text-xs text-bad">{error}</span>}
    </label>
  );
}

/** LibrarySheet adds or edits one category, project or goal. Errors show
 *  under the form; the toast's Undo reverses changes that only touched
 *  rules.toml. */
export function LibrarySheet({ edit, lib, onClose, onDone }: { edit: Edit | null; lib: Library | null; onClose: () => void; onDone: () => void }) {
  const [v, setV] = useState<Record<string, string | boolean>>({});
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setErr("");
    if (!edit) return;
    if (edit.kind === "category") {
      const c = edit.item;
      setV({ name: c?.name ?? "", kind: c?.kind ?? "deep", afk: c?.afkMS ? String(c.afkMS / MIN) : "", interrupt: c?.interrupt ?? true });
    } else if (edit.kind === "project") {
      const p = edit.item;
      setV({ name: p?.name ?? "", category: p?.category ?? "", aliases: p?.aliases.join(", ") ?? "", rate: p?.rate != null ? String(p.rate) : "", archived: p?.archived ?? false });
    } else {
      const g = edit.item;
      setV({ label: g ? g.project ? `${g.category}/${g.project}` : g.category : "", hours: g ? String(g.ms / 3_600_000) : "", week: g?.week ?? false, atMost: g?.atMost ?? false });
    }
  }, [edit]);
  if (!edit) return null;
  const s = (k: string) => String(v[k] ?? "");
  const b = (k: string) => Boolean(v[k]);
  const set = (k: string, x: string | boolean) => setV((o) => ({ ...o, [k]: x }));
  const adding = edit.item === null;

  async function send(path: string, body: object) {
    setBusy(true);
    try {
      const r = await post<LibChange>(path, body);
      if (r.undo) undoToast(cap(r.what), onDone);
      else toast(cap(r.what));
      onDone();
      onClose();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    setErr("");
    if (edit!.kind === "category") {
      const c = edit!.item as LibCategory | null;
      const afkMS = s("afk") ? Math.round(Number(s("afk")) * MIN) : c?.afkMS ? -1 : 0;
      if (c && s("name") !== c.name) return send("/api/library/category", { op: "rename", name: c.name, new: s("name") });
      return send("/api/library/category", { op: adding ? "add" : "set", name: s("name"), kind: s("kind"), afkMS, interrupt: b("interrupt") });
    }
    if (edit!.kind === "project") {
      const p = edit!.item as LibProject | null;
      if (p && s("name") !== p.name) return send("/api/library/project", { op: "rename", name: p.name, new: s("name") });
      const rate = s("rate").trim() === "" ? (p?.rate != null ? -1 : undefined) : Number(s("rate"));
      return send("/api/library/project", {
        op: adding ? "add" : "set", name: s("name"), category: s("category"),
        aliases: s("aliases").split(",").map((a) => a.trim()).filter(Boolean), rate, archived: b("archived"),
      });
    }
    const [category, project = ""] = s("label").split("/");
    const goal = { category, project, week: b("week"), atMost: b("atMost"), ms: Math.round(Number(s("hours")) * 3_600_000) };
    const g = edit!.item as LibGoal | null;
    return send("/api/library/goal", { op: "set", goal, was: g ? { category: g.category, project: g.project, week: g.week, atMost: g.atMost, ms: g.ms } : undefined });
  }

  async function remove() {
    if (edit!.kind === "goal") {
      const g = edit!.item!;
      return send("/api/library/goal", { op: "remove", goal: { category: g.category, project: g.project, week: g.week, atMost: g.atMost, ms: 0 } });
    }
    return send(`/api/library/${edit!.kind}`, { op: "remove", name: (edit!.item as LibCategory | LibProject).name });
  }

  const labels = lib ? [...lib.categories.map((c) => c.name), ...lib.projects.filter((p) => p.category).map((p) => `${p.category}/${p.name}`)] : [];
  const title = `${adding ? "Add" : "Edit"} ${edit.kind}`;
  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-[380px] space-y-5">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{edit.kind === "goal" ? "Time to reach, or a limit, per day or week." : adding ? "Rules and labels can file time under it once it's added." : "Renaming moves its rules, goals and past entries along."}</SheetDescription>
        </SheetHeader>
        <form className="space-y-4" onSubmit={(e) => (e.preventDefault(), save())}>
          {edit.kind !== "goal" && (
            <Row label="Name"><input className={field} value={s("name")} onChange={(e) => set("name", e.target.value)} autoFocus /></Row>
          )}
          {edit.kind === "category" && (
            <>
              <Row label="Kind">
                <select className={field} value={s("kind")} onChange={(e) => set("kind", e.target.value)}>
                  {KINDS.map((k) => <option key={k} value={k}>{kindLabel(k)}</option>)}
                </select>
              </Row>
              <Row label="Away after (minutes, empty = default)"><input className={field} inputMode="numeric" value={s("afk")} onChange={(e) => set("afk", e.target.value)} /></Row>
              <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={b("interrupt")} onChange={(e) => set("interrupt", e.target.checked)} /> Can interrupt a session</label>
            </>
          )}
          {edit.kind === "project" && (
            <>
              <Row label="Category (empty = none)">
                <select className={field} value={s("category")} onChange={(e) => set("category", e.target.value)}>
                  <option value="">None</option>
                  {lib?.categories.map((c) => <option key={c.name} value={c.name}>{c.name}</option>)}
                </select>
              </Row>
              <Row label="Other names (comma-separated)"><input className={field} value={s("aliases")} onChange={(e) => set("aliases", e.target.value)} /></Row>
              {lib?.billing !== false && (
                <Row label={`Hourly rate${lib?.currency ? ` (${lib.currency})` : ""}, empty = none`}><input className={field} inputMode="decimal" value={s("rate")} onChange={(e) => set("rate", e.target.value)} /></Row>
              )}
              <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={b("archived")} onChange={(e) => set("archived", e.target.checked)} /> Archived</label>
            </>
          )}
          {edit.kind === "goal" && (
            <>
              <Row label="On">
                <select className={field} value={s("label")} onChange={(e) => set("label", e.target.value)} autoFocus>
                  <option value="" disabled>Pick a category or module</option>
                  {labels.map((l) => <option key={l} value={l}>{l.replace("/", " / ")}</option>)}
                </select>
              </Row>
              <Row label="Hours"><input className={field} inputMode="decimal" value={s("hours")} onChange={(e) => set("hours", e.target.value)} /></Row>
              <div className="flex gap-5 text-sm">
                <label className="flex items-center gap-2"><input type="checkbox" checked={b("week")} onChange={(e) => set("week", e.target.checked)} /> Per week</label>
                <label className="flex items-center gap-2"><input type="checkbox" checked={b("atMost")} onChange={(e) => set("atMost", e.target.checked)} /> At most (a limit)</label>
              </div>
            </>
          )}
          {err && <p className="text-sm text-bad">{err}</p>}
          <div className="flex items-center gap-2 pt-2">
            <Button type="submit" size="sm" disabled={busy}>{adding ? "Add" : "Save"}</Button>
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>Cancel</Button>
            {!adding && <Button type="button" variant="ghost" size="sm" className="ml-auto text-bad" disabled={busy} onClick={remove}>Delete</Button>}
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}

const cap = (t: string) => t.charAt(0).toUpperCase() + t.slice(1);
