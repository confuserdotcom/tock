import { useEffect, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Panel } from "@/components/Panel";
import { post, type InvoiceClient, type InvoiceDetails, type Settings, type WorkHours } from "@/lib/api";
import { cn } from "@/lib/utils";

const DAYS: [string, string][] = [["mon", "Mon"], ["tue", "Tue"], ["wed", "Wed"], ["thu", "Thu"], ["fri", "Fri"], ["sat", "Sat"], ["sun", "Sun"]];
const ROUNDS: [string, string][] = [["", "No rounding"], ["1m", "1 minute"], ["6m", "6 minutes"], ["10m", "10 minutes"], ["15m", "15 minutes"], ["30m", "30 minutes"], ["1h", "1 hour"]];

const input = "h-8 w-full rounded-md border bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

/** The server's message without config.toml's table name: "[hours] start …". */
function plain(e: unknown): string {
  return (e as Error).message.replace(/^\[[a-z.]+\]\s*/, "").replace(/^\[\[invoice\.client\]\]\s*/, "Client ");
}

/** useForm keeps an edited copy of a section, its error, and whether it
 *  differs from what is saved. */
function useForm<T>(saved: T) {
  const [v, setV] = useState<T>(saved);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const key = JSON.stringify(saved);
  useEffect(() => { setV(saved); setErr(""); }, [key]); // eslint-disable-line react-hooks/exhaustive-deps
  const dirty = JSON.stringify(v) !== key;
  return { v, setV, err, setErr, busy, setBusy, dirty, reset: () => (setV(saved), setErr("")) };
}

/** Footer is a form's Save and Discard, shown while it has changes, and its error. */
function Footer({ dirty, busy, err, onSave, onReset, disabled }: { dirty: boolean; busy: boolean; err: string; onSave: () => void; onReset: () => void; disabled?: boolean }) {
  return (
    <div className="mt-4 flex min-h-8 items-center gap-3">
      {err && <p className="min-w-0 flex-1 text-sm text-bad" role="alert">{err}</p>}
      {!err && <span className="flex-1 text-xs text-faint">{dirty ? "Not saved yet" : ""}</span>}
      {dirty && <Button variant="ghost" size="sm" onClick={onReset} disabled={busy}>Discard</Button>}
      <Button size="sm" onClick={onSave} disabled={!dirty || busy || disabled}>{busy ? "Saving…" : "Save"}</Button>
    </div>
  );
}

function Field({ label, hint, children, className }: { label: string; hint?: string; children: ReactNode; className?: string }) {
  return (
    <label className={cn("block min-w-0", className)}>
      <span className="mb-1 block text-xs text-muted-foreground">{label}</span>
      {children}
      {hint && <span className="mt-1 block text-[11px] text-faint">{hint}</span>}
    </label>
  );
}

type SaveTo = (path: string, body: unknown, done: string) => Promise<void>;

/** useSave posts a section and hands the new settings up; it throws the
 *  server's reason for the form to show. */
export function useSave(setS: (s: Settings) => void, onChange: () => void): SaveTo {
  return async (path, body, done) => {
    const r = await post<Settings>(path, body);
    setS(r);
    onChange();
    toast(done);
  };
}

