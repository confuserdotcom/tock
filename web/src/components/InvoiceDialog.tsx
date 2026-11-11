import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { get, post, type InvoiceIssued, type InvoicePreview, type Library } from "@/lib/api";
import { download } from "@/lib/download";
import { dur } from "@/lib/format";
import { go } from "@/lib/route";
import { periods } from "@/lib/rules";

const field = "h-8 w-full rounded-md border bg-background px-2.5 text-sm placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/** InvoiceDialog previews an invoice for one project with a rate, over a
 *  month or the week shown: its totals, a PDF marked DRAFT, and Issue, which
 *  stores it under the next number for good (tock invoice issue). */
export function InvoiceDialog({ open, monday, onClose }: { open: boolean; monday: string; onClose: () => void }) {
  const [lib, setLib] = useState<Library | null>(null);
  const [project, setProject] = useState("");
  const [period, setPeriod] = useState(0);
  const [client, setClient] = useState("");
  const [number, setNumber] = useState("");
  const [prev, setPrev] = useState<InvoicePreview | null>(null);
  const [err, setErr] = useState("");
  const [okModel, setOkModel] = useState(false);
  const [okOverlap, setOkOverlap] = useState(false);
  const [busy, setBusy] = useState(false);
  const ps = periods(monday);

  useEffect(() => {
    if (!open) return;
    get<Library>("/api/library").then((l) => {
      setLib(l);
      const rated = l.projects.filter((p) => p.rate != null && !p.archived);
      setProject((cur) => cur || rated[0]?.name || "");
    }).catch((e: Error) => setErr(e.message));
  }, [open]);

  const q = () => {
    const p = ps[period];
    const u = new URLSearchParams({ project, from: p.from, to: p.to });
    if (client.trim()) u.set("client", client.trim());
    if (number.trim()) u.set("number", number.trim());
    return u.toString();
  };

  useEffect(() => {
    if (!open || !project) return;
    let live = true;
    const t = setTimeout(() => {
      get<InvoicePreview>(`/api/invoice?${q()}&preview=1`)
        .then((p) => live && (setPrev(p), setErr(""), setOkModel(false), setOkOverlap(false)))
        .catch((e: Error) => live && (setPrev(null), setErr(e.message)));
    }, 250);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [open, project, period, number, monday]);

  async function save() {
    try {
      const r = await download(`/api/invoice?${q()}`, "invoice.pdf");
      if (r.error) toast.error(`Couldn't save ${r.name}: ${r.error}`);
      else if (r.browser) toast(`Downloading ${r.name}`);
      else if (r.saved) toast(`Saved ${r.name}`);
      if (!r.error) onClose();
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function issue() {
    const p = ps[period];
    setBusy(true);
    try {
      const r = await post<InvoiceIssued>("/api/invoice/issue", {
        project, from: p.from, to: p.to, client: client.trim(), number: number.trim(),
        allowOverlap: okOverlap, confirmModel: okModel,
      });
      toast(`Issued ${r.number} · ${money(r.total, r.currency)}`, {
        description: `Stored with its PDF; it stays as issued. tock invoice show ${r.number} --pdf saves a copy.`,
      });
      onClose();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const rated = lib?.projects.filter((p) => p.rate != null && !p.archived) ?? [];
  const canIssue = !!prev && !busy && (prev.modelOnlyS === 0 || okModel) && (prev.overlaps.length === 0 || okOverlap);
  const money = (v: number, c: string) => `${v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}${c ? ` ${c}` : ""}`;
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Invoice</DialogTitle>
          <DialogDescription>One project's time at its hourly rate, day by day. Save a DRAFT to check it; Issue gives it the next number and keeps it as it is.</DialogDescription>
        </DialogHeader>
        {lib && !rated.length ? (
          <p className="text-sm text-muted-foreground">
            No project has an hourly rate yet.{" "}
            <button className="underline underline-offset-2" onClick={() => (onClose(), go({ view: "library", sub: "projects" }))}>Set one in Library</button>.
          </p>
        ) : (
          <form className="space-y-4" onSubmit={(e) => (e.preventDefault(), save())}>
            <div className="grid grid-cols-2 gap-3">
              <label className="block space-y-1.5">
                <span className="lbl">Project</span>
                <select className={field} value={project} onChange={(e) => setProject(e.target.value)}>
                  {rated.map((p) => <option key={p.name} value={p.name}>{p.name}</option>)}
                </select>
              </label>
              <label className="block space-y-1.5">
                <span className="lbl">Period</span>
                <select className={field} value={period} onChange={(e) => setPeriod(Number(e.target.value))}>
                  {ps.map((p, i) => <option key={p.label} value={i}>{p.label}</option>)}
                </select>
              </label>
            </div>
            <label className="block space-y-1.5">
              <span className="lbl">To (client)</span>
              <input className={field} value={client} onChange={(e) => setClient(e.target.value)} placeholder="Acme Ltd" />
            </label>
            <label className="block space-y-1.5">
              <span className="lbl">Number</span>
              <input className={field} value={number} onChange={(e) => setNumber(e.target.value)} placeholder={prev?.number ?? ""} />
            </label>
            <p className="num min-h-5 text-sm">
              {err ? <span className="text-muted-foreground">{err}</span> : prev && (
                <><b className="font-medium">{dur(prev.seconds * 1000)}</b> · <b className="font-medium">{money(prev.total, prev.currency)}</b> · <span className="text-muted-foreground">{prev.days} {prev.days === 1 ? "day" : "days"}</span></>
              )}
            </p>
            {prev && prev.modelOnlyS > 0 && (
              <label className="flex items-start gap-2 text-sm">
                <input type="checkbox" className="mt-1" checked={okModel} onChange={(e) => setOkModel(e.target.checked)} />
                <span>{dur(prev.modelOnlyS * 1000)} of it only the local model labelled. I've looked at it.</span>
              </label>
            )}
            {prev && prev.overlaps.length > 0 && (
              <label className="flex items-start gap-2 text-sm">
                <input type="checkbox" className="mt-1" checked={okOverlap} onChange={(e) => setOkOverlap(e.target.checked)} />
                <span>{prev.overlaps.join(", ")} already bills some of these days. Bill them again.</span>
              </label>
            )}
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button type="submit" variant="outline" disabled={!prev}>Save DRAFT</Button>
              <Button type="button" disabled={!canIssue} onClick={issue}>Issue</Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
