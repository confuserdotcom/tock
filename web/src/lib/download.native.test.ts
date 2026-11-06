// download() in the tock window: the result and the blob's life follow
// tockbar's tock-download event, not the click.
import { test, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("./api", () => ({
  getRaw: vi.fn(async () => ({
    headers: new Headers({ "Content-Disposition": 'attachment; filename="tock.csv"' }),
    blob: async () => new Blob(["a,b\n"]),
  })),
}));

let revoke: ReturnType<typeof vi.fn>;
let clicks: number;

async function load(ua: string) {
  vi.resetModules();
  vi.stubGlobal("navigator", { userAgent: ua });
  vi.stubGlobal("window", new EventTarget());
  vi.stubGlobal("document", {
    body: { appendChild: () => {} },
    createElement: () => ({ click: () => clicks++, remove: () => {} }),
  });
  revoke = vi.fn();
  vi.stubGlobal("URL", Object.assign(Object.create(URL), { createObjectURL: () => "blob:x", revokeObjectURL: revoke }));
  return (await import("./download")).download;
}

function tell(detail: object) {
  window.dispatchEvent(new CustomEvent("tock-download", { detail }));
}

/** settled reports whether p has settled after pending microtasks run. */
async function settled(p: Promise<unknown>) {
  let done = false;
  void p.then(() => (done = true), () => (done = true));
  await vi.advanceTimersByTimeAsync(0);
  return done;
}

beforeEach(() => {
  clicks = 0;
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

test("waits for tockbar, however long the save sheet stays open", async () => {
  const download = await load("Mozilla/5.0 tock-app");
  const p = download("/api/export", "x.csv");
  await vi.advanceTimersByTimeAsync(5 * 60_000);
  expect(clicks).toBe(1);
  expect(await settled(p)).toBe(false);
  expect(revoke).not.toHaveBeenCalled();
  tell({ ok: true, name: "renamed.csv" });
  await expect(p).resolves.toEqual({ saved: true, name: "renamed.csv", error: undefined });
  expect(revoke).toHaveBeenCalledWith("blob:x");
});

test("a cancelled sheet is not saved and not an error", async () => {
  const download = await load("tock-app");
  const p = download("/api/export", "x.csv");
  await vi.advanceTimersByTimeAsync(0);
  tell({ ok: false });
  await expect(p).resolves.toEqual({ saved: false, name: "tock.csv", error: undefined });
});

test("a failed download carries tockbar's error", async () => {
  const download = await load("tock-app");
  const p = download("/api/export", "x.csv");
  await vi.advanceTimersByTimeAsync(0);
  tell({ ok: false, name: "tock.csv", error: "disk full" });
  await expect(p).resolves.toMatchObject({ saved: false, error: "disk full" });
});

test("in a browser the click hands it over", async () => {
  const download = await load("Mozilla/5.0 Safari");
  await expect(download("/api/export", "x.csv")).resolves.toEqual({ saved: true, name: "tock.csv", browser: true });
  expect(revoke).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(60_000);
  expect(revoke).toHaveBeenCalledWith("blob:x");
});
