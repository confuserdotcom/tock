import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Copy, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, Panel } from "@/components/Panel";
import { openShortcuts } from "@/components/Shortcuts";
import { post, useApi, type Feature, type Settings as SettingsT } from "@/lib/api";
import { getTheme, setTheme, type Theme } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { HoursPanel, InvoicePanel, PresetsPanel, useSave } from "@/views/SettingsForms";

const SECTIONS: [string, string][] = [
  ["features", "Features"], ["hours", "Work hours"], ["sessions", "Focus sessions"], ["invoices", "Invoices"],
  ["privacy", "Never record"], ["page", "This page"], ["files", "Files"],
];

/** Settings: what tock choice turns on and off, grouped, the apps and sites
 *  tock never records, and how this page looks. Every switch writes
 *  config.toml at once, keeping the rest of the file, and can be undone. */
export default function Settings({ onChange }: { onChange: () => void }) {
  const [n, setN] = useState(0);
  const { data, error, loading } = useApi<SettingsT>("/api/settings", n);
  const [s, setS] = useState<SettingsT | null>(null);
  const [ask, setAsk] = useState<Feature | null>(null);
  useEffect(() => { if (data) setS(data); }, [data]);
  const saveTo = useSave((x) => setS(x), onChange);

  async function save(f: Feature, on: boolean, quiet = false) {
    // Show it at once; the answer from the server is the truth.
    setS((x) => x && { ...x, features: x.features.map((y) => (y.id === f.id ? { ...y, on } : y)) });
    try {
      const r = await post<SettingsT>("/api/settings", { features: { [f.id]: on } });
      setS(r);
      onChange();
      if (!quiet) toast(`${f.name} ${on ? "on" : "off"}`, { action: { label: "Undo", onClick: () => save(f, !on, true) } });
    } catch (e) {
      toast.error((e as Error).message);
      setN((x) => x + 1);
    }
  }
  function flip(f: Feature, on: boolean) {
    if (f.id === "claude" && on) return setAsk(f); // sends data off the machine: ask first
    save(f, on);
  }

  if (error) return <Empty>Couldn't load the settings: {error}</Empty>;
  if (loading && !s) return <Skeleton className="h-96" />;
  if (!s) return null;

  const groups: { name: string; head?: Feature; items: Feature[] }[] = [];
  for (const f of s.features) {
    let g = groups.find((x) => x.name === f.group);
    if (!g) groups.push((g = { name: f.group, items: [] }));
    if (f.isGroup) g.head = f;
    else g.items.push(f);
  }
  const byId = (id?: string) => s.features.find((f) => f.id === id);

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div className="flex items-baseline justify-between gap-4">
        <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
        <span className="text-xs text-muted-foreground">Switches save at once; forms save with Save</span>
      </div>
      <nav aria-label="Settings sections" className="-mt-2 flex flex-wrap gap-1">
        {SECTIONS.map(([id, name]) => (
          <a key={id} href={`#set-${id}`} onClick={(e) => (e.preventDefault(), document.getElementById(`set-${id}`)?.scrollIntoView({ block: "start" }))}
            className="rounded-md border px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            {name}
          </a>
        ))}
      </nav>
      {s.configError && (
        <p className="rounded-lg border border-bad/40 px-4 py-3 text-sm text-bad">
          config.toml has an error, so nothing can be changed here until it is fixed: {s.configError}
        </p>
      )}

      <Panel id="set-features" label="What tock does">
        <p className="-mt-1 mb-4 text-sm text-muted-foreground">
          Turn off a whole part of tock with its group: its settings below are kept and come back when you turn it on.
        </p>
        <div className="divide-y">
          {groups.map((g) => (
            <section key={g.name} className="py-4 first:pt-0 last:pb-0">
              {g.head ? (
                <Row f={g.head} head onFlip={flip} disabled={!!s.configError} />
              ) : (
                <h3 className="mb-1 text-xs font-medium text-muted-foreground">{g.name}</h3>
              )}
              {g.items.length > 0 && (
                <ul className={cn(g.head && "mt-2 ml-1 border-l pl-4")}>
                  {g.items.map((f) => (
                    <li key={f.id}>
                      <Row f={f} waitsFor={f.waits || (f.on && byId(f.needs) && !byId(f.needs)!.on) ? byId(f.needs)?.name : undefined} onFlip={flip} disabled={!!s.configError} />
                    </li>
                  ))}
                </ul>
              )}
            </section>
          ))}
        </div>
      </Panel>

      <HoursPanel hours={s.hours} save={saveTo} disabled={!!s.configError} />
      <PresetsPanel presets={s.presets} max={s.maxSession} save={saveTo} disabled={!!s.configError} focusOff={byId("focus")?.on === false} />
      {byId("billing")?.on !== false ? (
        <InvoicePanel invoice={s.invoice} save={saveTo} disabled={!!s.configError} />
      ) : (
        <Panel id="set-invoices" label="Invoices"><p className="text-sm text-muted-foreground">Billing is off. Turn it on above to set your invoice details.</p></Panel>
      )}
      <Privacy s={s} setS={setS} disabled={!!s.configError} />
      <Appearance />

      <Panel id="set-files" label="Files">
        <div className="flex items-center gap-3">
          <code className="min-w-0 flex-1 truncate rounded-md bg-secondary px-2 py-1 text-xs">{s.configPath}</code>
          <Button variant="outline" size="sm" className="h-7 px-2.5 text-xs" onClick={() => copy(s.configPath)}><Copy className="h-3.5 w-3.5" />Copy path</Button>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">
          Everything else (notice timings, the score's weights, blocking lists) is set in this file. tock keeps your comments when it writes it.
        </p>
      </Panel>

      <Dialog open={!!ask} onOpenChange={(o) => !o && setAsk(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Send snippets to Claude?</DialogTitle>
            <DialogDescription>
              At most once an hour, tock sends what it couldn't label (app names, site hosts, cleaned titles and minutes) and your category and
              project names to Claude. Password managers, mail, messaging, banks and the apps and sites you hide are never sent.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setAsk(null)}>Keep it off</Button>
            <Button onClick={() => (ask && save(ask, true), setAsk(null))}>Turn Claude on</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function Row({ f, head, waitsFor, onFlip, disabled }: { f: Feature; head?: boolean; waitsFor?: string; onFlip: (f: Feature, on: boolean) => void; disabled?: boolean }) {
  const id = `set-${f.id}`;
  return (
    <div className={cn("flex items-center gap-4", head ? "py-1" : "py-2")}>
      <label htmlFor={id} className={cn("min-w-0 flex-1 cursor-pointer", waitsFor && "text-muted-foreground")}>
        <span className={cn(head ? "text-sm font-semibold" : "")}>{f.name}</span>
        {waitsFor && <span className="ml-2 text-xs text-faint">waits for {waitsFor}</span>}
        <span className="block text-xs text-muted-foreground">{f.note}</span>
      </label>
      <Switch id={id} checked={f.on} onChange={(v) => onFlip(f, v)} label={f.name} disabled={disabled} />
    </div>
  );
}

function Privacy({ s, setS, disabled }: { s: SettingsT; setS: (s: SettingsT) => void; disabled?: boolean }) {
  async function set(kind: "app" | "site", value: string, hide: boolean) {
    try {
      setS(await post<SettingsT>("/api/settings/hide", { kind, value, hide }));
      toast(hide ? `tock won't record ${value}` : `tock records ${value} again`, hide ? undefined : { action: { label: "Undo", onClick: () => set(kind, value, true) } });
    } catch (e) {
      toast.error((e as Error).message);
    }
  }
  return (
    <Panel id="set-privacy" label="Never record">
      <p className="-mt-1 mb-4 text-sm text-muted-foreground">
        tock records these as private time only: no title, no URL, no screen text, never sent to Claude. Time already recorded stays as it was.
      </p>
      <div className="grid gap-6 sm:grid-cols-2">
        <HideList title="Apps" hint="Bundle id, like com.apple.MobileSMS" items={s.hideApps} onAdd={(v) => set("app", v, true)} onRemove={(v) => set("app", v, false)} disabled={disabled} />
        <HideList title="Sites" hint="Host, like bank.example" items={s.hideSites} onAdd={(v) => set("site", v, true)} onRemove={(v) => set("site", v, false)} disabled={disabled} />
      </div>
    </Panel>
  );
}

function HideList({ title, hint, items, onAdd, onRemove, disabled }: { title: string; hint: string; items: string[]; onAdd: (v: string) => void; onRemove: (v: string) => void; disabled?: boolean }) {
  const [v, setV] = useState("");
  const add = () => {
    const t = v.trim();
    if (!t) return;
    onAdd(t);
    setV("");
  };
  return (
    <div>
      <h3 className="mb-2 text-xs font-medium text-muted-foreground">{title}</h3>
      <form className="flex gap-2" onSubmit={(e) => (e.preventDefault(), add())}>
        <input value={v} onChange={(e) => setV(e.target.value)} placeholder={hint} aria-label={`Add to hidden ${title.toLowerCase()}`} disabled={disabled}
          className="h-8 min-w-0 flex-1 rounded-md border bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" />
        <Button type="submit" variant="outline" size="sm" disabled={disabled || !v.trim()}>Add</Button>
      </form>
      {items.length ? (
        <ul className="mt-2 flex flex-wrap gap-1.5">
          {items.map((x) => (
            <li key={x} className="flex items-center gap-1 rounded-md border bg-secondary py-0.5 pl-2 pr-0.5 text-xs">
              {x}
              <button className="grid h-5 w-5 place-items-center rounded hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`Record ${x} again`} title="Record it again" onClick={() => onRemove(x)} disabled={disabled}>
                <X className="h-3 w-3" />
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="mt-2 text-xs text-faint">None yet.</p>
      )}
    </div>
  );
}

function Appearance() {
  const [t, setT] = useState<Theme>(getTheme);
  const native = /tock-app/.test(navigator.userAgent);
  return (
    <Panel id="set-page" label="This page">
      <div className="space-y-4">
        <div className="flex items-center gap-4">
          <div className="min-w-0 flex-1">
            <div>Theme</div>
            <div className="text-xs text-muted-foreground">Kept in this {native ? "window" : "browser"}</div>
          </div>
          <ToggleGroup type="single" value={t} onValueChange={(v) => v && (setTheme(v as Theme), setT(v as Theme))} variant="outline" size="sm">
            <ToggleGroupItem value="system">System</ToggleGroupItem>
            <ToggleGroupItem value="dark">Dark</ToggleGroupItem>
            <ToggleGroupItem value="light">Light</ToggleGroupItem>
          </ToggleGroup>
        </div>
        <div className="flex items-center gap-4">
          <div className="min-w-0 flex-1">
            <div>Size</div>
            <div className="text-xs text-muted-foreground">⌘+ and ⌘− zoom the page, ⌘0 sets it back{native ? "; the window remembers it" : ""}</div>
          </div>
        </div>
        <div className="flex items-center gap-4">
          <div className="min-w-0 flex-1">
            <div>Keys and gestures</div>
            <div className="text-xs text-muted-foreground">Every shortcut, and how to zoom the timeline</div>
          </div>
          <Button variant="outline" size="sm" onClick={openShortcuts}>Show <kbd className="ml-1 text-faint">?</kbd></Button>
        </div>
      </div>
    </Panel>
  );
}

async function copy(s: string) {
  try {
    await navigator.clipboard.writeText(s);
    toast("Copied the path");
  } catch {
    toast.error("Couldn't copy: select the path and copy it");
  }
}