export function HoursPanel({ hours, save, disabled }: { hours: WorkHours; save: SaveTo; disabled?: boolean }) {
  const f = useForm(hours);
  const h = f.v;
  async function submit() {
    f.setBusy(true);
    try {
      await save("/api/settings/hours", h, h.set ? "Saved your work hours" : "Work hours off");
    } catch (e) {
      f.setErr(plain(e));
    }
    f.setBusy(false);
  }
  const toggleDay = (d: string) => f.setV({ ...h, days: h.days.includes(d) ? h.days.filter((x) => x !== d) : DAYS.map(([k]) => k).filter((k) => k === d || h.days.includes(k)) });
  return (
    <Panel id="set-hours" label="Work hours">
      <div className="flex items-center gap-4">
        <label htmlFor="hours-set" className="min-w-0 flex-1 cursor-pointer">
          <span>Keep to work hours</span>
          <span className="block text-xs text-muted-foreground">
            The focus score counts only these hours, days off get no start-of-day notice, and Overworking needs them.
          </span>
        </label>
        <Switch id="hours-set" checked={h.set} onChange={(v) => f.setV({ ...h, set: v })} label="Keep to work hours" disabled={disabled} />
      </div>
      <div className={cn("mt-4 grid gap-4 sm:grid-cols-[8rem_8rem_1fr]", !h.set && "pointer-events-none opacity-40")} aria-disabled={!h.set}>
        <Field label="Start"><input type="time" className={input} value={h.start} onChange={(e) => f.setV({ ...h, start: e.target.value })} disabled={!h.set || disabled} /></Field>
        <Field label="End"><input type="time" className={input} value={h.end} onChange={(e) => f.setV({ ...h, end: e.target.value })} disabled={!h.set || disabled} /></Field>
        <div>
          <span className="mb-1 block text-xs text-muted-foreground">Days</span>
          <div className="flex flex-wrap gap-1" role="group" aria-label="Work days">
            {DAYS.map(([k, name]) => (
              <button key={k} type="button" aria-pressed={h.days.includes(k)} disabled={!h.set || disabled} onClick={() => toggleDay(k)}
                className={cn("h-8 w-11 rounded-md border text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  h.days.includes(k) ? "border-foreground bg-foreground text-background" : "text-muted-foreground hover:bg-accent")}>
                {name}
              </button>
            ))}
          </div>
        </div>
      </div>
      <Footer dirty={f.dirty} busy={f.busy} err={f.err} onSave={submit} onReset={f.reset} disabled={disabled} />
    </Panel>
  );
}

export function PresetsPanel({ presets, max, save, disabled, focusOff }: { presets: number[]; max: number; save: SaveTo; disabled?: boolean; focusOff?: boolean }) {
  const f = useForm(presets);
  const p = f.v;
  const set = (i: number, v: number) => f.setV(p.map((x, j) => (j === i ? v : x)));
  async function submit() {
    if (p.some((x) => !Number.isInteger(x) || x < 1 || x > max)) return f.setErr(`Each length is 1 to ${max} minutes`);
    f.setBusy(true);
    try {
      await save("/api/settings/presets", { presets: p }, "Saved the session lengths");
    } catch (e) {
      f.setErr(plain(e));
    }
    f.setBusy(false);
  }
  return (
    <Panel id="set-sessions" label="Focus sessions">
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">
        The lengths the menu bar, the notch and ⌥⌘F offer. The first is the default.
        {focusOff && " Focus is off, so none are offered until you turn it on."}
      </p>
      <ol className="flex flex-wrap items-end gap-2">
        {p.map((m, i) => (
          <li key={i} className="flex items-center gap-1 rounded-md border bg-secondary/50 py-1 pl-2 pr-1">
            <input type="number" min={1} max={max} value={Number.isNaN(m) ? "" : m} aria-label={`Session ${i + 1} in minutes`} disabled={disabled}
              onChange={(e) => set(i, e.target.valueAsNumber)}
              className="num h-7 w-14 rounded border bg-background px-1.5 text-right text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" />
            <span className="text-xs text-muted-foreground">min{i === 0 && " · default"}</span>
            {i > 0 && (
              <button type="button" className="ml-1 rounded px-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground" title="Make it the default" disabled={disabled}
                onClick={() => f.setV([m, ...p.filter((_, j) => j !== i)])}>
                First
              </button>
            )}
            <button type="button" aria-label={`Remove ${m} minutes`} title="Remove" disabled={disabled || p.length <= 1}
              className="grid h-6 w-6 place-items-center rounded hover:bg-accent disabled:opacity-30" onClick={() => f.setV(p.filter((_, j) => j !== i))}>
              <X className="h-3 w-3" />
            </button>
          </li>
        ))}
        {p.length < 6 && (
          <li>
            <Button variant="outline" size="sm" disabled={disabled} onClick={() => f.setV([...p, 30])}><Plus className="h-3.5 w-3.5" />Add a length</Button>
          </li>
        )}
      </ol>
      <Footer dirty={f.dirty} busy={f.busy} err={f.err} onSave={submit} onReset={f.reset} disabled={disabled} />
    </Panel>
  );
}

/** Lines edits a list of lines (an address) as a text box, one per line. */
function Lines({ value, onChange, disabled, label }: { value: string[]; onChange: (v: string[]) => void; disabled?: boolean; label: string }) {
  return (
    <textarea rows={Math.max(2, value.length + 1)} value={value.join("\n")} aria-label={label} disabled={disabled}
      onChange={(e) => onChange(e.target.value.split("\n"))}
      className="w-full resize-y rounded-md border bg-background px-2 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50" />
  );
}

export function InvoicePanel({ invoice, save, disabled }: { invoice: InvoiceDetails; save: SaveTo; disabled?: boolean }) {
  const f = useForm(invoice);
  const v = f.v;
  const set = <K extends keyof InvoiceDetails>(k: K, x: InvoiceDetails[K]) => f.setV({ ...v, [k]: x });
  const setClient = (i: number, c: InvoiceClient) => set("clients", v.clients.map((x, j) => (j === i ? c : x)));
  async function submit() {
    const nameless = v.clients.findIndex((c) => !c.name.trim());
    if (nameless >= 0) return f.setErr(`Client ${nameless + 1} needs a name`);
    f.setBusy(true);
    try {
      await save("/api/settings/invoice", v, "Saved the invoice details");
    } catch (e) {
      f.setErr(plain(e));
    }
    f.setBusy(false);
  }
  const d = disabled;
  return (
    <Panel id="set-invoices" label="Invoices">
      <p className="-mt-1 mb-4 text-sm text-muted-foreground">
        What every invoice says about you, how it rounds and taxes, and who it can be to. An issued invoice keeps the details it was issued with.
      </p>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Your name or business"><input className={input} value={v.name} onChange={(e) => set("name", e.target.value)} disabled={d} /></Field>
        <Field label="Your tax or VAT number" hint="Leave empty if you have none"><input className={input} value={v.taxId} onChange={(e) => set("taxId", e.target.value)} disabled={d} /></Field>
        <Field label="Your address" hint="One line per line"><Lines label="Your address" value={v.address} onChange={(x) => set("address", x)} disabled={d} /></Field>
        <Field label="Note at the bottom" hint="Payment terms, bank details"><textarea rows={3} className="w-full rounded-md border bg-background px-2 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" value={v.note} onChange={(e) => set("note", e.target.value)} disabled={d} /></Field>
      </div>

      <h3 className="mb-2 mt-6 text-xs font-medium text-muted-foreground">Amounts</h3>
      <div className="grid gap-4 sm:grid-cols-4">
        <Field label="Currency" hint="Printed after amounts"><input className={input} value={v.currency} placeholder="EUR" onChange={(e) => set("currency", e.target.value)} disabled={d} /></Field>
        <Field label="Decimals">
          <select className={input} value={v.minorUnits} onChange={(e) => set("minorUnits", Number(e.target.value))} disabled={d}>
            {[0, 1, 2, 3].map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
        </Field>
        <Field label="Round each day to">
          <select className={input} value={ROUNDS.some(([k]) => k === v.round) ? v.round : "custom"} onChange={(e) => set("round", e.target.value)} disabled={d}>
            {ROUNDS.map(([k, name]) => <option key={k} value={k}>{name}</option>)}
            {!ROUNDS.some(([k]) => k === v.round) && <option value="custom" disabled>{v.round}</option>}
          </select>
        </Field>
        <Field label="Rounding">
          <select className={input} value={v.roundMode || "nearest"} onChange={(e) => set("roundMode", e.target.value)} disabled={d || !v.round}>
            <option value="nearest">To the nearest</option>
            <option value="up">Always up</option>
          </select>
        </Field>
      </div>

      <h3 className="mb-2 mt-6 text-xs font-medium text-muted-foreground">Tax</h3>
      <div className="grid gap-4 sm:grid-cols-[8rem_8rem_1fr]">
        <Field label="Name"><input className={input} value={v.taxLabel} placeholder="VAT" onChange={(e) => set("taxLabel", e.target.value)} disabled={d} /></Field>
        <Field label="Rate, %" hint="Empty: no tax line"><input className={cn(input, "num")} inputMode="decimal" value={v.taxRate} onChange={(e) => set("taxRate", e.target.value)} disabled={d} /></Field>
        <Field label="Tax note" hint="Reverse charge, exemption"><input className={input} value={v.taxNote} onChange={(e) => set("taxNote", e.target.value)} disabled={d} /></Field>
      </div>

      <div className="mb-2 mt-6 flex items-center justify-between gap-3">
        <h3 className="text-xs font-medium text-muted-foreground">Clients</h3>
        <span className="text-[11px] text-faint">A project picks its client in the Library</span>
      </div>
      {v.clients.length === 0 && <p className="mb-2 text-sm text-faint">None yet: invoices then go to the project's name.</p>}
      <ul className="space-y-3">
        {v.clients.map((c, i) => (
          <li key={i} className="grid gap-3 rounded-md border p-3 sm:grid-cols-[1fr_1fr_auto]">
            <div className="space-y-3">
              <Field label="Name"><input className={input} value={c.name} onChange={(e) => setClient(i, { ...c, name: e.target.value })} disabled={d} /></Field>
              <Field label="Tax or VAT number"><input className={input} value={c.taxId} onChange={(e) => setClient(i, { ...c, taxId: e.target.value })} disabled={d} /></Field>
            </div>
            <Field label="Address"><Lines label={`${c.name || "Client"} address`} value={c.address} onChange={(x) => setClient(i, { ...c, address: x })} disabled={d} /></Field>
            <button type="button" className="h-8 self-start rounded-md px-2 text-xs text-muted-foreground hover:bg-accent hover:text-foreground" disabled={d}
              onClick={() => set("clients", v.clients.filter((_, j) => j !== i))}>Remove</button>
          </li>
        ))}
      </ul>
      <Button variant="outline" size="sm" className="mt-3" disabled={d} onClick={() => set("clients", [...v.clients, { name: "", address: [], taxId: "" }])}>
        <Plus className="h-3.5 w-3.5" />Add a client
      </Button>
      <Footer dirty={f.dirty} busy={f.busy} err={f.err} onSave={submit} onReset={f.reset} disabled={disabled} />
    </Panel>
  );
}
